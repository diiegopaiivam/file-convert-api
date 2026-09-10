package ports

import (
	"context"

	"github.com/file-convert-api/internal/domain"
)

// JobRepository abstrai a persistência e consulta dos jobs de conversão.
type JobRepository interface {
	// Create persiste um novo job no estado inicial 'pending'.
	Create(ctx context.Context, job *domain.Job) error

	// GetByID busca um job pelo seu ID único.
	GetByID(ctx context.Context, id string) (*domain.Job, error)

	// UpdateStatus atualiza o status do job, chave do objeto convertido e erros associados.
	UpdateStatus(ctx context.Context, id string, status domain.JobStatus, targetObjectKey string, jobErr *domain.JobError) error
}
