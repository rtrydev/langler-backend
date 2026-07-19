package dynamoprogress

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	domain "github.com/rtrydev/langler-backend/internal/domain/progress"
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

type itemRecord struct {
	PK                  string  `dynamodbav:"PK"`
	SK                  string  `dynamodbav:"SK"`
	ItemID              string  `dynamodbav:"itemId"`
	Language            string  `dynamodbav:"language"`
	Kind                string  `dynamodbav:"kind"`
	Headword            string  `dynamodbav:"headword"`
	Reading             string  `dynamodbav:"reading,omitempty"`
	Gloss               string  `dynamodbav:"gloss"`
	Example             string  `dynamodbav:"example,omitempty"`
	ExampleMeaning      string  `dynamodbav:"exampleMeaning,omitempty"`
	EaseFactor          float64 `dynamodbav:"easeFactor"`
	IntervalDays        int     `dynamodbav:"intervalDays"`
	Repetitions         int     `dynamodbav:"repetitions"`
	DueDate             string  `dynamodbav:"dueDate"`
	CreatedAt           string  `dynamodbav:"createdAt"`
	UpdatedAt           string  `dynamodbav:"updatedAt"`
	LastReviewedAt      string  `dynamodbav:"lastReviewedAt,omitempty"`
	LastLessonAttemptID string  `dynamodbav:"lastLessonAttemptId,omitempty"`
	Version             int     `dynamodbav:"version"`
}

type lessonActivityRecord struct {
	PK          string `dynamodbav:"PK"`
	SK          string `dynamodbav:"SK"`
	AttemptID   string `dynamodbav:"attemptId"`
	LessonID    string `dynamodbav:"lessonId"`
	Language    string `dynamodbav:"language"`
	Title       string `dynamodbav:"title"`
	Score       int    `dynamodbav:"score"`
	MaxScore    int    `dynamodbav:"maxScore"`
	CompletedAt string `dynamodbav:"completedAt"`
}

type reviewActivityRecord struct {
	PK         string `dynamodbav:"PK"`
	SK         string `dynamodbav:"SK"`
	ItemID     string `dynamodbav:"itemId"`
	Language   string `dynamodbav:"language"`
	Grade      string `dynamodbav:"grade"`
	ReviewedAt string `dynamodbav:"reviewedAt"`
	ReviewedOn string `dynamodbav:"reviewedOn,omitempty"`
}

func (r *Repository) GetItems(ctx context.Context, owner, language string, keys []string) (map[string]domain.Item, error) {
	items := make(map[string]domain.Item, len(keys))
	for offset := 0; offset < len(keys); offset += 100 {
		end := min(offset+100, len(keys))
		batch := make([]map[string]types.AttributeValue, 0, end-offset)
		for _, key := range keys[offset:end] {
			batch = append(batch, itemKey(owner, language, key))
		}
		if err := r.loadItems(ctx, batch, items); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (r *Repository) loadItems(ctx context.Context, keys []map[string]types.AttributeValue, result map[string]domain.Item) error {
	pending := keys
	for attempt := 0; len(pending) > 0 && attempt < 4; attempt++ {
		out, err := r.client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{RequestItems: map[string]types.KeysAndAttributes{
			r.table: {Keys: pending},
		}})
		if err != nil {
			return fmt.Errorf("%w: batch get progress items: %v", domain.ErrStorageFailure, err)
		}
		var records []itemRecord
		if err := attributevalue.UnmarshalListOfMaps(out.Responses[r.table], &records); err != nil {
			return fmt.Errorf("%w: unmarshal progress items: %v", domain.ErrStorageFailure, err)
		}
		for _, record := range records {
			item, err := record.toDomain()
			if err != nil {
				return err
			}
			result[domainKey(item)] = item
		}
		pending = out.UnprocessedKeys[r.table].Keys
	}
	if len(pending) > 0 {
		return fmt.Errorf("%w: progress batch remained unprocessed", domain.ErrStorageFailure)
	}
	return nil
}

func (r *Repository) SaveLesson(ctx context.Context, owner string, items []domain.Item, activity domain.LessonActivity) error {
	writes := make([]types.TransactWriteItem, 0, len(items)+1)
	activityMap, err := attributevalue.MarshalMap(toLessonActivity(owner, activity))
	if err != nil {
		return fmt.Errorf("%w: marshal lesson activity: %v", domain.ErrStorageFailure, err)
	}
	for _, item := range items {
		mapped, err := attributevalue.MarshalMap(toItem(owner, item))
		if err != nil {
			return fmt.Errorf("%w: marshal progress item: %v", domain.ErrStorageFailure, err)
		}
		writes = append(writes, types.TransactWriteItem{Put: conditionalItemPut(r.table, mapped, item.Version)})
	}
	writes = append(writes, types.TransactWriteItem{Put: &types.Put{
		TableName: aws.String(r.table), Item: activityMap, ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}})
	if _, err := r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: writes}); err != nil {
		if exists, getErr := r.activityExists(ctx, activityMap); getErr == nil && exists {
			return nil
		}
		var canceled *types.TransactionCanceledException
		if errors.As(err, &canceled) {
			return fmt.Errorf("%w: save lesson progress", domain.ErrConflict)
		}
		return fmt.Errorf("%w: save lesson progress: %v", domain.ErrStorageFailure, err)
	}
	return nil
}

func (r *Repository) activityExists(ctx context.Context, activity map[string]types.AttributeValue) (bool, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(r.table),
		Key:            map[string]types.AttributeValue{"PK": activity["PK"], "SK": activity["SK"]},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return false, err
	}
	return len(out.Item) > 0, nil
}

func (r *Repository) DueItems(ctx context.Context, owner, language string, dueOn time.Time) ([]domain.Item, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(r.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		FilterExpression:       aws.String("dueDate <= :due"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":  &types.AttributeValueMemberS{Value: "USER#" + owner},
			":sk":  &types.AttributeValueMemberS{Value: "SRS#" + language + "#"},
			":due": &types.AttributeValueMemberS{Value: endOfDay(dueOn).Format(time.RFC3339Nano)},
		},
	}
	var records []itemRecord
	for {
		out, err := r.client.Query(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("%w: query due items: %v", domain.ErrStorageFailure, err)
		}
		var page []itemRecord
		if err := attributevalue.UnmarshalListOfMaps(out.Items, &page); err != nil {
			return nil, fmt.Errorf("%w: unmarshal due items: %v", domain.ErrStorageFailure, err)
		}
		records = append(records, page...)
		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		input.ExclusiveStartKey = out.LastEvaluatedKey
	}
	items := make([]domain.Item, 0, len(records))
	for _, record := range records {
		item, err := record.toDomain()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].DueDate.Equal(items[j].DueDate) {
			return domainKey(items[i]) < domainKey(items[j])
		}
		return items[i].DueDate.Before(items[j].DueDate)
	})
	return items, nil
}

func (r *Repository) SaveReview(ctx context.Context, owner string, item domain.Item, activity domain.ReviewActivity) error {
	itemMap, err := attributevalue.MarshalMap(toItem(owner, item))
	if err != nil {
		return fmt.Errorf("%w: marshal progress item: %v", domain.ErrStorageFailure, err)
	}
	activityMap, err := attributevalue.MarshalMap(toReviewActivity(owner, item.Kind, activity))
	if err != nil {
		return fmt.Errorf("%w: marshal review activity: %v", domain.ErrStorageFailure, err)
	}
	_, err = r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{Put: conditionalItemPut(r.table, itemMap, item.Version)},
		{Put: &types.Put{TableName: aws.String(r.table), Item: activityMap, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
	}})
	if err != nil {
		if exists, getErr := r.activityExists(ctx, activityMap); getErr == nil && exists {
			return nil
		}
		var canceled *types.TransactionCanceledException
		if errors.As(err, &canceled) {
			return fmt.Errorf("%w: save review", domain.ErrConflict)
		}
		return fmt.Errorf("%w: save review: %v", domain.ErrStorageFailure, err)
	}
	return nil
}

func (r *Repository) CoveredItemIDs(ctx context.Context, owner, language string, kind domain.ItemKind) ([]string, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(r.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ProjectionExpression:   aws.String("itemId"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "USER#" + owner},
			":sk": &types.AttributeValueMemberS{Value: "SRS#" + language + "#" + string(kind) + "#"},
		},
	}
	var ids []string
	for {
		out, err := r.client.Query(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("%w: query covered items: %v", domain.ErrStorageFailure, err)
		}
		var records []struct {
			ItemID string `dynamodbav:"itemId"`
		}
		if err := attributevalue.UnmarshalListOfMaps(out.Items, &records); err != nil {
			return nil, fmt.Errorf("%w: unmarshal covered items: %v", domain.ErrStorageFailure, err)
		}
		for _, record := range records {
			ids = append(ids, record.ItemID)
		}
		if len(out.LastEvaluatedKey) == 0 {
			return ids, nil
		}
		input.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func (r *Repository) Snapshot(ctx context.Context, owner string) (outbound.ProgressSnapshot, error) {
	var snapshot outbound.ProgressSnapshot
	itemMaps, err := r.queryPrefix(ctx, owner, "SRS#")
	if err != nil {
		return snapshot, err
	}
	var items []itemRecord
	if err := attributevalue.UnmarshalListOfMaps(itemMaps, &items); err != nil {
		return snapshot, fmt.Errorf("%w: unmarshal progress snapshot: %v", domain.ErrStorageFailure, err)
	}
	for _, record := range items {
		item, err := record.toDomain()
		if err != nil {
			return snapshot, err
		}
		snapshot.Items = append(snapshot.Items, item)
	}

	lessonMaps, err := r.queryPrefix(ctx, owner, "ACTIVITY#LESSON#")
	if err != nil {
		return snapshot, err
	}
	var lessons []lessonActivityRecord
	if err := attributevalue.UnmarshalListOfMaps(lessonMaps, &lessons); err != nil {
		return snapshot, fmt.Errorf("%w: unmarshal lesson activity: %v", domain.ErrStorageFailure, err)
	}
	for _, record := range lessons {
		activity, err := record.toDomain()
		if err != nil {
			return snapshot, err
		}
		snapshot.LessonActivity = append(snapshot.LessonActivity, activity)
	}

	reviewMaps, err := r.queryPrefix(ctx, owner, "ACTIVITY#REVIEW#")
	if err != nil {
		return snapshot, err
	}
	var reviews []reviewActivityRecord
	if err := attributevalue.UnmarshalListOfMaps(reviewMaps, &reviews); err != nil {
		return snapshot, fmt.Errorf("%w: unmarshal review activity: %v", domain.ErrStorageFailure, err)
	}
	for _, record := range reviews {
		activity, err := record.toDomain()
		if err != nil {
			return snapshot, err
		}
		snapshot.ReviewActivity = append(snapshot.ReviewActivity, activity)
	}
	return snapshot, nil
}

func (r *Repository) queryPrefix(ctx context.Context, owner, prefix string) ([]map[string]types.AttributeValue, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(r.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "USER#" + owner},
			":sk": &types.AttributeValueMemberS{Value: prefix},
		},
	}
	var result []map[string]types.AttributeValue
	for {
		out, err := r.client.Query(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("%w: query %s: %v", domain.ErrStorageFailure, prefix, err)
		}
		result = append(result, out.Items...)
		if len(out.LastEvaluatedKey) == 0 {
			return result, nil
		}
		input.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func itemKey(owner, language, key string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "USER#" + owner},
		"SK": &types.AttributeValueMemberS{Value: "SRS#" + language + "#" + key},
	}
}

func domainKey(item domain.Item) string {
	return string(item.Kind) + "#" + item.ID
}

func toItem(owner string, item domain.Item) itemRecord {
	lastReviewed := ""
	if !item.LastReviewedAt.IsZero() {
		lastReviewed = item.LastReviewedAt.UTC().Format(time.RFC3339Nano)
	}
	return itemRecord{
		PK: "USER#" + owner, SK: "SRS#" + item.Language + "#" + domainKey(item),
		ItemID: item.ID, Language: item.Language, Kind: string(item.Kind), Headword: item.Headword,
		Reading: item.Reading, Gloss: item.Gloss, Example: item.Example, ExampleMeaning: item.ExampleMeaning,
		EaseFactor: item.EaseFactor, IntervalDays: item.IntervalDays, Repetitions: item.Repetitions,
		DueDate: item.DueDate.UTC().Format(time.RFC3339Nano), CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: item.UpdatedAt.UTC().Format(time.RFC3339Nano), LastReviewedAt: lastReviewed,
		LastLessonAttemptID: item.LastLessonAttemptID, Version: item.Version,
	}
}

func (record itemRecord) toDomain() (domain.Item, error) {
	dueDate, err := time.Parse(time.RFC3339Nano, record.DueDate)
	if err != nil {
		return domain.Item{}, fmt.Errorf("%w: invalid due date: %v", domain.ErrStorageFailure, err)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, record.CreatedAt)
	if err != nil {
		return domain.Item{}, fmt.Errorf("%w: invalid created time: %v", domain.ErrStorageFailure, err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, record.UpdatedAt)
	if err != nil {
		return domain.Item{}, fmt.Errorf("%w: invalid updated time: %v", domain.ErrStorageFailure, err)
	}
	var lastReviewed time.Time
	if record.LastReviewedAt != "" {
		lastReviewed, err = time.Parse(time.RFC3339Nano, record.LastReviewedAt)
		if err != nil {
			return domain.Item{}, fmt.Errorf("%w: invalid review time: %v", domain.ErrStorageFailure, err)
		}
	}
	return domain.Item{
		ID: record.ItemID, Language: record.Language, Kind: domain.ItemKind(record.Kind), Headword: record.Headword,
		Reading: record.Reading, Gloss: record.Gloss, Example: record.Example, ExampleMeaning: record.ExampleMeaning,
		EaseFactor: record.EaseFactor, IntervalDays: record.IntervalDays, Repetitions: record.Repetitions,
		DueDate: dueDate, CreatedAt: createdAt, UpdatedAt: updatedAt, LastReviewedAt: lastReviewed,
		LastLessonAttemptID: record.LastLessonAttemptID, Version: record.Version,
	}, nil
}

func toLessonActivity(owner string, activity domain.LessonActivity) lessonActivityRecord {
	return lessonActivityRecord{
		PK:        "USER#" + owner,
		SK:        "ACTIVITY#LESSON#" + activity.CompletedAt.UTC().Format("20060102T150405.000000000Z") + "#" + activity.AttemptID,
		AttemptID: activity.AttemptID, LessonID: activity.LessonID, Language: activity.Language, Title: activity.Title,
		Score: activity.Score, MaxScore: activity.MaxScore, CompletedAt: activity.CompletedAt.UTC().Format(time.RFC3339Nano),
	}
}

func (record lessonActivityRecord) toDomain() (domain.LessonActivity, error) {
	completedAt, err := time.Parse(time.RFC3339Nano, record.CompletedAt)
	if err != nil {
		return domain.LessonActivity{}, fmt.Errorf("%w: invalid completion time: %v", domain.ErrStorageFailure, err)
	}
	return domain.LessonActivity{
		AttemptID: record.AttemptID, LessonID: record.LessonID, Language: record.Language,
		Title: record.Title, Score: record.Score, MaxScore: record.MaxScore, CompletedAt: completedAt,
	}, nil
}

func toReviewActivity(owner string, kind domain.ItemKind, activity domain.ReviewActivity) reviewActivityRecord {
	reviewedOn := ""
	if !activity.ReviewedOn.IsZero() {
		reviewedOn = activity.ReviewedOn.Format(time.DateOnly)
	}
	return reviewActivityRecord{
		PK:     "USER#" + owner,
		SK:     "ACTIVITY#REVIEW#" + activity.ReviewedAt.UTC().Format("20060102T150405.000000000Z") + "#" + string(kind) + "#" + activity.ItemID,
		ItemID: activity.ItemID, Language: activity.Language, Grade: string(activity.Grade),
		ReviewedAt: activity.ReviewedAt.UTC().Format(time.RFC3339Nano), ReviewedOn: reviewedOn,
	}
}

func (record reviewActivityRecord) toDomain() (domain.ReviewActivity, error) {
	reviewedAt, err := time.Parse(time.RFC3339Nano, record.ReviewedAt)
	if err != nil {
		return domain.ReviewActivity{}, fmt.Errorf("%w: invalid review time: %v", domain.ErrStorageFailure, err)
	}
	var reviewedOn time.Time
	if record.ReviewedOn != "" {
		reviewedOn, err = time.Parse(time.DateOnly, record.ReviewedOn)
		if err != nil {
			return domain.ReviewActivity{}, fmt.Errorf("%w: invalid review date: %v", domain.ErrStorageFailure, err)
		}
	}
	return domain.ReviewActivity{
		ItemID: record.ItemID, Language: record.Language, Grade: domain.Grade(record.Grade),
		ReviewedAt: reviewedAt, ReviewedOn: reviewedOn,
	}, nil
}

func conditionalItemPut(table string, item map[string]types.AttributeValue, version int) *types.Put {
	expected := max(0, version-1)
	put := &types.Put{
		TableName:                aws.String(table),
		Item:                     item,
		ExpressionAttributeNames: map[string]string{"#version": "version"},
	}
	if expected == 0 {
		put.ConditionExpression = aws.String("attribute_not_exists(PK) OR attribute_not_exists(#version)")
		return put
	}
	put.ConditionExpression = aws.String("#version = :expected")
	put.ExpressionAttributeValues = map[string]types.AttributeValue{
		":expected": &types.AttributeValueMemberN{Value: strconv.Itoa(expected)},
	}
	return put
}

func endOfDay(value time.Time) time.Time {
	value = value.UTC()
	start := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
	return start.AddDate(0, 0, 1).Add(-time.Nanosecond)
}
