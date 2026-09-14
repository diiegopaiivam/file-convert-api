package http

import (
	"net/http"

	"github.com/file-convert-api/internal/adapters/http/docs"
	"github.com/file-convert-api/internal/adapters/observability"
	"github.com/file-convert-api/internal/ports"
	httpSwagger "github.com/swaggo/http-swagger/v2"
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

	// @Summary Health Check
	// @Description Endpoint público para verificação da saúde do serviço
	// @Tags Health
	// @Produce json
	// @Success 200 {object} map[string]string "Status OK"
	// @Router /health [get]
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		renderJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// @Summary Métricas Prometheus
	// @Description Exposição de telemetria e métricas de negócio do Prometheus
	// @Tags Observability
	// @Produce plain
	// @Success 200 {string} string "Métricas no formato Prometheus"
	// @Router /metrics [get]
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
	mux.HandleFunc("/v1/conversions/document", audioHandler.ConvertAudio)
	mux.HandleFunc("/v1/conversions/image", audioHandler.ConvertAudio)
	mux.HandleFunc("/v1/conversions", audioHandler.ConvertAudio)
	mux.HandleFunc("/v1/jobs/", jobHandler.HandleJob)

	// Rotas da Documentação OpenAPI / Swagger UI (Públicas, sem auth)
	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/index.html", http.StatusMovedPermanently)
	})

	mux.HandleFunc("/docs/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		_, _ = w.Write([]byte(docs.SwaggerYAML))
	})

	mux.HandleFunc("/docs/swagger.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		_, _ = w.Write([]byte(docs.SwaggerYAML))
	})

	mux.HandleFunc("/docs/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(docs.SwaggerInfo.ReadDoc()))
	})

	mux.HandleFunc("/docs/doc.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(docs.SwaggerInfo.ReadDoc()))
	})

	mux.HandleFunc("/docs/", httpSwagger.Handler(
		httpSwagger.URL("/docs/doc.json"),
	))

	return mux
}

func NewRouter(repo ports.JobRepository, storage ports.ObjectStorage, queue ports.JobQueue) *http.ServeMux {
	return NewRouterWithOptions(RouterOptions{
		Repo:    repo,
		Storage: storage,
		Queue:   queue,
	})
}
