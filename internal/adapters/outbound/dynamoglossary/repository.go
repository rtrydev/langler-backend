package dynamoglossary

import (
	"context"
	"errors"
	"fmt"
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

type entryRecord struct {
	PK        string   `dynamodbav:"PK"`
	SK        string   `dynamodbav:"SK"`
	ItemID    string   `dynamodbav:"itemId"`
	Language  string   `dynamodbav:"language"`
	Kind      string   `dynamodbav:"kind"`
	LessonIDs []string `dynamodbav:"lessonIds,stringset,omitempty"`
	AddedAt   string   `dynamodbav:"addedAt"`
	UpdatedAt string   `dynamodbav:"updatedAt"`
}

type wordRef struct {
	kind domain.ItemKind
	id   string
}

func wordRefs(refs outbound.GlossaryRefs) []wordRef {
	words := make([]wordRef, 0, len(refs.VocabIDs)+len(refs.GrammarIDs))
	for _, id := range refs.VocabIDs {
		words = append(words, wordRef{kind: domain.KindVocab, id: id})
	}
	for _, id := range refs.GrammarIDs {
		words = append(words, wordRef{kind: domain.KindGrammar, id: id})
	}
	return words
}

func (r *Repository) AddLessonWords(ctx context.Context, owner, language, lessonID string, refs outbound.GlossaryRefs, addedAt time.Time) error {
	now := addedAt.UTC().Format(time.RFC3339Nano)
	for _, word := range wordRefs(refs) {
		_, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName:        aws.String(r.table),
			Key:              entryKey(owner, language, word.kind, word.id),
			UpdateExpression: aws.String("ADD #lessons :lesson SET #itemId = if_not_exists(#itemId, :id), #language = if_not_exists(#language, :language), #kind = if_not_exists(#kind, :kind), #addedAt = if_not_exists(#addedAt, :now), #updatedAt = :now"),
			ExpressionAttributeNames: map[string]string{
				"#lessons": "lessonIds", "#itemId": "itemId", "#language": "language",
				"#kind": "kind", "#addedAt": "addedAt", "#updatedAt": "updatedAt",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":lesson":   &types.AttributeValueMemberSS{Value: []string{lessonID}},
				":id":       &types.AttributeValueMemberS{Value: word.id},
				":language": &types.AttributeValueMemberS{Value: language},
				":kind":     &types.AttributeValueMemberS{Value: string(word.kind)},
				":now":      &types.AttributeValueMemberS{Value: now},
			},
		})
		if err != nil {
			return fmt.Errorf("%w: add glossary word %s: %v", domain.ErrStorageFailure, word.id, err)
		}
	}
	return nil
}

func (r *Repository) RemoveLessonWords(ctx context.Context, owner, language, lessonID string, refs outbound.GlossaryRefs) error {
	for _, word := range wordRefs(refs) {
		out, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName:                aws.String(r.table),
			Key:                      entryKey(owner, language, word.kind, word.id),
			ConditionExpression:      aws.String("attribute_exists(PK)"),
			UpdateExpression:         aws.String("DELETE #lessons :lesson"),
			ExpressionAttributeNames: map[string]string{"#lessons": "lessonIds"},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":lesson": &types.AttributeValueMemberSS{Value: []string{lessonID}},
			},
			ReturnValues: types.ReturnValueAllNew,
		})
		if err != nil {
			var conditionFailed *types.ConditionalCheckFailedException
			if errors.As(err, &conditionFailed) {
				continue
			}
			return fmt.Errorf("%w: remove glossary word %s: %v", domain.ErrStorageFailure, word.id, err)
		}
		if _, stillReferenced := out.Attributes["lessonIds"]; stillReferenced {
			continue
		}
		_, err = r.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName:                aws.String(r.table),
			Key:                      entryKey(owner, language, word.kind, word.id),
			ConditionExpression:      aws.String("attribute_not_exists(#lessons)"),
			ExpressionAttributeNames: map[string]string{"#lessons": "lessonIds"},
		})
		if err != nil {
			var conditionFailed *types.ConditionalCheckFailedException
			if errors.As(err, &conditionFailed) {
				continue
			}
			return fmt.Errorf("%w: delete glossary word %s: %v", domain.ErrStorageFailure, word.id, err)
		}
	}
	return nil
}

func (r *Repository) Entries(ctx context.Context, owner, language string, kind domain.ItemKind) ([]outbound.GlossaryEntry, error) {
	records, err := r.queryPrefix(ctx, owner, language, kind, nil)
	if err != nil {
		return nil, err
	}
	entries := make([]outbound.GlossaryEntry, 0, len(records))
	for _, record := range records {
		addedAt, err := time.Parse(time.RFC3339Nano, record.AddedAt)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid glossary added time: %v", domain.ErrStorageFailure, err)
		}
		entries = append(entries, outbound.GlossaryEntry{
			ID:          record.ItemID,
			Language:    record.Language,
			Kind:        domain.ItemKind(record.Kind),
			LessonCount: len(record.LessonIDs),
			AddedAt:     addedAt,
		})
	}
	return entries, nil
}

func (r *Repository) GlossaryItemIDs(ctx context.Context, owner, language string, kind domain.ItemKind) ([]string, error) {
	records, err := r.queryPrefix(ctx, owner, language, kind, aws.String("itemId"))
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ItemID)
	}
	return ids, nil
}

func (r *Repository) queryPrefix(ctx context.Context, owner, language string, kind domain.ItemKind, projection *string) ([]entryRecord, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(r.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ProjectionExpression:   projection,
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "USER#" + owner},
			":sk": &types.AttributeValueMemberS{Value: "GLOSSARY#" + language + "#" + string(kind) + "#"},
		},
	}
	var records []entryRecord
	for {
		out, err := r.client.Query(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("%w: query glossary: %v", domain.ErrStorageFailure, err)
		}
		var page []entryRecord
		if err := attributevalue.UnmarshalListOfMaps(out.Items, &page); err != nil {
			return nil, fmt.Errorf("%w: unmarshal glossary entries: %v", domain.ErrStorageFailure, err)
		}
		records = append(records, page...)
		if len(out.LastEvaluatedKey) == 0 {
			return records, nil
		}
		input.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func entryKey(owner, language string, kind domain.ItemKind, id string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "USER#" + owner},
		"SK": &types.AttributeValueMemberS{Value: "GLOSSARY#" + language + "#" + string(kind) + "#" + id},
	}
}
