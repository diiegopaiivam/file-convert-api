package ports

import (
	"context"
)

// JobQueue abstrai o enfileiramento e consumo de jobs.
type JobQueue interface {
	// Enqueue envia o ID do job para a fila de processamento assíncrono.
	Enqueue(ctx context.Context, jobID string) error

	// Dequeue retira um jobID da fila para ser processado pelo worker.
	// Retorna uma função de ack para confirmar o sucesso do processamento.
	Dequeue(ctx context.Context) (jobID string, ack func() error, err error)
}
