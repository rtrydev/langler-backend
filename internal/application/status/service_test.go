package status_test

import (
	"context"
	"testing"

	"github.com/rtrydev/langler-backend/internal/application/status"
)

func TestNewService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		service string
		stage   string
		wantErr bool
	}{
		{name: "valid", service: "langler-backend", stage: "dev"},
		{name: "empty stage allowed", service: "langler-backend"},
		{name: "empty service rejected", service: "", stage: "dev", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := status.NewService(tt.service, tt.stage)
			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Fatalf("NewService(%q, %q) error = %v, wantErr %v", tt.service, tt.stage, err, tt.wantErr)
			}
		})
	}
}

func TestServiceStatus(t *testing.T) {
	t.Parallel()

	svc, err := status.NewService("langler-backend", "dev")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got.Service != "langler-backend" {
		t.Errorf("Service = %q, want %q", got.Service, "langler-backend")
	}
	if got.Stage != "dev" {
		t.Errorf("Stage = %q, want %q", got.Stage, "dev")
	}
	if got.Message == "" {
		t.Error("Message is empty")
	}
}
