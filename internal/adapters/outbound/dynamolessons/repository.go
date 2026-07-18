package dynamolessons

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	domain "github.com/rtrydev/langler-backend/internal/domain/lesson"
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

func (r *Repository) Save(ctx context.Context, record outbound.LessonRecord) error {
	item, err := attributevalue.MarshalMap(toItem(record.Owner, record.ContentHash, record.CreatedAt, record.Lesson))
	if err != nil {
		return fmt.Errorf("%w: marshal lesson: %v", domain.ErrStorageFailure, err)
	}
	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		var conditionFailed *types.ConditionalCheckFailedException
		if errors.As(err, &conditionFailed) {
			return domain.ErrAlreadyExists
		}
		return fmt.Errorf("%w: put lesson: %v", domain.ErrStorageFailure, err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, owner, id string) (outbound.LessonRecord, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.table),
		Key:       lessonKey(owner, id),
	})
	if err != nil {
		return outbound.LessonRecord{}, fmt.Errorf("%w: get lesson: %v", domain.ErrStorageFailure, err)
	}
	if out.Item == nil {
		return outbound.LessonRecord{}, domain.ErrNotFound
	}
	var item lessonItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return outbound.LessonRecord{}, fmt.Errorf("%w: unmarshal lesson: %v", domain.ErrStorageFailure, err)
	}
	return toRecord(owner, item), nil
}

func (r *Repository) List(ctx context.Context, owner string, limit int, cursor string) (outbound.LessonPage, error) {
	partition := "USER#" + owner
	input := &dynamodb.QueryInput{
		TableName:              aws.String(r.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: partition},
			":sk": &types.AttributeValueMemberS{Value: "LESSON#"},
		},
		Limit: aws.Int32(int32(limit)),
	}
	if cursor != "" {
		startKey, err := decodeCursor(cursor, partition)
		if err != nil {
			return outbound.LessonPage{}, err
		}
		input.ExclusiveStartKey = startKey
	}

	out, err := r.client.Query(ctx, input)
	if err != nil {
		return outbound.LessonPage{}, fmt.Errorf("%w: query lessons: %v", domain.ErrStorageFailure, err)
	}

	var items []lessonItem
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &items); err != nil {
		return outbound.LessonPage{}, fmt.Errorf("%w: unmarshal lessons: %v", domain.ErrStorageFailure, err)
	}
	records := make([]outbound.LessonRecord, 0, len(items))
	for _, item := range items {
		records = append(records, toRecord(owner, item))
	}

	nextCursor, err := encodeCursor(out.LastEvaluatedKey)
	if err != nil {
		return outbound.LessonPage{}, err
	}
	return outbound.LessonPage{Records: records, NextCursor: nextCursor}, nil
}

func (r *Repository) Delete(ctx context.Context, owner, id string) error {
	_, err := r.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(r.table),
		Key:                 lessonKey(owner, id),
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})
	if err != nil {
		var conditionFailed *types.ConditionalCheckFailedException
		if errors.As(err, &conditionFailed) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("%w: delete lesson: %v", domain.ErrStorageFailure, err)
	}
	return nil
}

func lessonKey(owner, id string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "USER#" + owner},
		"SK": &types.AttributeValueMemberS{Value: "LESSON#" + id},
	}
}

func toRecord(owner string, item lessonItem) outbound.LessonRecord {
	l, createdAt, contentHash := item.toDomain()
	return outbound.LessonRecord{Owner: owner, ContentHash: contentHash, CreatedAt: createdAt, Lesson: l}
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

func decodeCursor(cursor, partition string) (map[string]types.AttributeValue, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidCursor, err)
	}
	var key cursorKey
	if err := json.Unmarshal(decoded, &key); err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidCursor, err)
	}
	if key.PK != partition || !strings.HasPrefix(key.SK, "LESSON#") {
		return nil, fmt.Errorf("%w: cursor does not match the query", domain.ErrInvalidCursor)
	}
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: key.PK},
		"SK": &types.AttributeValueMemberS{Value: key.SK},
	}, nil
}
