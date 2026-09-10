package observability

import (
	"context"
	"time"

	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
)

var _ ports.Logger = (*NoOpLogger)(nil)
var _ ports.MetricsRecorder = (*NoOpMetricsRecorder)(nil)
var _ ports.Tracer = (*NoOpTracer)(nil)

type NoOpLogger struct{}

func NewNoOpLogger() *NoOpLogger                                                     { return &NoOpLogger{} }
func (n *NoOpLogger) Debug(ctx context.Context, msg string, fields ...ports.Field) {}
func (n *NoOpLogger) Info(ctx context.Context, msg string, fields ...ports.Field)  {}
func (n *NoOpLogger) Warn(ctx context.Context, msg string, fields ...ports.Field)  {}
func (n *NoOpLogger) Error(ctx context.Context, msg string, fields ...ports.Field) {}
func (n *NoOpLogger) With(fields ...ports.Field) ports.Logger                      { return n }

type NoOpMetricsRecorder struct{}

func NewNoOpMetricsRecorder() *NoOpMetricsRecorder { return &NoOpMetricsRecorder{} }
func (n *NoOpMetricsRecorder) RecordConversionResult(ctx context.Context, src, target domain.Format, status domain.JobStatus) {
}
func (n *NoOpMetricsRecorder) RecordConversionDuration(ctx context.Context, src, target domain.Format, d time.Duration) {
}
func (n *NoOpMetricsRecorder) RecordJobDwellTime(ctx context.Context, src, target domain.Format, d time.Duration) {
}
func (n *NoOpMetricsRecorder) RecordQueueDepth(ctx context.Context, queueName string, depth int64) {
}
func (n *NoOpMetricsRecorder) RecordEdgeRequest(ctx context.Context, method, endpoint string, statusCode int, errorCode string) {
}

type NoOpTracer struct{}
type NoOpSpan struct{}

func NewNoOpTracer() *NoOpTracer { return &NoOpTracer{} }
func (t *NoOpTracer) StartSpan(ctx context.Context, op string) (context.Context, ports.Span) {
	return ctx, &NoOpSpan{}
}
func (s *NoOpSpan) End()                             {}
func (s *NoOpSpan) SetAttribute(key string, val any) {}
func (s *NoOpSpan) RecordError(err error)           {}
