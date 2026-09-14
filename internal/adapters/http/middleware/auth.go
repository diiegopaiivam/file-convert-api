package middleware

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
)

// AuthMiddleware intercepta a requisição HTTP e valida a chave de API (API Key).
// Se válida, injeta o ClientIdentity no contexto da requisição; caso contrário, retorna HTTP 401 Unauthorized.
func AuthMiddleware(authProvider ports.AuthProvider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := extractAPIKey(r)
			if apiKey == "" {
				writeUnauthorizedError(w, "Chave de API ausente na requisição.")
				return
			}

			clientIdentity, err := authProvider.Authenticate(r.Context(), apiKey)
			if err != nil || clientIdentity == nil {
				writeUnauthorizedError(w, "Chave de API inválida, revogada ou expirada.")
				return
			}

			// Injeta os dados da identidade autenticada no contexto
			ctx := ports.WithClientIdentity(r.Context(), clientIdentity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractAPIKey(r *http.Request) string {
	// 1. Tentar extrair do cabeçalho Authorization: Bearer <key>
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}

	// 2. Tentar extrair do cabeçalho X-API-Key: <key>
	apiKeyHeader := r.Header.Get("X-API-Key")
	if apiKeyHeader != "" {
		return strings.TrimSpace(apiKeyHeader)
	}

	return ""
}

func writeUnauthorizedError(w http.ResponseWriter, issueMsg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)

	resp := domain.APIErrorResponse{
		Code:    domain.ErrCodeUnauthorized,
		Message: "Autenticação necessária para acessar este recurso.",
		Details: []domain.ErrorDetail{
			{Issue: issueMsg},
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	_ = json.NewEncoder(w).Encode(resp)
}
