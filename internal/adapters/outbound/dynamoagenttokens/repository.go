package dynamoagenttokens

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/rtrydev/langler-backend/internal/domain/agenttoken"
)

type Repository struct {
	client *dynamodb.Client
	table  string
}

type tokenItem struct {
	PK        string   `dynamodbav:"PK"`
	SK        string   `dynamodbav:"SK"`
	TokenID   string   `dynamodbav:"tokenId"`
	Owner     string   `dynamodbav:"owner"`
	Label     string   `dynamodbav:"label"`
	Scopes    []string `dynamodbav:"scopes"`
	CreatedAt string   `dynamodbav:"createdAt"`
	ExpiresAt string   `dynamodbav:"expiresAt"`
	RevokedAt string   `dynamodbav:"revokedAt,omitempty"`
	LastUsed  string   `dynamodbav:"lastUsed,omitempty"`
	Suffix    string   `dynamodbav:"suffix"`
	TokenHash string   `dynamodbav:"tokenHash"`
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

func (r *Repository) Create(ctx context.Context, token agenttoken.Token, hash string) error {
	ownerItem, err := attributevalue.MarshalMap(toItem(token, hash, false))
	if err != nil {
		return fmt.Errorf("%w: marshal owner token: %v", agenttoken.ErrStorage, err)
	}
	lookupItem, err := attributevalue.MarshalMap(toItem(token, hash, true))
	if err != nil {
		return fmt.Errorf("%w: marshal token lookup: %v", agenttoken.ErrStorage, err)
	}
	_, err = r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{Put: &types.Put{TableName: aws.String(r.table), Item: ownerItem, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
		{Put: &types.Put{TableName: aws.String(r.table), Item: lookupItem, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
	}})
	if err != nil {
		var cancelled *types.TransactionCanceledException
		if errors.As(err, &cancelled) {
			return agenttoken.ErrAlreadyExists
		}
		return fmt.Errorf("%w: create token: %v", agenttoken.ErrStorage, err)
	}
	return nil
}

func (r *Repository) List(ctx context.Context, owner string) ([]agenttoken.Token, error) {
	out, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "USER#" + owner},
			":sk": &types.AttributeValueMemberS{Value: "AGENTTOKEN#"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: list tokens: %v", agenttoken.ErrStorage, err)
	}
	var items []tokenItem
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &items); err != nil {
		return nil, fmt.Errorf("%w: unmarshal tokens: %v", agenttoken.ErrStorage, err)
	}
	tokens := make([]agenttoken.Token, 0, len(items))
	for _, item := range items {
		token, err := item.toDomain()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	slices.SortFunc(tokens, func(a, b agenttoken.Token) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return tokens, nil
}

func (r *Repository) Revoke(ctx context.Context, owner, id string, at time.Time) error {
	item, err := r.ownerItem(ctx, owner, id)
	if err != nil {
		return err
	}
	values := map[string]types.AttributeValue{":revoked": &types.AttributeValueMemberS{Value: at.UTC().Format(time.RFC3339Nano)}}
	_, err = r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{Update: &types.Update{TableName: aws.String(r.table), Key: ownerKey(owner, id), UpdateExpression: aws.String("SET revokedAt = :revoked"), ExpressionAttributeValues: values}},
		{Update: &types.Update{TableName: aws.String(r.table), Key: lookupKey(item.TokenHash), UpdateExpression: aws.String("SET revokedAt = :revoked"), ExpressionAttributeValues: values}},
	}})
	if err != nil {
		return fmt.Errorf("%w: revoke token: %v", agenttoken.ErrStorage, err)
	}
	return nil
}

func (r *Repository) FindByHash(ctx context.Context, hash string) (agenttoken.Token, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(r.table), Key: lookupKey(hash), ConsistentRead: aws.Bool(true)})
	if err != nil {
		return agenttoken.Token{}, fmt.Errorf("%w: find token: %v", agenttoken.ErrStorage, err)
	}
	if out.Item == nil {
		return agenttoken.Token{}, agenttoken.ErrNotFound
	}
	var item tokenItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return agenttoken.Token{}, fmt.Errorf("%w: unmarshal token: %v", agenttoken.ErrStorage, err)
	}
	return item.toDomain()
}

func (r *Repository) Touch(ctx context.Context, token agenttoken.Token, at time.Time) error {
	values := map[string]types.AttributeValue{":used": &types.AttributeValueMemberS{Value: at.UTC().Format(time.RFC3339Nano)}}
	_, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.table), Key: ownerKey(token.Owner, token.ID),
		UpdateExpression: aws.String("SET lastUsed = :used"), ExpressionAttributeValues: values,
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})
	if err != nil {
		return fmt.Errorf("%w: update last used: %v", agenttoken.ErrStorage, err)
	}
	return nil
}

func (r *Repository) Consume(ctx context.Context, tokenID string, window time.Time, limit int) error {
	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "RATELIMIT#" + tokenID},
		"SK": &types.AttributeValueMemberS{Value: window.UTC().Format("20060102T1504")},
	}
	_, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(r.table),
		Key:                 key,
		UpdateExpression:    aws.String("SET expiresAtUnix = :expires ADD requestCount :one"),
		ConditionExpression: aws.String("attribute_not_exists(requestCount) OR requestCount < :limit"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":one":     &types.AttributeValueMemberN{Value: "1"},
			":limit":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", limit)},
			":expires": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", window.Add(2*time.Minute).Unix())},
		},
	})
	if err != nil {
		var conditionFailed *types.ConditionalCheckFailedException
		if errors.As(err, &conditionFailed) {
			return agenttoken.ErrRateLimited
		}
		return fmt.Errorf("%w: consume rate limit: %v", agenttoken.ErrStorage, err)
	}
	return nil
}

func (r *Repository) ownerItem(ctx context.Context, owner, id string) (tokenItem, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(r.table), Key: ownerKey(owner, id), ConsistentRead: aws.Bool(true)})
	if err != nil {
		return tokenItem{}, fmt.Errorf("%w: get token: %v", agenttoken.ErrStorage, err)
	}
	if out.Item == nil {
		return tokenItem{}, agenttoken.ErrNotFound
	}
	var item tokenItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return tokenItem{}, fmt.Errorf("%w: unmarshal token: %v", agenttoken.ErrStorage, err)
	}
	return item, nil
}

func toItem(token agenttoken.Token, hash string, lookup bool) tokenItem {
	pk, sk := "USER#"+token.Owner, "AGENTTOKEN#"+token.ID
	if lookup {
		pk, sk = "AGENTTOKENHASH#"+hash, "AGENTTOKEN"
	}
	scopes := make([]string, 0, len(token.Scopes))
	for _, scope := range token.Scopes {
		scopes = append(scopes, string(scope))
	}
	return tokenItem{PK: pk, SK: sk, TokenID: token.ID, Owner: token.Owner, Label: token.Label, Scopes: scopes, CreatedAt: token.CreatedAt.Format(time.RFC3339Nano), ExpiresAt: token.ExpiresAt.Format(time.RFC3339Nano), Suffix: token.Suffix, TokenHash: hash}
}

func (item tokenItem) toDomain() (agenttoken.Token, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, item.CreatedAt)
	if err != nil {
		return agenttoken.Token{}, fmt.Errorf("%w: invalid createdAt", agenttoken.ErrStorage)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, item.ExpiresAt)
	if err != nil {
		return agenttoken.Token{}, fmt.Errorf("%w: invalid expiresAt", agenttoken.ErrStorage)
	}
	scopes := make([]agenttoken.Scope, 0, len(item.Scopes))
	for _, scope := range item.Scopes {
		scopes = append(scopes, agenttoken.Scope(scope))
	}
	token, err := agenttoken.New(item.TokenID, item.Owner, item.Label, item.Suffix, scopes, createdAt, expiresAt)
	if err != nil {
		return agenttoken.Token{}, fmt.Errorf("%w: invalid stored token", agenttoken.ErrStorage)
	}
	if item.RevokedAt != "" {
		token.RevokedAt, err = time.Parse(time.RFC3339Nano, item.RevokedAt)
		if err != nil {
			return agenttoken.Token{}, fmt.Errorf("%w: invalid revokedAt", agenttoken.ErrStorage)
		}
	}
	if item.LastUsed != "" {
		token.LastUsed, err = time.Parse(time.RFC3339Nano, item.LastUsed)
		if err != nil {
			return agenttoken.Token{}, fmt.Errorf("%w: invalid lastUsed", agenttoken.ErrStorage)
		}
	}
	return token, nil
}

func ownerKey(owner, id string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"PK": &types.AttributeValueMemberS{Value: "USER#" + owner}, "SK": &types.AttributeValueMemberS{Value: "AGENTTOKEN#" + id}}
}

func lookupKey(hash string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"PK": &types.AttributeValueMemberS{Value: "AGENTTOKENHASH#" + hash}, "SK": &types.AttributeValueMemberS{Value: "AGENTTOKEN"}}
}
