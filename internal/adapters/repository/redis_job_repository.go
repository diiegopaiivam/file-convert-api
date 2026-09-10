package repository

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
)

var _ ports.JobRepository = (*RedisJobRepository)(nil)

type RedisJobRepository struct {
	addr string
	mu   sync.Mutex
}

func NewRedisJobRepository(addr string) *RedisJobRepository {
	if addr == "" {
		addr = "redis:6379"
	}
	return &RedisJobRepository{
		addr: addr,
	}
}

func (r *RedisJobRepository) sendCommand(cmd ...string) (string, error) {
	conn, err := net.DialTimeout("tcp", r.addr, 3*time.Second)
	if err != nil {
		return "", fmt.Errorf("falha ao conectar ao Redis em %s: %w", r.addr, err)
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

func (r *RedisJobRepository) key(id string) string {
	return "fileconvert:job:" + id
}

func (r *RedisJobRepository) Create(ctx context.Context, job *domain.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := json.Marshal(job)
	if err != nil {
		return err
	}

	_, err = r.sendCommand("SET", r.key(job.ID), string(data))
	return err
}

func (r *RedisJobRepository) GetByID(ctx context.Context, id string) (*domain.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	val, err := r.sendCommand("GET", r.key(id))
	if err != nil || val == "" {
		return nil, nil
	}

	var job domain.Job
	if err := json.Unmarshal([]byte(val), &job); err != nil {
		return nil, err
	}

	return &job, nil
}

func (r *RedisJobRepository) UpdateStatus(ctx context.Context, id string, status domain.JobStatus, targetKey string, jobErr *domain.JobError) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	val, err := r.sendCommand("GET", r.key(id))
	if err != nil || val == "" {
		return domain.ErrJobNotFound
	}

	var job domain.Job
	if err := json.Unmarshal([]byte(val), &job); err != nil {
		return err
	}

	job.Status = status
	if targetKey != "" {
		job.TargetObjectKey = targetKey
	}
	if jobErr != nil {
		job.Error = jobErr
	}
	job.UpdatedAt = time.Now().UTC()

	data, err := json.Marshal(job)
	if err != nil {
		return err
	}

	_, err = r.sendCommand("SET", r.key(id), string(data))
	return err
}
