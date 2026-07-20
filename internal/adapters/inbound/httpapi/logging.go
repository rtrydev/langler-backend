package httpapi

import (
	"context"
	"log/slog"
)

type contextKey int

const loggerContextKey contextKey = 0

func withLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey, logger)
}

func loggerFrom(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerContextKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
