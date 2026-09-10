package ports

import (
	"context"
)

type contextKey string

const (
	JobIDContextKey         contextKey = "job_id"
	CorrelationIDContextKey contextKey = "correlation_id"
)

// WithJobID injeta o job_id no contexto de execução.
func WithJobID(ctx context.Context, jobID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, JobIDContextKey, jobID)
}

// JobIDFromContext recupera o job_id do contexto de execução, se presente.
func JobIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	id, ok := ctx.Value(JobIDContextKey).(string)
	return id, ok
}

// WithCorrelationID injeta o correlation_id no contexto de execução.
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, CorrelationIDContextKey, correlationID)
}

// CorrelationIDFromContext recupera o correlation_id do contexto de execução, se presente.
func CorrelationIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	id, ok := ctx.Value(CorrelationIDContextKey).(string)
	return id, ok
}
