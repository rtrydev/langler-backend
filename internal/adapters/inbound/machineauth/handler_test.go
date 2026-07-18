package machineauth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/rtrydev/langler-backend/internal/adapters/inbound/machineauth"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

type fakeAuthorizer struct {
	result inbound.MachineAuthorization
	err    error
	header string
	route  string
}

func (f *fakeAuthorizer) Authorize(_ context.Context, header, route string) (inbound.MachineAuthorization, error) {
	f.header, f.route = header, route
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
	if !response.IsAuthorized || response.Context["owner"] != "user-1" || fake.header != "Bearer lang_sk_secret" {
		t.Errorf("response = %+v header = %q", response, fake.header)
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
