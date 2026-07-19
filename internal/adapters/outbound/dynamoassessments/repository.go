package dynamoassessments

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	domain "github.com/rtrydev/langler-backend/internal/domain/assessment"
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
	Kind         string   `dynamodbav:"kind"`
	Prompt       string   `dynamodbav:"prompt"`
	Options      []string `dynamodbav:"options"`
	CorrectIndex int      `dynamodbav:"correctIndex"`
	ReferenceID  string   `dynamodbav:"referenceId,omitempty"`
}

type stageRecord struct {
	Band       string       `dynamodbav:"band"`
	Items      []itemRecord `dynamodbav:"items"`
	Answers    []int        `dynamodbav:"answers,omitempty"`
	Answered   bool         `dynamodbav:"answered"`
	Correct    int          `dynamodbav:"correct"`
	AnsweredAt string       `dynamodbav:"answeredAt,omitempty"`
}

type sessionRecord struct {
	PK             string        `dynamodbav:"PK"`
	SK             string        `dynamodbav:"SK"`
	AssessmentID   string        `dynamodbav:"assessmentId"`
	Language       string        `dynamodbav:"language"`
	Status         string        `dynamodbav:"status"`
	Bands          []string      `dynamodbav:"bands"`
	Stages         []stageRecord `dynamodbav:"stages"`
	EstimatedLevel string        `dynamodbav:"estimatedLevel,omitempty"`
	Confidence     string        `dynamodbav:"confidence,omitempty"`
	Floor          bool          `dynamodbav:"floor"`
	StartedAt      string        `dynamodbav:"startedAt"`
	CompletedAt    string        `dynamodbav:"completedAt,omitempty"`
	Version        int           `dynamodbav:"version"`
}

type levelRecord struct {
	PK           string `dynamodbav:"PK"`
	SK           string `dynamodbav:"SK"`
	Language     string `dynamodbav:"language"`
	Level        string `dynamodbav:"level"`
	AssessmentID string `dynamodbav:"assessmentId"`
	UpdatedAt    string `dynamodbav:"updatedAt"`
}

func (r *Repository) Create(ctx context.Context, owner string, session domain.Session) error {
	item, err := attributevalue.MarshalMap(toRecord(owner, session))
	if err != nil {
		return fmt.Errorf("%w: marshal assessment: %v", domain.ErrStorageFailure, err)
	}
	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		var conditionFailed *types.ConditionalCheckFailedException
		if errors.As(err, &conditionFailed) {
			return fmt.Errorf("%w: create assessment", domain.ErrConflict)
		}
		return fmt.Errorf("%w: create assessment: %v", domain.ErrStorageFailure, err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, owner, id string) (domain.Session, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + owner},
			"SK": &types.AttributeValueMemberS{Value: "ASSESSMENT#" + id},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return domain.Session{}, fmt.Errorf("%w: get assessment: %v", domain.ErrStorageFailure, err)
	}
	if len(out.Item) == 0 {
		return domain.Session{}, domain.ErrNotFound
	}
	var record sessionRecord
	if err := attributevalue.UnmarshalMap(out.Item, &record); err != nil {
		return domain.Session{}, fmt.Errorf("%w: unmarshal assessment: %v", domain.ErrStorageFailure, err)
	}
	return record.toDomain()
}

func (r *Repository) Save(ctx context.Context, owner string, session domain.Session, level *outbound.ProfileLevelRecord) error {
	item, err := attributevalue.MarshalMap(toRecord(owner, session))
	if err != nil {
		return fmt.Errorf("%w: marshal assessment: %v", domain.ErrStorageFailure, err)
	}
	put := &types.Put{
		TableName:                aws.String(r.table),
		Item:                     item,
		ConditionExpression:      aws.String("#version = :expected"),
		ExpressionAttributeNames: map[string]string{"#version": "version"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":expected": &types.AttributeValueMemberN{Value: strconv.Itoa(session.Version - 1)},
		},
	}
	if level == nil {
		_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName:                 put.TableName,
			Item:                      put.Item,
			ConditionExpression:       put.ConditionExpression,
			ExpressionAttributeNames:  put.ExpressionAttributeNames,
			ExpressionAttributeValues: put.ExpressionAttributeValues,
		})
		if err != nil {
			var conditionFailed *types.ConditionalCheckFailedException
			if errors.As(err, &conditionFailed) {
				return fmt.Errorf("%w: save assessment", domain.ErrConflict)
			}
			return fmt.Errorf("%w: save assessment: %v", domain.ErrStorageFailure, err)
		}
		return nil
	}
	levelItem, err := attributevalue.MarshalMap(levelRecord{
		PK:           "USER#" + owner,
		SK:           "PROFILE#LEVEL#" + level.Language,
		Language:     level.Language,
		Level:        level.Level,
		AssessmentID: level.AssessmentID,
		UpdatedAt:    level.UpdatedAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("%w: marshal profile level: %v", domain.ErrStorageFailure, err)
	}
	_, err = r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{Put: put},
		{Put: &types.Put{TableName: aws.String(r.table), Item: levelItem}},
	}})
	if err != nil {
		var canceled *types.TransactionCanceledException
		if errors.As(err, &canceled) {
			return fmt.Errorf("%w: save assessment", domain.ErrConflict)
		}
		return fmt.Errorf("%w: save assessment: %v", domain.ErrStorageFailure, err)
	}
	return nil
}

func (r *Repository) List(ctx context.Context, owner string) ([]domain.Session, error) {
	maps, err := r.queryPrefix(ctx, owner, "ASSESSMENT#")
	if err != nil {
		return nil, err
	}
	var records []sessionRecord
	if err := attributevalue.UnmarshalListOfMaps(maps, &records); err != nil {
		return nil, fmt.Errorf("%w: unmarshal assessments: %v", domain.ErrStorageFailure, err)
	}
	sessions := make([]domain.Session, 0, len(records))
	for _, record := range records {
		session, err := record.toDomain()
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (r *Repository) Levels(ctx context.Context, owner string) ([]outbound.ProfileLevelRecord, error) {
	maps, err := r.queryPrefix(ctx, owner, "PROFILE#LEVEL#")
	if err != nil {
		return nil, err
	}
	var records []levelRecord
	if err := attributevalue.UnmarshalListOfMaps(maps, &records); err != nil {
		return nil, fmt.Errorf("%w: unmarshal profile levels: %v", domain.ErrStorageFailure, err)
	}
	levels := make([]outbound.ProfileLevelRecord, 0, len(records))
	for _, record := range records {
		updatedAt, err := time.Parse(time.RFC3339Nano, record.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid profile level time: %v", domain.ErrStorageFailure, err)
		}
		levels = append(levels, outbound.ProfileLevelRecord{
			Language:     record.Language,
			Level:        record.Level,
			AssessmentID: record.AssessmentID,
			UpdatedAt:    updatedAt,
		})
	}
	return levels, nil
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

func toRecord(owner string, session domain.Session) sessionRecord {
	stages := make([]stageRecord, 0, len(session.Stages))
	for _, stage := range session.Stages {
		items := make([]itemRecord, 0, len(stage.Items))
		for _, item := range stage.Items {
			items = append(items, itemRecord{
				Kind:         string(item.Kind),
				Prompt:       item.Prompt,
				Options:      item.Options,
				CorrectIndex: item.CorrectIndex,
				ReferenceID:  item.ReferenceID,
			})
		}
		answeredAt := ""
		if !stage.AnsweredAt.IsZero() {
			answeredAt = stage.AnsweredAt.UTC().Format(time.RFC3339Nano)
		}
		stages = append(stages, stageRecord{
			Band:       stage.Band,
			Items:      items,
			Answers:    stage.Answers,
			Answered:   stage.Answered,
			Correct:    stage.Correct,
			AnsweredAt: answeredAt,
		})
	}
	completedAt := ""
	if !session.CompletedAt.IsZero() {
		completedAt = session.CompletedAt.UTC().Format(time.RFC3339Nano)
	}
	return sessionRecord{
		PK:             "USER#" + owner,
		SK:             "ASSESSMENT#" + session.ID,
		AssessmentID:   session.ID,
		Language:       session.Language,
		Status:         string(session.Status),
		Bands:          session.Bands,
		Stages:         stages,
		EstimatedLevel: session.EstimatedLevel,
		Confidence:     string(session.Confidence),
		Floor:          session.Floor,
		StartedAt:      session.StartedAt.UTC().Format(time.RFC3339Nano),
		CompletedAt:    completedAt,
		Version:        session.Version,
	}
}

func (record sessionRecord) toDomain() (domain.Session, error) {
	startedAt, err := time.Parse(time.RFC3339Nano, record.StartedAt)
	if err != nil {
		return domain.Session{}, fmt.Errorf("%w: invalid start time: %v", domain.ErrStorageFailure, err)
	}
	var completedAt time.Time
	if record.CompletedAt != "" {
		completedAt, err = time.Parse(time.RFC3339Nano, record.CompletedAt)
		if err != nil {
			return domain.Session{}, fmt.Errorf("%w: invalid completion time: %v", domain.ErrStorageFailure, err)
		}
	}
	stages := make([]domain.Stage, 0, len(record.Stages))
	for _, stage := range record.Stages {
		items := make([]domain.Item, 0, len(stage.Items))
		for _, item := range stage.Items {
			items = append(items, domain.Item{
				Kind:         domain.ItemKind(item.Kind),
				Prompt:       item.Prompt,
				Options:      item.Options,
				CorrectIndex: item.CorrectIndex,
				ReferenceID:  item.ReferenceID,
			})
		}
		var answeredAt time.Time
		if stage.AnsweredAt != "" {
			answeredAt, err = time.Parse(time.RFC3339Nano, stage.AnsweredAt)
			if err != nil {
				return domain.Session{}, fmt.Errorf("%w: invalid stage time: %v", domain.ErrStorageFailure, err)
			}
		}
		stages = append(stages, domain.Stage{
			Band:       stage.Band,
			Items:      items,
			Answers:    stage.Answers,
			Answered:   stage.Answered,
			Correct:    stage.Correct,
			AnsweredAt: answeredAt,
		})
	}
	return domain.Session{
		ID:             record.AssessmentID,
		Language:       record.Language,
		Status:         domain.Status(record.Status),
		Bands:          record.Bands,
		Stages:         stages,
		EstimatedLevel: record.EstimatedLevel,
		Confidence:     domain.Confidence(record.Confidence),
		Floor:          record.Floor,
		StartedAt:      startedAt,
		CompletedAt:    completedAt,
		Version:        record.Version,
	}, nil
}
