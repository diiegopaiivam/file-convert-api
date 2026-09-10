package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/file-convert-api/internal/adapters/observability"
	"github.com/file-convert-api/internal/adapters/queue"
	"github.com/file-convert-api/internal/adapters/repository"
	"github.com/file-convert-api/internal/adapters/storage"
	apphttp "github.com/file-convert-api/internal/delivery/http"
	"github.com/file-convert-api/internal/ports"
)

func main() {
	// Inicialização dos adaptadores de Observabilidade
	logger := observability.NewJSONLogger()
	metrics := observability.NewPrometheusMetricsRecorder()

	storagePath := os.Getenv("STORAGE_PATH")
	if storagePath == "" {
		storagePath = "/tmp/file-convert-storage"
	}

	storageAdapter, err := storage.NewLocalStorage(storagePath)
	if err != nil {
		log.Fatalf("Falha ao inicializar storage: %v", err)
	}

	redisAddr := os.Getenv("REDIS_ADDR")

	var repoAdapter ports.JobRepository
	var queueAdapter ports.JobQueue

	ctx := context.Background()

	if redisAddr != "" {
		logger.Info(ctx, "Conectando ao repositório e fila compartilhada via Redis", ports.Field{Key: "redis_addr", Value: redisAddr})
		repoAdapter = repository.NewRedisJobRepository(redisAddr)
		queueAdapter = queue.NewRedisJobQueue(redisAddr).WithMetrics(metrics)
	} else {
		logger.Info(ctx, "Usando repositório e fila em memória (modo local)")
		repoAdapter = repository.NewMemoryJobRepository()
		queueAdapter = queue.NewChannelJobQueue(500).WithMetrics(metrics)
	}

	router := apphttp.NewRouterWithOptions(apphttp.RouterOptions{
		Repo:    repoAdapter,
		Storage: storageAdapter,
		Queue:   queueAdapter,
		Logger:  logger,
		Metrics: metrics,
	})

	handlerWithMiddleware := apphttp.ObservabilityMiddleware(logger)(router)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("FileConvert API rodando na porta %s com observabilidade e persistência ativa...\n", port)
	if err := http.ListenAndServe(":"+port, handlerWithMiddleware); err != nil {
		log.Fatalf("Erro no servidor HTTP: %v", err)
	}
}
