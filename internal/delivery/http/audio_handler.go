package http

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/file-convert-api/internal/adapters/observability"
	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
)

const MaxUploadSize = 50 * 1024 * 1024 // 50 MB

type AudioHandler struct {
	repo    ports.JobRepository
	storage ports.ObjectStorage
	queue   ports.JobQueue
	logger  ports.Logger
	metrics ports.MetricsRecorder
}

func NewAudioHandler(repo ports.JobRepository, storage ports.ObjectStorage, queue ports.JobQueue) *AudioHandler {
	return &AudioHandler{
		repo:    repo,
		storage: storage,
		queue:   queue,
		logger:  observability.NewNoOpLogger(),
		metrics: observability.NewNoOpMetricsRecorder(),
	}
}

func (h *AudioHandler) WithLogger(logger ports.Logger) *AudioHandler {
	if logger != nil {
		h.logger = logger
	}
	return h
}

func (h *AudioHandler) WithMetrics(metrics ports.MetricsRecorder) *AudioHandler {
	if metrics != nil {
		h.metrics = metrics
	}
	return h
}

func (h *AudioHandler) ConvertAudio(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.metrics.RecordEdgeRequest(r.Context(), r.Method, "/v1/conversions/audio", http.StatusMethodNotAllowed, domain.ErrCodeInternalError)
		renderError(w, http.StatusMethodNotAllowed, domain.ErrCodeInternalError, "Método HTTP não permitido", nil)
		return
	}

	// Limitar tamanho do corpo da requisição a 50 MB
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadSize)
	if err := r.ParseMultipartForm(MaxUploadSize); err != nil {
		if strings.Contains(err.Error(), "too large") {
			h.metrics.RecordEdgeRequest(r.Context(), r.Method, "/v1/conversions/audio", http.StatusRequestEntityTooLarge, domain.ErrCodeFileTooLarge)
			renderError(w, http.StatusRequestEntityTooLarge, domain.ErrCodeFileTooLarge, "O arquivo enviado excede o limite máximo permitido de 50 MB.", nil)
			return
		}
		h.metrics.RecordEdgeRequest(r.Context(), r.Method, "/v1/conversions/audio", http.StatusBadRequest, domain.ErrCodeEmptyFile)
		renderError(w, http.StatusBadRequest, domain.ErrCodeEmptyFile, "Falha ao processar formulário multipart: "+err.Error(), nil)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.metrics.RecordEdgeRequest(r.Context(), r.Method, "/v1/conversions/audio", http.StatusBadRequest, domain.ErrCodeEmptyFile)
		renderError(w, http.StatusBadRequest, domain.ErrCodeEmptyFile, "O parâmetro 'file' é obrigatório no formulário.", nil)
		return
	}
	defer file.Close()

	if header.Size == 0 {
		h.metrics.RecordEdgeRequest(r.Context(), r.Method, "/v1/conversions/audio", http.StatusBadRequest, domain.ErrCodeEmptyFile)
		renderError(w, http.StatusBadRequest, domain.ErrCodeEmptyFile, "O arquivo enviado está vazio (0 bytes).", nil)
		return
	}

	if header.Size > MaxUploadSize {
		h.metrics.RecordEdgeRequest(r.Context(), r.Method, "/v1/conversions/audio", http.StatusRequestEntityTooLarge, domain.ErrCodeFileTooLarge)
		renderError(w, http.StatusRequestEntityTooLarge, domain.ErrCodeFileTooLarge, "O arquivo enviado excede o limite de 50 MB.", nil)
		return
	}

	targetFormatStr := strings.ToLower(strings.TrimSpace(r.FormValue("target_format")))
	if targetFormatStr == "" {
		h.metrics.RecordEdgeRequest(r.Context(), r.Method, "/v1/conversions/audio", http.StatusBadRequest, domain.ErrCodeUnsupportedFormat)
		renderError(w, http.StatusBadRequest, domain.ErrCodeUnsupportedFormat, "O parâmetro 'target_format' é obrigatório.", nil)
		return
	}

	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(header.Filename)), ".")
	sourceFormat := domain.Format(ext)
	targetFormat := domain.Format(targetFormatStr)

	// Validar que é um par de conversão de áudio suportado (.wav ↔ .mp3)
	isValidPair := (sourceFormat == domain.FormatWAV && targetFormat == domain.FormatMP3) ||
		(sourceFormat == domain.FormatMP3 && targetFormat == domain.FormatWAV)

	if !isValidPair {
		h.metrics.RecordEdgeRequest(r.Context(), r.Method, "/v1/conversions/audio", http.StatusUnsupportedMediaType, domain.ErrCodeUnsupportedFormat)
		renderError(w, http.StatusUnsupportedMediaType, domain.ErrCodeUnsupportedFormat,
			fmt.Sprintf("Conversão de '%s' para '%s' não é suportada na v1.", sourceFormat, targetFormat), nil)
		return
	}

	// Gerar ID do Job e associar ao contexto
	jobID := generateUUID()
	ctx := ports.WithJobID(r.Context(), jobID)
	sourceKey := fmt.Sprintf("sources/%s.%s", jobID, sourceFormat)

	// Upload do arquivo para o ObjectStorage (usando metadados sanitizados)
	contentType := "audio/" + string(sourceFormat)
	if err := h.storage.Upload(ctx, sourceKey, file, header.Size, contentType); err != nil {
		h.logger.Error(ctx, "Falha ao armazenar arquivo recebido no storage", ports.Field{Key: "error", Value: err.Error()})
		h.metrics.RecordEdgeRequest(ctx, r.Method, "/v1/conversions/audio", http.StatusInternalServerError, domain.ErrCodeInternalError)
		renderError(w, http.StatusInternalServerError, domain.ErrCodeInternalError, "Falha ao armazenar arquivo enviado: "+err.Error(), nil)
		return
	}

	// Criar entidade Job
	now := time.Now().UTC()
	job := &domain.Job{
		ID:              jobID,
		SourceFormat:    sourceFormat,
		TargetFormat:    targetFormat,
		SourceObjectKey: sourceKey,
		Status:          domain.StatusPending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := h.repo.Create(ctx, job); err != nil {
		h.logger.Error(ctx, "Falha ao criar registro do job no repositório", ports.Field{Key: "error", Value: err.Error()})
		h.metrics.RecordEdgeRequest(ctx, r.Method, "/v1/conversions/audio", http.StatusInternalServerError, domain.ErrCodeInternalError)
		renderError(w, http.StatusInternalServerError, domain.ErrCodeInternalError, "Falha ao criar registro de job: "+err.Error(), nil)
		return
	}

	// Enfileirar Job
	if err := h.queue.Enqueue(ctx, jobID); err != nil {
		h.logger.Error(ctx, "Falha ao enfileirar job", ports.Field{Key: "error", Value: err.Error()})
		h.metrics.RecordEdgeRequest(ctx, r.Method, "/v1/conversions/audio", http.StatusInternalServerError, domain.ErrCodeInternalError)
		renderError(w, http.StatusInternalServerError, domain.ErrCodeInternalError, "Falha ao enfileirar job: "+err.Error(), nil)
		return
	}

	h.metrics.RecordEdgeRequest(ctx, r.Method, "/v1/conversions/audio", http.StatusAccepted, "NONE")
	h.logger.Info(ctx, "Recepção de job de áudio efetuada com sucesso (status pending)",
		ports.Field{Key: "source_format", Value: sourceFormat},
		ports.Field{Key: "target_format", Value: targetFormat},
		ports.Field{Key: "file_size_bytes", Value: header.Size},
	)

	// Retornar resposta de confirmação 202 Accepted
	response := map[string]interface{}{
		"job_id":        job.ID,
		"source_format": job.SourceFormat,
		"target_format": job.TargetFormat,
		"status":        job.Status,
		"created_at":    job.CreatedAt.Format(time.RFC3339),
	}
	renderJSON(w, http.StatusAccepted, response)
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("job_%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
