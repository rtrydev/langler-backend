package machineauth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/rtrydev/langler-backend/internal/adapters/inbound/machineauth"
	"github.com/rtrydev/langler-backend/internal/domain/agenttoken"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

type fakeAuthorizer struct {
	result inbound.MachineAuthorization
	err    error
	secret string
	scope  agenttoken.Scope
}

func (f *fakeAuthorizer) Authorize(_ context.Context, secret string, scope agenttoken.Scope) (inbound.MachineAuthorization, error) {
	f.secret, f.scope = secret, scope
	return f.result, f.err
}

func TestHandlerReturnsOwnerContext(t *testing.T) {
	fake := &fakeAuthorizer{result: inbound.MachineAuthorization{Owner: "user-1", TokenID: "token-1"}}
	handler, err := machineauth.NewHandler(fake)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	response, err := handler.Handle(context.Background(), events.APIGatewayV2CustomAuthorizerV2Request{
		RouteKey: "POST /lessons/import", Headers: map[string]string{"authorization": "Bearer lang_sk_secret"},
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !response.IsAuthorized || response.Context["owner"] != "user-1" || fake.secret != "lang_sk_secret" || fake.scope != agenttoken.ScopeImportLessons {
		t.Errorf("response = %+v secret = %q scope = %q", response, fake.secret, fake.scope)
	}
}

func TestHandlerDeniesInvalidTokenWithoutReturningLambdaError(t *testing.T) {
	handler, err := machineauth.NewHandler(&fakeAuthorizer{err: errors.New("denied")})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	response, err := handler.Handle(context.Background(), events.APIGatewayV2CustomAuthorizerV2Request{})
	if err != nil || response.IsAuthorized {
		t.Fatalf("response = %+v error = %v", response, err)
	}
}

func TestHandlerDeniesMalformedAuthorizationAndUnknownRoutes(t *testing.T) {
	tests := []events.APIGatewayV2CustomAuthorizerV2Request{
		{RouteKey: "GET /reference/vocab", Headers: map[string]string{"Authorization": "lang_sk_secret"}},
		{RouteKey: "DELETE /lessons/lesson-1", Headers: map[string]string{"Authorization": "Bearer lang_sk_secret"}},
	}
	for _, request := range tests {
		handler, err := machineauth.NewHandler(&fakeAuthorizer{})
		if err != nil {
			t.Fatalf("NewHandler: %v", err)
		}
		response, err := handler.Handle(context.Background(), request)
		if err != nil || response.IsAuthorized {
			t.Fatalf("request = %+v response = %+v error = %v", request, response, err)
		}
	}
}
