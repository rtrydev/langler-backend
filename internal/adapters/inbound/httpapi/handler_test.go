package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/rtrydev/langler-backend/internal/adapters/inbound/httpapi"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

type fakeStatusProvider struct {
	status inbound.Status
	err    error
}

func (f fakeStatusProvider) Status(context.Context) (inbound.Status, error) {
	return f.status, f.err
}

func TestHandlerHandle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		provider   fakeStatusProvider
		wantStatus int
		wantBody   map[string]string
	}{
		{
			name: "success",
			provider: fakeStatusProvider{status: inbound.Status{
				Message: "Hello from Langler",
				Service: "langler-backend",
				Stage:   "dev",
			}},
			wantStatus: http.StatusOK,
			wantBody: map[string]string{
				"message": "Hello from Langler",
				"service": "langler-backend",
				"stage":   "dev",
			},
		},
		{
			name:       "provider failure",
			provider:   fakeStatusProvider{err: errors.New("boom")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, err := httpapi.NewHandler(tt.provider)
			if err != nil {
				t.Fatalf("NewHandler: %v", err)
			}
			resp, err := h.Handle(context.Background(), events.APIGatewayV2HTTPRequest{})
			if err != nil {
				t.Fatalf("Handle: %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if tt.wantBody == nil {
				return
			}
			if ct := resp.Headers["Content-Type"]; ct != "application/json" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/json")
			}
			var got map[string]string
			if err := json.Unmarshal([]byte(resp.Body), &got); err != nil {
				t.Fatalf("unmarshal body: %v", err)
			}
			for k, want := range tt.wantBody {
				if got[k] != want {
					t.Errorf("body[%q] = %q, want %q", k, got[k], want)
				}
			}
		})
	}
}

func TestNewHandlerRejectsNilStatusProvider(t *testing.T) {
	t.Parallel()

	if _, err := httpapi.NewHandler(nil); err == nil {
		t.Fatal("NewHandler(nil) error = nil")
	}
}
