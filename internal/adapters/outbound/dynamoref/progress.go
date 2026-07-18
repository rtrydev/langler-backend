package dynamoref

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/rtrydev/langler-backend/internal/domain/progress"
	"github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

func (r *Repository) LookupProgress(ctx context.Context, language string, vocabIDs, grammarIDs []string) (map[string]outbound.ReferenceContext, error) {
	keys := make([]map[string]types.AttributeValue, 0, len(vocabIDs)+len(grammarIDs))
	for _, id := range vocabIDs {
		keys = append(keys, referenceKey(language, "VOCAB#"+id))
	}
	for _, id := range grammarIDs {
		keys = append(keys, referenceKey(language, "GRAMMAR#"+id))
	}
	result := make(map[string]outbound.ReferenceContext, len(keys))
	for offset := 0; offset < len(keys); offset += 100 {
		end := min(offset+100, len(keys))
		if err := r.loadProgressReferences(ctx, keys[offset:end], result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *Repository) loadProgressReferences(ctx context.Context, keys []map[string]types.AttributeValue, result map[string]outbound.ReferenceContext) error {
	pending := keys
	for attempt := 0; len(pending) > 0 && attempt < 4; attempt++ {
		out, err := r.client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{RequestItems: map[string]types.KeysAndAttributes{
			r.table: {Keys: pending},
		}})
		if err != nil {
			return fmt.Errorf("%w: batch get progress references: %v", reference.ErrStorageFailure, err)
		}
		for _, item := range out.Responses[r.table] {
			sk, ok := item["SK"].(*types.AttributeValueMemberS)
			if !ok {
				return fmt.Errorf("%w: progress reference is missing SK", reference.ErrStorageFailure)
			}
			switch {
			case len(sk.Value) > len("VOCAB#") && sk.Value[:len("VOCAB#")] == "VOCAB#":
				var record vocabItem
				if err := attributevalue.UnmarshalMap(item, &record); err != nil {
					return fmt.Errorf("%w: unmarshal vocab context: %v", reference.ErrStorageFailure, err)
				}
				entry := record.toDomain()
				context := outbound.ReferenceContext{
					ID: entry.ID, Kind: progress.KindVocab, Headword: entry.Headword, Reading: entry.Reading,
					Gloss: first(entry.Gloss),
				}
				if entry.Example != nil {
					context.Example = entry.Example.Text
					context.ExampleMeaning = entry.Example.Translation
				}
				result[string(progress.KindVocab)+"#"+entry.ID] = context
			case len(sk.Value) > len("GRAMMAR#") && sk.Value[:len("GRAMMAR#")] == "GRAMMAR#":
				var record grammarItem
				if err := attributevalue.UnmarshalMap(item, &record); err != nil {
					return fmt.Errorf("%w: unmarshal grammar context: %v", reference.ErrStorageFailure, err)
				}
				entry := record.toDomain()
				context := outbound.ReferenceContext{
					ID: entry.ID, Kind: progress.KindGrammar, Headword: entry.Name, Gloss: entry.Description,
				}
				if entry.Example != nil {
					context.Example = entry.Example.Text
					context.ExampleMeaning = entry.Example.Translation
				}
				result[string(progress.KindGrammar)+"#"+entry.ID] = context
			}
		}
		pending = out.UnprocessedKeys[r.table].Keys
	}
	if len(pending) > 0 {
		return fmt.Errorf("%w: reference batch remained unprocessed", reference.ErrStorageFailure)
	}
	return nil
}

func referenceKey(language, sk string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "REF#" + language},
		"SK": &types.AttributeValueMemberS{Value: sk},
	}
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
