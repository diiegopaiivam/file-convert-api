package http

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
)

type JobHandler struct {
	repo    ports.JobRepository
	storage ports.ObjectStorage
}

func NewJobHandler(repo ports.JobRepository, storage ports.ObjectStorage) *JobHandler {
	return &JobHandler{
		repo:    repo,
		storage: storage,
	}
}

func (h *JobHandler) HandleJob(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) == 0 || parts[0] == "" {
		renderError(w, http.StatusNotFound, domain.ErrCodeInternalError, "ID do Job não informado", nil)
		return
	}

	jobID := parts[0]

	// Se houver sub-recurso /download
	if len(parts) > 1 && parts[1] == "download" {
		h.DownloadJobResult(w, r, jobID)
		return
	}

	// Consultar Job
	h.GetJobStatus(w, r, jobID)
}

func (h *JobHandler) GetJobStatus(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodGet {
		renderError(w, http.StatusMethodNotAllowed, domain.ErrCodeInternalError, "Método HTTP não permitido", nil)
		return
	}

	ctx := r.Context()
	job, err := h.repo.GetByID(ctx, jobID)
	if err != nil {
		renderError(w, http.StatusInternalServerError, domain.ErrCodeInternalError, "Falha ao buscar job: "+err.Error(), nil)
		return
	}

	if job == nil {
		renderError(w, http.StatusNotFound, domain.ErrCodeInternalError, "Job não encontrado", nil)
		return
	}

	response := map[string]interface{}{
		"job_id":        job.ID,
		"source_format": job.SourceFormat,
		"target_format": job.TargetFormat,
		"status":        job.Status,
		"created_at":    job.CreatedAt.Format(time.RFC3339),
		"updated_at":    job.UpdatedAt.Format(time.RFC3339),
	}

	if job.Status == domain.StatusDone {
		response["download_url"] = fmt.Sprintf("/v1/jobs/%s/download", job.ID)
	}

	if job.Status == domain.StatusFailed && job.Error != nil {
		response["error"] = job.Error
	}

	renderJSON(w, http.StatusOK, response)
}

func (h *JobHandler) DownloadJobResult(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodGet {
		renderError(w, http.StatusMethodNotAllowed, domain.ErrCodeInternalError, "Método HTTP não permitido", nil)
		return
	}

	ctx := r.Context()
	job, err := h.repo.GetByID(ctx, jobID)
	if err != nil || job == nil {
		renderError(w, http.StatusNotFound, domain.ErrCodeInternalError, "Job não encontrado", nil)
		return
	}

	if job.Status != domain.StatusDone || job.TargetObjectKey == "" {
		renderError(w, http.StatusBadRequest, domain.ErrCodeInternalError, "O job ainda não foi concluído ou não possui arquivo disponível para download.", nil)
		return
	}

	stream, err := h.storage.Download(ctx, job.TargetObjectKey)
	if err != nil {
		renderError(w, http.StatusNotFound, domain.ErrCodeInternalError, "Arquivo não encontrado no armazenamento: "+err.Error(), nil)
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", getContentType(job.TargetFormat))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.%s\"", job.ID, job.TargetFormat))
	w.WriteHeader(http.StatusOK)

	_, _ = io.Copy(w, stream)
}

func getContentType(format domain.Format) string {
	switch format {
	case domain.FormatMP3:
		return "audio/mpeg"
	case domain.FormatWAV:
		return "audio/wav"
	case domain.FormatPDF:
		return "application/pdf"
	case domain.FormatJPG:
		return "image/jpeg"
	case domain.FormatPNG:
		return "image/png"
	default:
		return "application/octet-stream"
	}
}
