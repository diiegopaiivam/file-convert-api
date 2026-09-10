package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/file-convert-api/internal/domain"
)

func renderJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

func renderError(w http.ResponseWriter, statusCode int, code, message string, details []domain.ErrorDetail) {
	errResp := domain.APIErrorResponse{
		Code:      code,
		Message:   message,
		Details:   details,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	renderJSON(w, statusCode, errResp)
}
