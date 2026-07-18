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

type fakeReferenceProvider struct {
	vocab   inbound.VocabResult
	grammar inbound.GrammarResult
	scripts inbound.ScriptResult
	err     error

	vocabQuery   inbound.VocabQuery
	grammarQuery inbound.GrammarQuery
	scriptQuery  inbound.ScriptQuery
}

func (f *fakeReferenceProvider) Vocab(_ context.Context, query inbound.VocabQuery) (inbound.VocabResult, error) {
	f.vocabQuery = query
	return f.vocab, f.err
}

func (f *fakeReferenceProvider) Grammar(_ context.Context, query inbound.GrammarQuery) (inbound.GrammarResult, error) {
	f.grammarQuery = query
	return f.grammar, f.err
}

func (f *fakeReferenceProvider) Scripts(_ context.Context, query inbound.ScriptQuery) (inbound.ScriptResult, error) {
	f.scriptQuery = query
	return f.scripts, f.err
}

func getRequest(path string, params map[string]string) events.APIGatewayV2HTTPRequest {
	req := events.APIGatewayV2HTTPRequest{
		RawPath:               path,
		QueryStringParameters: params,
	}
	req.RequestContext.HTTP.Method = http.MethodGet
	return req
}

func newHandler(t *testing.T, status fakeStatusProvider, reference *fakeReferenceProvider) *httpapi.Handler {
	t.Helper()

	h, err := httpapi.NewHandler(status, reference)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func TestHandleStatus(t *testing.T) {
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

			h := newHandler(t, tt.provider, &fakeReferenceProvider{})
			resp, err := h.Handle(context.Background(), getRequest("/hello", nil))
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

func TestHandleRouting(t *testing.T) {
	t.Parallel()

	t.Run("unknown path", func(t *testing.T) {
		t.Parallel()

		h := newHandler(t, fakeStatusProvider{}, &fakeReferenceProvider{})
		resp, err := h.Handle(context.Background(), getRequest("/nope", nil))
		if err != nil {
			t.Fatalf("Handle: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
	})

	t.Run("wrong method", func(t *testing.T) {
		t.Parallel()

		h := newHandler(t, fakeStatusProvider{}, &fakeReferenceProvider{})
		req := getRequest("/reference/vocab", nil)
		req.RequestContext.HTTP.Method = http.MethodPost
		resp, err := h.Handle(context.Background(), req)
		if err != nil {
			t.Fatalf("Handle: %v", err)
		}
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
		}
	})
}

func TestNewHandlerRejectsNilDependencies(t *testing.T) {
	t.Parallel()

	if _, err := httpapi.NewHandler(nil, &fakeReferenceProvider{}); err == nil {
		t.Fatal("NewHandler(nil status) error = nil")
	}
	if _, err := httpapi.NewHandler(fakeStatusProvider{}, nil); err == nil {
		t.Fatal("NewHandler(nil reference) error = nil")
	}
}
