package dynamoref

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	domain "github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

type Repository struct {
	client *dynamodb.Client
	table  string
}

func NewRepository(client *dynamodb.Client, table string) (*Repository, error) {
	if client == nil {
		return nil, errors.New("dynamodb client must not be nil")
	}
	if table == "" {
		return nil, errors.New("table name must not be empty")
	}
	return &Repository{client: client, table: table}, nil
}

func (r *Repository) Vocab(ctx context.Context, filter outbound.VocabFilter) (outbound.VocabPage, error) {
	prefix := "VOCAB#"
	if filter.Level != "" {
		prefix += string(filter.Level) + "#"
	}

	var filterExpression string
	var filterValues map[string]types.AttributeValue
	if filter.Topic != "" {
		filterExpression = "contains(topics, :topic)"
		filterValues = map[string]types.AttributeValue{
			":topic": &types.AttributeValueMemberS{Value: string(filter.Topic)},
		}
	}

	page, err := r.query(ctx, filter.Language, prefix, filter.Limit, filter.Cursor, filterExpression, filterValues)
	if err != nil {
		return outbound.VocabPage{}, err
	}

	var items []vocabItem
	if err := attributevalue.UnmarshalListOfMaps(page.items, &items); err != nil {
		return outbound.VocabPage{}, fmt.Errorf("%w: unmarshal vocab items: %v", domain.ErrStorageFailure, err)
	}
	entries := make([]domain.VocabEntry, 0, len(items))
	for _, item := range items {
		entries = append(entries, item.toDomain())
	}
	return outbound.VocabPage{Entries: entries, NextCursor: page.nextCursor}, nil
}

func (r *Repository) Grammar(ctx context.Context, filter outbound.GrammarFilter) (outbound.GrammarPage, error) {
	prefix := "GRAMMAR#"
	if filter.Level != "" {
		prefix += string(filter.Level) + "#"
	}

	page, err := r.query(ctx, filter.Language, prefix, filter.Limit, filter.Cursor, "", nil)
	if err != nil {
		return outbound.GrammarPage{}, err
	}

	var items []grammarItem
	if err := attributevalue.UnmarshalListOfMaps(page.items, &items); err != nil {
		return outbound.GrammarPage{}, fmt.Errorf("%w: unmarshal grammar items: %v", domain.ErrStorageFailure, err)
	}
	topics := make([]domain.GrammarTopic, 0, len(items))
	for _, item := range items {
		topics = append(topics, item.toDomain())
	}
	return outbound.GrammarPage{Topics: topics, NextCursor: page.nextCursor}, nil
}

func (r *Repository) Scripts(ctx context.Context, filter outbound.ScriptFilter) (outbound.ScriptPage, error) {
	prefix := "SCRIPT#"
	if filter.ScriptType != "" {
		prefix += strings.ToUpper(string(filter.ScriptType)) + "#"
		if filter.Level != "" {
			prefix += string(filter.Level) + "#"
		}
	}

	page, err := r.query(ctx, filter.Language, prefix, filter.Limit, filter.Cursor, "", nil)
	if err != nil {
		return outbound.ScriptPage{}, err
	}

	var items []scriptItem
	if err := attributevalue.UnmarshalListOfMaps(page.items, &items); err != nil {
		return outbound.ScriptPage{}, fmt.Errorf("%w: unmarshal script items: %v", domain.ErrStorageFailure, err)
	}
	glyphs := make([]domain.ScriptGlyph, 0, len(items))
	for _, item := range items {
		glyphs = append(glyphs, item.toDomain())
	}
	return outbound.ScriptPage{Glyphs: glyphs, NextCursor: page.nextCursor}, nil
}

type rawPage struct {
	items      []map[string]types.AttributeValue
	nextCursor string
}

func (r *Repository) query(
	ctx context.Context,
	lang domain.Language,
	skPrefix string,
	limit int,
	cursor string,
	filterExpression string,
	filterValues map[string]types.AttributeValue,
) (rawPage, error) {
	partition := "REF#" + string(lang)
	input := &dynamodb.QueryInput{
		TableName:              aws.String(r.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: partition},
			":sk": &types.AttributeValueMemberS{Value: skPrefix},
		},
		Limit: aws.Int32(int32(limit)),
	}
	if filterExpression != "" {
		input.FilterExpression = aws.String(filterExpression)
		maps.Copy(input.ExpressionAttributeValues, filterValues)
	}
	if cursor != "" {
		startKey, err := decodeCursor(cursor, partition, skPrefix)
		if err != nil {
			return rawPage{}, err
		}
		input.ExclusiveStartKey = startKey
	}

	out, err := r.client.Query(ctx, input)
	if err != nil {
		return rawPage{}, fmt.Errorf("%w: query %s: %v", domain.ErrStorageFailure, skPrefix, err)
	}

	nextCursor, err := encodeCursor(out.LastEvaluatedKey)
	if err != nil {
		return rawPage{}, err
	}
	return rawPage{items: out.Items, nextCursor: nextCursor}, nil
}

type cursorKey struct {
	PK string `json:"pk"`
	SK string `json:"sk"`
}

func encodeCursor(lastKey map[string]types.AttributeValue) (string, error) {
	if lastKey == nil {
		return "", nil
	}
	pk, okPK := lastKey["PK"].(*types.AttributeValueMemberS)
	sk, okSK := lastKey["SK"].(*types.AttributeValueMemberS)
	if !okPK || !okSK {
		return "", errors.New("last evaluated key is missing string PK/SK members")
	}
	encoded, err := json.Marshal(cursorKey{PK: pk.Value, SK: sk.Value})
	if err != nil {
		return "", fmt.Errorf("%w: encode cursor: %v", domain.ErrStorageFailure, err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeCursor(cursor, partition, skPrefix string) (map[string]types.AttributeValue, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidCursor, err)
	}
	var key cursorKey
	if err := json.Unmarshal(decoded, &key); err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidCursor, err)
	}
	if key.PK != partition || !strings.HasPrefix(key.SK, skPrefix) {
		return nil, fmt.Errorf("%w: cursor does not match the query", domain.ErrInvalidCursor)
	}
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: key.PK},
		"SK": &types.AttributeValueMemberS{Value: key.SK},
	}, nil
}
