package dynamoref

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	domain "github.com/rtrydev/langler-backend/internal/domain/reference"
)

const batchGetLimit = 100

func (r *Repository) MissingVocab(ctx context.Context, language domain.Language, ids []string) ([]string, error) {
	return r.missing(ctx, language, "VOCAB#", ids)
}

func (r *Repository) MissingGrammar(ctx context.Context, language domain.Language, ids []string) ([]string, error) {
	return r.missing(ctx, language, "GRAMMAR#", ids)
}

func (r *Repository) missing(ctx context.Context, language domain.Language, prefix string, ids []string) ([]string, error) {
	partition := "REF#" + string(language)
	found := make(map[string]bool, len(ids))

	for chunk := range slices.Chunk(ids, batchGetLimit) {
		keys := make([]map[string]types.AttributeValue, 0, len(chunk))
		for _, id := range chunk {
			keys = append(keys, map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: partition},
				"SK": &types.AttributeValueMemberS{Value: prefix + id},
			})
		}

		request := map[string]types.KeysAndAttributes{
			r.table: {Keys: keys, ProjectionExpression: aws.String("SK")},
		}
		for len(request) > 0 {
			out, err := r.client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{RequestItems: request})
			if err != nil {
				return nil, fmt.Errorf("%w: batch get %s: %v", domain.ErrStorageFailure, prefix, err)
			}
			for _, item := range out.Responses[r.table] {
				if sk, ok := item["SK"].(*types.AttributeValueMemberS); ok {
					found[strings.TrimPrefix(sk.Value, prefix)] = true
				}
			}
			request = out.UnprocessedKeys
		}
	}

	var missing []string
	for _, id := range ids {
		if !found[id] {
			missing = append(missing, id)
		}
	}
	return missing, nil
}
