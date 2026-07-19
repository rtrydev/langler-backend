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
	if (filter.Language == "pl" || filter.Language == "my") && filter.Level != "" {
		start := "GRAMMAR#A1#"
		end := "GRAMMAR#" + string(filter.Level) + "#\uffff"
		page, err := r.queryRange(ctx, filter.Language, start, end, filter.Limit, filter.Cursor)
		if err != nil {
			return outbound.GrammarPage{}, err
		}
		return grammarPage(page)
	}

	prefix := "GRAMMAR#"
	if filter.Level != "" {
		prefix += string(filter.Level) + "#"
	}

	page, err := r.query(ctx, filter.Language, prefix, filter.Limit, filter.Cursor, "", nil)
	if err != nil {
		return outbound.GrammarPage{}, err
	}

	return grammarPage(page)
}

func grammarPage(page rawPage) (outbound.GrammarPage, error) {
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

func (r *Repository) Topics(ctx context.Context, filter outbound.TopicFilter) ([]domain.Topic, error) {
	prefix := "TOPIC#"
	if filter.Level != "" {
		prefix += string(filter.Level) + "#"
	}

	var topics []domain.Topic
	cursor := ""
	for {
		page, err := r.query(ctx, filter.Language, prefix, 100, cursor, "", nil)
		if err != nil {
			return nil, err
		}
		var items []topicItem
		if err := attributevalue.UnmarshalListOfMaps(page.items, &items); err != nil {
			return nil, fmt.Errorf("%w: unmarshal topic items: %v", domain.ErrStorageFailure, err)
		}
		for _, item := range items {
			topic := item.toDomain()
			if filter.Slug != "" && topic.Slug != filter.Slug {
				continue
			}
			topics = append(topics, topic)
		}
		if page.nextCursor == "" {
			return topics, nil
		}
		cursor = page.nextCursor
	}
}

func (r *Repository) VocabByIDs(ctx context.Context, language domain.Language, ids []string) ([]domain.VocabEntry, error) {
	found := make(map[string]domain.VocabEntry, len(ids))
	for offset := 0; offset < len(ids); offset += 100 {
		end := min(offset+100, len(ids))
		keys := make([]map[string]types.AttributeValue, 0, end-offset)
		for _, id := range ids[offset:end] {
			keys = append(keys, referenceKey(string(language), "VOCAB#"+id))
		}
		if err := r.loadVocab(ctx, keys, found); err != nil {
			return nil, err
		}
	}
	entries := make([]domain.VocabEntry, 0, len(found))
	for _, id := range ids {
		if entry, ok := found[id]; ok {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (r *Repository) loadVocab(ctx context.Context, keys []map[string]types.AttributeValue, result map[string]domain.VocabEntry) error {
	pending := keys
	for attempt := 0; len(pending) > 0 && attempt < 4; attempt++ {
		out, err := r.client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{RequestItems: map[string]types.KeysAndAttributes{
			r.table: {Keys: pending},
		}})
		if err != nil {
			return fmt.Errorf("%w: batch get vocab: %v", domain.ErrStorageFailure, err)
		}
		var items []vocabItem
		if err := attributevalue.UnmarshalListOfMaps(out.Responses[r.table], &items); err != nil {
			return fmt.Errorf("%w: unmarshal vocab items: %v", domain.ErrStorageFailure, err)
		}
		for _, item := range items {
			entry := item.toDomain()
			result[entry.ID] = entry
		}
		pending = out.UnprocessedKeys[r.table].Keys
	}
	if len(pending) > 0 {
		return fmt.Errorf("%w: vocab batch remained unprocessed", domain.ErrStorageFailure)
	}
	return nil
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

func (r *Repository) Readings(ctx context.Context, filter outbound.ReadingFilter) (outbound.ReadingPage, error) {
	prefix := "READING#"
	if filter.Level != "" {
		prefix += string(filter.Level) + "#"
	}
	page, err := r.query(ctx, filter.Language, prefix, filter.Limit, filter.Cursor, "", nil)
	if err != nil {
		return outbound.ReadingPage{}, err
	}
	var items []readingItem
	if err := attributevalue.UnmarshalListOfMaps(page.items, &items); err != nil {
		return outbound.ReadingPage{}, fmt.Errorf("%w: unmarshal reading items: %v", domain.ErrStorageFailure, err)
	}
	passages := make([]domain.ReadingPassage, 0, len(items))
	for _, item := range items {
		passages = append(passages, item.toDomain())
	}
	return outbound.ReadingPage{Passages: passages, NextCursor: page.nextCursor}, nil
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

func (r *Repository) queryRange(
	ctx context.Context,
	lang domain.Language,
	start string,
	end string,
	limit int,
	cursor string,
) (rawPage, error) {
	partition := "REF#" + string(lang)
	input := &dynamodb.QueryInput{
		TableName:              aws.String(r.table),
		KeyConditionExpression: aws.String("PK = :pk AND SK BETWEEN :start AND :end"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: partition},
			":start": &types.AttributeValueMemberS{Value: start},
			":end":   &types.AttributeValueMemberS{Value: end},
		},
		Limit:            aws.Int32(int32(limit)),
		ScanIndexForward: aws.Bool(false),
	}
	if cursor != "" {
		startKey, err := decodeRangeCursor(cursor, partition, start, end)
		if err != nil {
			return rawPage{}, err
		}
		input.ExclusiveStartKey = startKey
	}

	out, err := r.client.Query(ctx, input)
	if err != nil {
		return rawPage{}, fmt.Errorf("%w: query %s through %s: %v", domain.ErrStorageFailure, start, end, err)
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

func decodeRangeCursor(cursor, partition, start, end string) (map[string]types.AttributeValue, error) {
	key, err := decodeCursor(cursor, partition, "GRAMMAR#")
	if err != nil {
		return nil, err
	}
	sk := key["SK"].(*types.AttributeValueMemberS).Value
	if sk < start || sk > end {
		return nil, fmt.Errorf("%w: cursor does not match the query", domain.ErrInvalidCursor)
	}
	return key, nil
}
