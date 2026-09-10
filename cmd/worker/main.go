package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/file-convert-api/internal/adapters/converter/audio"
	"github.com/file-convert-api/internal/adapters/observability"
	"github.com/file-convert-api/internal/adapters/queue"
	"github.com/file-convert-api/internal/adapters/repository"
	"github.com/file-convert-api/internal/adapters/storage"
	"github.com/file-convert-api/internal/ports"
	"github.com/file-convert-api/internal/usecase"
)

func main() {
	// Inicialização da Observabilidade no Worker
	logger := observability.NewJSONLogger()
	metrics := observability.NewPrometheusMetricsRecorder()

	storagePath := os.Getenv("STORAGE_PATH")
	if storagePath == "" {
		storagePath = "/tmp/file-convert-storage"
	}

	storageAdapter, err := storage.NewLocalStorage(storagePath)
	if err != nil {
		log.Fatalf("Falha ao inicializar storage no worker: %v", err)
	}

	redisAddr := os.Getenv("REDIS_ADDR")

	var repoAdapter ports.JobRepository
	var queueAdapter ports.JobQueue

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if redisAddr != "" {
		logger.Info(ctx, "Worker conectando ao repositório e fila compartilhada via Redis", ports.Field{Key: "redis_addr", Value: redisAddr})
		repoAdapter = repository.NewRedisJobRepository(redisAddr)
		queueAdapter = queue.NewRedisJobQueue(redisAddr).WithMetrics(metrics)
	} else {
		logger.Info(ctx, "Worker usando repositório e fila em memória (modo local)")
		repoAdapter = repository.NewMemoryJobRepository()
		queueAdapter = queue.NewChannelJobQueue(500).WithMetrics(metrics)
	}

	// Configurar Registro de Conversores
	registry := usecase.NewDefaultConverterRegistry()
	ffmpegConverter := audio.NewFFmpegAudioConverter("")
	if err := registry.Register(ffmpegConverter); err != nil {
		log.Fatalf("Falha ao registrar conversor de áudio FFmpeg: %v", err)
	}

	processJobUseCase := usecase.NewProcessJobUseCase(repoAdapter, storageAdapter, registry).
		WithLogger(logger).
		WithMetrics(metrics)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	logger.Info(ctx, "FileConvert Worker inicializado e pronto para consumo da fila compartilhada")

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				jobID, ack, err := queueAdapter.Dequeue(ctx)
				if err != nil {
					time.Sleep(200 * time.Millisecond)
					continue
				}

				jobCtx := ports.WithJobID(ctx, jobID)
				logger.Info(jobCtx, "Worker desenfileirou job para processamento")

				jobCtxWithTimeout, jobCancel := context.WithTimeout(jobCtx, 60*time.Second)
				err = processJobUseCase.Execute(jobCtxWithTimeout, jobID)
				jobCancel()

				if err != nil {
					logger.Error(jobCtx, "Worker finalizou processamento do job com falha", ports.Field{Key: "error", Value: err.Error()})
				} else {
					logger.Info(jobCtx, "Worker finalizou processamento do job com sucesso")
				}

				if ack != nil {
					_ = ack()
				}
			}
		}
	}()

	<-sigChan
	logger.Info(ctx, "Encerrando Worker com segurança...")
}
