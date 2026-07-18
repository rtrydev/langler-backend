package inbound

import "context"

type Status struct {
	Message string
	Service string
	Stage   string
}

type StatusProvider interface {
	Status(ctx context.Context) (Status, error)
}
