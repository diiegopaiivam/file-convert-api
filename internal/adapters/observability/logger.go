package observability

import (
	"context"
	"log/slog"
	"os"

	"github.com/file-convert-api/internal/ports"
)

var _ ports.Logger = (*JSONLogger)(nil)

// JSONLogger implementa o port ports.Logger gerando logs estruturados em JSON para stdout.
type JSONLogger struct {
	handler slog.Handler
	logger  *slog.Logger
	fields  []ports.Field
}

func NewJSONLogger() *JSONLogger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	return &JSONLogger{
		handler: handler,
		logger:  slog.New(handler),
		fields:  nil,
	}
}

func (l *JSONLogger) With(fields ...ports.Field) ports.Logger {
	newFields := make([]ports.Field, 0, len(l.fields)+len(fields))
	newFields = append(newFields, l.fields...)
	newFields = append(newFields, fields...)
	return &JSONLogger{
		handler: l.handler,
		logger:  l.logger,
		fields:  newFields,
	}
}

func (l *JSONLogger) buildAttrs(ctx context.Context, fields []ports.Field) []slog.Attr {
	attrs := make([]slog.Attr, 0, len(l.fields)+len(fields)+2)

	// Extração automática do job_id e correlation_id a partir do context
	if jobID, ok := ports.JobIDFromContext(ctx); ok && jobID != "" {
		attrs = append(attrs, slog.String("job_id", jobID))
	}
	if correlationID, ok := ports.CorrelationIDFromContext(ctx); ok && correlationID != "" {
		attrs = append(attrs, slog.String("correlation_id", correlationID))
	}

	for _, f := range l.fields {
		attrs = append(attrs, slog.Any(f.Key, f.Value))
	}
	for _, f := range fields {
		attrs = append(attrs, slog.Any(f.Key, f.Value))
	}
	return attrs
}

func (l *JSONLogger) Debug(ctx context.Context, msg string, fields ...ports.Field) {
	attrs := l.buildAttrs(ctx, fields)
	l.logger.LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
}

func (l *JSONLogger) Info(ctx context.Context, msg string, fields ...ports.Field) {
	attrs := l.buildAttrs(ctx, fields)
	l.logger.LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
}

func (l *JSONLogger) Warn(ctx context.Context, msg string, fields ...ports.Field) {
	attrs := l.buildAttrs(ctx, fields)
	l.logger.LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
}

func (l *JSONLogger) Error(ctx context.Context, msg string, fields ...ports.Field) {
	attrs := l.buildAttrs(ctx, fields)
	l.logger.LogAttrs(ctx, slog.LevelError, msg, attrs...)
}
