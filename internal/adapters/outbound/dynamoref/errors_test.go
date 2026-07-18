package dynamoref_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoref"
	domain "github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

func TestQueryFailureTranslatesToDomainError(t *testing.T) {
	t.Parallel()

	client := dynamodb.New(dynamodb.Options{
		Region:           "us-east-1",
		Credentials:      credentials.NewStaticCredentialsProvider("local", "local", ""),
		BaseEndpoint:     aws.String("http://127.0.0.1:1"),
		RetryMaxAttempts: 1,
	})
	repo, err := dynamoref.NewRepository(client, "unreachable")
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}

	_, err = repo.Vocab(context.Background(), outbound.VocabFilter{Language: "ja", Limit: 1})
	if !errors.Is(err, domain.ErrStorageFailure) {
		t.Fatalf("error = %v, want %v", err, domain.ErrStorageFailure)
	}
}
