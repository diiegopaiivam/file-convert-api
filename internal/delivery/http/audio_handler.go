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

const MaxUploadSize = 50 * 1024 * 1024 // 50 MB (52.428.800 bytes) conforme RNF 4.1 e ADR 0006

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

type ConversionJobResponse struct {
	JobID        string           `json:"job_id" example:"job_a1b2c3d4"`
	SourceFormat domain.Format    `json:"source_format" example:"wav"`
	TargetFormat domain.Format    `json:"target_format" example:"mp3"`
	Status       domain.JobStatus `json:"status" example:"pending"`
	CreatedAt    string           `json:"created_at" example:"2026-09-14T15:00:00Z"`
}

// @Summary Criar job de conversão de áudio
// @Description Recebe um arquivo de áudio (.wav ou .mp3) até 50 MB e enfileira um job de conversão assíncrona.
// @Tags Conversions
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Arquivo de áudio de entrada (.wav ou .mp3, máx 50 MB)"
// @Param target_format formData string true "Formato de destino desejado ('mp3' ou 'wav')"
// @Success 202 {object} ConversionJobResponse "Job criado e enfileirado com sucesso (status: pending)"
// @Failure 400 {object} domain.APIErrorResponse "Arquivo vazio ou parâmetros inválidos"
// @Failure 401 {object} domain.APIErrorResponse "Autenticação necessária / Chave de API inválida"
// @Failure 413 {object} domain.APIErrorResponse "Arquivo enviado excede o limite de 50 MB"
// @Failure 415 {object} domain.APIErrorResponse "Par de formatos de conversão não suportado"
// @Failure 429 {object} domain.APIErrorResponse "Limite de requisições por segundo ou quota de jobs simultâneos excedida"
// @Failure 500 {object} domain.APIErrorResponse "Erro interno do servidor"
// @Security ApiKeyAuth
// @Security BearerAuth
// @Router /v1/conversions/audio [post]
func (h *AudioHandler) ConvertAudio(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.metrics.RecordEdgeRequest(r.Context(), r.Method, "/v1/conversions/audio", http.StatusMethodNotAllowed, domain.ErrCodeInternalError)
		renderError(w, http.StatusMethodNotAllowed, domain.ErrCodeInternalError, "Método HTTP não permitido", nil)
		return
	}

	// 1. Validar Content-Length antecipadamente antes de processar qualquer payload
	if r.ContentLength > MaxUploadSize {
		h.metrics.RecordEdgeRequest(r.Context(), r.Method, "/v1/conversions/audio", http.StatusRequestEntityTooLarge, domain.ErrCodeFileTooLarge)
		renderError(w, http.StatusRequestEntityTooLarge, domain.ErrCodeFileTooLarge, "O arquivo enviado excede o limite máximo permitido de 50 MB.", nil)
		return
	}

	// 2. Limitar o tamanho do corpo da requisição usando MaxBytesReader para estancar uploads em streaming
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadSize)
	if err := r.ParseMultipartForm(MaxUploadSize); err != nil {
		if strings.Contains(err.Error(), "too large") || strings.Contains(err.Error(), "request body too large") {
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
		renderError(w, http.StatusRequestEntityTooLarge, domain.ErrCodeFileTooLarge, "O arquivo enviado excede o limite máximo permitido de 50 MB.", nil)
		return
	}

	targetFormatStr := strings.ToLower(strings.TrimSpace(r.FormValue("target_format")))
	targetFormatStr = strings.Trim(targetFormatStr, "\"'")
	if targetFormatStr == "" {
		h.metrics.RecordEdgeRequest(r.Context(), r.Method, "/v1/conversions/audio", http.StatusBadRequest, domain.ErrCodeUnsupportedFormat)
		renderError(w, http.StatusBadRequest, domain.ErrCodeUnsupportedFormat, "O parâmetro 'target_format' é obrigatório.", nil)
		return
	}

	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(header.Filename)), ".")
	sourceFormat := domain.Format(ext)
	targetFormat := domain.Format(targetFormatStr)

	// Validar se é um par de conversão suportado no domínio
	if !domain.IsValidConversionPair(sourceFormat, targetFormat) {
		h.metrics.RecordEdgeRequest(r.Context(), r.Method, r.URL.Path, http.StatusUnsupportedMediaType, domain.ErrCodeUnsupportedFormat)
		renderError(w, http.StatusUnsupportedMediaType, domain.ErrCodeUnsupportedFormat,
			fmt.Sprintf("Conversão de '%s' para '%s' não é suportada na v1.", sourceFormat, targetFormat), nil)
		return
	}

	// Gerar ID do Job e associar ao contexto
	jobID := generateUUID()
	ctx := ports.WithJobID(r.Context(), jobID)
	sourceKey := fmt.Sprintf("sources/%s.%s", jobID, sourceFormat)

	// Upload do arquivo para o ObjectStorage (usando metadados sanitizados)
	contentType := getContentType(sourceFormat)
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
