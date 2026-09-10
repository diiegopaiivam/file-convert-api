package repository

import (
	"context"
	"sync"
	"time"

	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
)

var _ ports.JobRepository = (*MemoryJobRepository)(nil)

type MemoryJobRepository struct {
	mu   sync.RWMutex
	jobs map[string]*domain.Job
}

func NewMemoryJobRepository() *MemoryJobRepository {
	return &MemoryJobRepository{
		jobs: make(map[string]*domain.Job),
	}
}

func (r *MemoryJobRepository) Create(ctx context.Context, job *domain.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs[job.ID] = job
	return nil
}

func (r *MemoryJobRepository) GetByID(ctx context.Context, id string) (*domain.Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	job, ok := r.jobs[id]
	if !ok {
		return nil, nil
	}
	return job, nil
}

func (r *MemoryJobRepository) UpdateStatus(ctx context.Context, id string, status domain.JobStatus, targetKey string, jobErr *domain.JobError) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return domain.ErrJobNotFound
	}
	job.Status = status
	if targetKey != "" {
		job.TargetObjectKey = targetKey
	}
	if jobErr != nil {
		job.Error = jobErr
	}
	job.UpdatedAt = time.Now().UTC()
	return nil
}
