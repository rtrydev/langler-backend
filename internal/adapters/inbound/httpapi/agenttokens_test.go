package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/rtrydev/langler-backend/internal/adapters/inbound/httpapi"
	"github.com/rtrydev/langler-backend/internal/domain/agenttoken"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

type fakeAgentTokenManager struct {
	created inbound.AgentTokenCreated
	tokens  []agenttoken.Token
	command inbound.AgentTokenCreateCommand
	owner   string
	id      string
}

func (f *fakeAgentTokenManager) Create(_ context.Context, command inbound.AgentTokenCreateCommand) (inbound.AgentTokenCreated, error) {
	f.command = command
	return f.created, nil
}

func (f *fakeAgentTokenManager) List(_ context.Context, owner string) ([]agenttoken.Token, error) {
	f.owner = owner
	return f.tokens, nil
}

func (f *fakeAgentTokenManager) Revoke(_ context.Context, owner, id string) error {
	f.owner, f.id = owner, id
	return nil
}

func tokenRequest(method, path, owner, body string) events.APIGatewayV2HTTPRequest {
	req := events.APIGatewayV2HTTPRequest{RawPath: path, Body: body}
	req.RequestContext.HTTP.Method = method
	if owner != "" {
		req.RequestContext.Authorizer = &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
			JWT: &events.APIGatewayV2HTTPRequestContextAuthorizerJWTDescription{Claims: map[string]string{"sub": owner}},
		}
	}
	return req
}

func tokenHandler(t *testing.T, manager *fakeAgentTokenManager) *httpapi.Handler {
	t.Helper()
	handler, err := httpapi.NewHandler(fakeStatusProvider{}, &fakeReferenceProvider{}, &fakeLessonImporter{}, &fakeLessonLibrary{}, &fakeLessonPromptBuilder{}, &fakeLessonTopicAdvisor{}, &fakeLessonResultRecorder{}, &fakeProgressProvider{}, manager, &fakeAssessmentProvider{})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return handler
}

func TestAgentTokenLifecycleRoutes(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	token, err := agenttoken.New("token-1", "user-1", "Claude Code", "abcd", []agenttoken.Scope{agenttoken.ScopeReadReference, agenttoken.ScopeImportLessons}, now, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	manager := &fakeAgentTokenManager{created: inbound.AgentTokenCreated{Token: token, Secret: "lang_sk_secret"}, tokens: []agenttoken.Token{token}}
	handler := tokenHandler(t, manager)

	createResponse, err := handler.Handle(context.Background(), tokenRequest(http.MethodPost, "/agent-tokens", "user-1", `{"label":"Claude Code","scopes":["read-reference","import-lessons"],"expiresAt":"2026-12-01T00:00:00Z"}`))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if createResponse.StatusCode != http.StatusCreated || manager.command.Owner != "user-1" {
		t.Fatalf("create response = %+v command = %+v", createResponse, manager.command)
	}
	var created map[string]any
	if err := json.Unmarshal([]byte(createResponse.Body), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if created["secret"] != "lang_sk_secret" {
		t.Errorf("body = %v", created)
	}

	listResponse, _ := handler.Handle(context.Background(), tokenRequest(http.MethodGet, "/agent-tokens", "user-1", ""))
	if listResponse.StatusCode != http.StatusOK || manager.owner != "user-1" {
		t.Fatalf("list response = %+v owner = %q", listResponse, manager.owner)
	}
	if listResponse.Body == "" || strings.Contains(listResponse.Body, "lang_sk_secret") {
		t.Errorf("list body = %s", listResponse.Body)
	}

	revokeResponse, _ := handler.Handle(context.Background(), tokenRequest(http.MethodDelete, "/agent-tokens/token-1", "user-1", ""))
	if revokeResponse.StatusCode != http.StatusNoContent || manager.id != "token-1" {
		t.Fatalf("revoke response = %+v id = %q", revokeResponse, manager.id)
	}
}

func TestMachineAuthorizerOwnerIsAcceptedForImport(t *testing.T) {
	req := lessonRequest(http.MethodPost, "/lessons/import", "", validLessonBody)
	req.RequestContext.Authorizer = &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{Lambda: map[string]interface{}{"owner": "machine-owner"}}
	importer := &fakeLessonImporter{}
	handler := newLessonHandler(t, importer, &fakeLessonLibrary{}, &fakeLessonPromptBuilder{})
	response, _ := handler.Handle(context.Background(), req)
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		t.Fatalf("response = %+v", response)
	}
	if importer.command.Owner != "machine-owner" {
		t.Errorf("Owner = %q", importer.command.Owner)
	}
}
