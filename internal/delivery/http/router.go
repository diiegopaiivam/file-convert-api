package http

import (
	"net/http"

	"github.com/file-convert-api/internal/adapters/observability"
	"github.com/file-convert-api/internal/ports"
)

type RouterOptions struct {
	Repo    ports.JobRepository
	Storage ports.ObjectStorage
	Queue   ports.JobQueue
	Logger  ports.Logger
	Metrics ports.MetricsRecorder
}

func NewRouterWithOptions(opts RouterOptions) *http.ServeMux {
	mux := http.NewServeMux()

	if opts.Logger == nil {
		opts.Logger = observability.NewNoOpLogger()
	}
	if opts.Metrics == nil {
		opts.Metrics = observability.NewNoOpMetricsRecorder()
	}

	audioHandler := NewAudioHandler(opts.Repo, opts.Storage, opts.Queue).
		WithLogger(opts.Logger).
		WithMetrics(opts.Metrics)

	jobHandler := NewJobHandler(opts.Repo, opts.Storage)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		renderJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Endpoint de Métricas do Prometheus
	if promRecorder, ok := opts.Metrics.(*observability.PrometheusMetricsRecorder); ok {
		mux.HandleFunc("/metrics", promRecorder.Handler())
	} else {
		mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("# HELP fileconvert_conversions_total Total de conversões executadas\n# TYPE fileconvert_conversions_total counter\n"))
		})
	}

	mux.HandleFunc("/v1/conversions/audio", audioHandler.ConvertAudio)
	mux.HandleFunc("/v1/jobs/", jobHandler.HandleJob)

	return mux
}

func NewRouter(repo ports.JobRepository, storage ports.ObjectStorage, queue ports.JobQueue) *http.ServeMux {
	return NewRouterWithOptions(RouterOptions{
		Repo:    repo,
		Storage: storage,
		Queue:   queue,
	})
}
