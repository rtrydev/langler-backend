package dynamolessons

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	domain "github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

type idempotencyItem struct {
	PK          string `dynamodbav:"PK"`
	SK          string `dynamodbav:"SK"`
	LessonID    string `dynamodbav:"lessonId"`
	ContentHash string `dynamodbav:"contentHash"`
	CreatedAt   string `dynamodbav:"createdAt"`
}

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

func (r *Repository) SaveIdempotent(ctx context.Context, record outbound.LessonRecord, key string) (outbound.LessonRecord, bool, error) {
	lessonMap, err := attributevalue.MarshalMap(toItem(record.Owner, record.ContentHash, record.CreatedAt, record.Lesson))
	if err != nil {
		return outbound.LessonRecord{}, false, fmt.Errorf("%w: marshal lesson: %v", domain.ErrStorageFailure, err)
	}
	keyHash := sha256.Sum256([]byte(key))
	marker := idempotencyItem{
		PK: "USER#" + record.Owner, SK: "IDEMPOTENCY#" + fmt.Sprintf("%x", keyHash[:]),
		LessonID: record.Lesson.ID, ContentHash: record.ContentHash, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	markerMap, err := attributevalue.MarshalMap(marker)
	if err != nil {
		return outbound.LessonRecord{}, false, fmt.Errorf("%w: marshal idempotency marker: %v", domain.ErrStorageFailure, err)
	}
	_, err = r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{Put: &types.Put{TableName: aws.String(r.table), Item: lessonMap, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
		{Put: &types.Put{TableName: aws.String(r.table), Item: markerMap, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
	}})
	if err == nil {
		return record, true, nil
	}
	var cancelled *types.TransactionCanceledException
	if !errors.As(err, &cancelled) {
		return outbound.LessonRecord{}, false, fmt.Errorf("%w: import lesson: %v", domain.ErrStorageFailure, err)
	}
	existingID := record.Lesson.ID
	markerOut, getErr := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: marker.PK},
			"SK": &types.AttributeValueMemberS{Value: marker.SK},
		},
		ConsistentRead: aws.Bool(true),
	})
	if getErr != nil {
		return outbound.LessonRecord{}, false, fmt.Errorf("%w: get idempotency marker: %v", domain.ErrStorageFailure, getErr)
	}
	if markerOut.Item != nil {
		var existingMarker idempotencyItem
		if err := attributevalue.UnmarshalMap(markerOut.Item, &existingMarker); err != nil {
			return outbound.LessonRecord{}, false, fmt.Errorf("%w: unmarshal idempotency marker: %v", domain.ErrStorageFailure, err)
		}
		existingID = existingMarker.LessonID
	}
	existing, getErr := r.Get(ctx, record.Owner, existingID)
	if getErr != nil {
		return outbound.LessonRecord{}, false, getErr
	}
	return existing, false, nil
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

func (r *Repository) SaveResult(ctx context.Context, record outbound.ResultRecord) error {
	result := record.Result
	exercises := make([]exerciseResultItem, 0, len(result.Exercises))
	for _, exercise := range result.Exercises {
		exercises = append(exercises, exerciseResultItem{
			ExerciseID: exercise.ExerciseID,
			Type:       string(exercise.Type),
			Grading:    exercise.Grading,
			Score:      exercise.Score,
			MaxScore:   exercise.MaxScore,
			Correct:    exercise.Correct,
			Total:      exercise.Total,
		})
	}
	item, err := attributevalue.MarshalMap(resultItem{
		PK:          "USER#" + record.Owner,
		SK:          "RESULT#" + result.LessonID + "#" + result.CompletedAt.UTC().Format("20060102T150405.000000000Z") + "#" + result.AttemptID,
		AttemptID:   result.AttemptID,
		LessonID:    result.LessonID,
		StartedAt:   result.StartedAt.UTC().Format(time.RFC3339Nano),
		CompletedAt: result.CompletedAt.UTC().Format(time.RFC3339Nano),
		Score:       result.Score,
		MaxScore:    result.MaxScore,
		AutoScore:   result.AutoScore,
		AutoMax:     result.AutoMax,
		SelfScore:   result.SelfScore,
		SelfMax:     result.SelfMax,
		Exercises:   exercises,
	})
	if err != nil {
		return fmt.Errorf("%w: marshal result: %v", domain.ErrStorageFailure, err)
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
		return fmt.Errorf("%w: put result: %v", domain.ErrStorageFailure, err)
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
