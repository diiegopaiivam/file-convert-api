package queue

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/file-convert-api/internal/adapters/observability"
	"github.com/file-convert-api/internal/ports"
)

var _ ports.JobQueue = (*RedisJobQueue)(nil)

type RedisJobQueue struct {
	addr    string
	key     string
	metrics ports.MetricsRecorder
	mu      sync.Mutex
}

func NewRedisJobQueue(addr string) *RedisJobQueue {
	if addr == "" {
		addr = "redis:6379"
	}
	return &RedisJobQueue{
		addr:    addr,
		key:     "fileconvert:queue",
		metrics: observability.NewNoOpMetricsRecorder(),
	}
}

func (q *RedisJobQueue) WithMetrics(metrics ports.MetricsRecorder) *RedisJobQueue {
	if metrics != nil {
		q.metrics = metrics
	}
	return q
}

func (q *RedisJobQueue) sendCommand(cmd ...string) (string, error) {
	conn, err := net.DialTimeout("tcp", q.addr, 3*time.Second)
	if err != nil {
		return "", fmt.Errorf("falha ao conectar ao Redis em %s: %w", q.addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	var req strings.Builder
	req.WriteString(fmt.Sprintf("*%d\r\n", len(cmd)))
	for _, arg := range cmd {
		req.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(arg), arg))
	}

	if _, err := conn.Write([]byte(req.String())); err != nil {
		return "", err
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "-") {
		return "", fmt.Errorf("erro redis: %s", line[1:])
	}

	if strings.HasPrefix(line, "+") || strings.HasPrefix(line, ":") {
		return line[1:], nil
	}

	if strings.HasPrefix(line, "$") {
		length, _ := strconv.Atoi(line[1:])
		if length == -1 {
			return "", nil
		}
		valBuf := make([]byte, length+2)
		if _, err := io.ReadFull(reader, valBuf); err != nil {
			return "", err
		}
		return string(valBuf[:length]), nil
	}

	return line, nil
}

func (q *RedisJobQueue) Enqueue(ctx context.Context, jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	_, err := q.sendCommand("RPUSH", q.key, jobID)
	if err != nil {
		return err
	}

	depthStr, _ := q.sendCommand("LLEN", q.key)
	if depth, err := strconv.ParseInt(depthStr, 10, 64); err == nil {
		q.metrics.RecordQueueDepth(ctx, "conversion_jobs", depth)
	}

	return nil
}

func (q *RedisJobQueue) Dequeue(ctx context.Context) (string, func() error, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	depthStr, _ := q.sendCommand("LLEN", q.key)
	if depth, err := strconv.ParseInt(depthStr, 10, 64); err == nil {
		q.metrics.RecordQueueDepth(ctx, "conversion_jobs", depth)
	}

	jobID, err := q.sendCommand("LPOP", q.key)
	if err != nil {
		return "", nil, err
	}

	if jobID == "" {
		return "", nil, fmt.Errorf("fila vazia")
	}

	ack := func() error { return nil }
	return jobID, ack, nil
}
