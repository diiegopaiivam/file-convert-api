package queue

import (
	"context"
	"errors"

	"github.com/file-convert-api/internal/adapters/observability"
	"github.com/file-convert-api/internal/ports"
)

var _ ports.JobQueue = (*ChannelJobQueue)(nil)

type ChannelJobQueue struct {
	ch      chan string
	metrics ports.MetricsRecorder
}

func NewChannelJobQueue(bufferSize int) *ChannelJobQueue {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	return &ChannelJobQueue{
		ch:      make(chan string, bufferSize),
		metrics: observability.NewNoOpMetricsRecorder(),
	}
}

func (q *ChannelJobQueue) WithMetrics(metrics ports.MetricsRecorder) *ChannelJobQueue {
	if metrics != nil {
		q.metrics = metrics
	}
	return q
}

func (q *ChannelJobQueue) Enqueue(ctx context.Context, jobID string) error {
	select {
	case q.ch <- jobID:
		q.metrics.RecordQueueDepth(ctx, "conversion_jobs", int64(len(q.ch)))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return errors.New("fila cheia")
	}
}

func (q *ChannelJobQueue) Dequeue(ctx context.Context) (string, func() error, error) {
	select {
	case jobID := <-q.ch:
		q.metrics.RecordQueueDepth(ctx, "conversion_jobs", int64(len(q.ch)))
		ack := func() error { return nil }
		return jobID, ack, nil
	case <-ctx.Done():
		return "", nil, ctx.Err()
	}
}
