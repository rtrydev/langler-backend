package status

import (
	"context"
	"errors"

	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

type Service struct {
	service string
	stage   string
}

func NewService(service, stage string) (*Service, error) {
	if service == "" {
		return nil, errors.New("service name must not be empty")
	}
	return &Service{service: service, stage: stage}, nil
}

func (s *Service) Status(_ context.Context) (inbound.Status, error) {
	return inbound.Status{
		Message: "Hello from Langler",
		Service: s.service,
		Stage:   s.stage,
	}, nil
}
