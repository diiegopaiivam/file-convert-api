package middleware

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
)

// RateLimitMiddleware intercepta as requisições HTTP e aplica o controle de vazão por IP e por Chave de API,
// além de limitar a quota de jobs simultâneos em requisições de criação de job.
func RateLimitMiddleware(rateLimiter ports.RateLimiter, ipLimitRPS int) func(http.Handler) http.Handler {
	if ipLimitRPS <= 0 {
		ipLimitRPS = 5 // Limite default de borda por IP (5 req/s)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// 1. Checagem de Rate Limit por IP (Borda)
			clientIP := extractClientIP(r)
			ipResult, err := rateLimiter.AllowIP(ctx, clientIP, ipLimitRPS)
			if err == nil && ipResult != nil && !ipResult.Allowed {
				writeRateLimitExceededError(w, ipResult, "Muitas requisições originadas do mesmo endereço IP.")
				return
			}

			// 2. Checagem de Rate Limit por Chave de API (Autenticado)
			clientIdentity, isAuth := ports.ClientIdentityFromContext(ctx)
			if isAuth && clientIdentity != nil {
				keyLimitRPS := clientIdentity.TierLimits.RateLimitRPS
				if keyLimitRPS <= 0 {
					keyLimitRPS = 10
				}

				keyResult, err := rateLimiter.AllowKey(ctx, clientIdentity.APIKey, keyLimitRPS)
				if err == nil && keyResult != nil {
					setRateLimitHeaders(w, keyResult)

					if !keyResult.Allowed {
						writeRateLimitExceededError(w, keyResult, "Sua chave de API excedeu a taxa máxima de requisições por segundo.")
						return
					}
				}

				// 3. Checagem de Quota de Jobs Simultâneos (Apenas no POST de criação de job)
				if r.Method == http.MethodPost && (strings.Contains(r.URL.Path, "/v1/jobs") || strings.Contains(r.URL.Path, "/v1/conversions")) {
					maxConcurrent := clientIdentity.TierLimits.MaxConcurrentJobs
					if maxConcurrent <= 0 {
						maxConcurrent = 2
					}

					concurrentResult, err := rateLimiter.AcquireConcurrentJob(ctx, clientIdentity.APIKey, maxConcurrent)
					if err == nil && concurrentResult != nil && !concurrentResult.Allowed {
						writeConcurrentJobsExceededError(w, concurrentResult, clientIdentity.Plan, maxConcurrent)
						return
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func extractClientIP(r *http.Request) string {
	// Verificar o cabeçalho X-Forwarded-For enviado por proxies/Nginx
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		clientIP := strings.TrimSpace(ips[0])
		if clientIP != "" {
			return clientIP
		}
	}

	// Verificar o cabeçalho X-Real-IP
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return strings.TrimSpace(xri)
	}

	// Extrair IP do RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}

	return r.RemoteAddr
}

func setRateLimitHeaders(w http.ResponseWriter, result *ports.RateLimitResult) {
	if result == nil {
		return
	}
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(result.ResetDuration).Unix(), 10))
}

func writeRateLimitExceededError(w http.ResponseWriter, result *ports.RateLimitResult, issueMsg string) {
	retryAfter := 1
	if result != nil && result.RetryAfterSeconds > 0 {
		retryAfter = result.RetryAfterSeconds
	}

	if result != nil {
		setRateLimitHeaders(w, result)
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)

	resp := domain.APIErrorResponse{
		Code:    domain.ErrCodeRateLimitExceeded,
		Message: "Limite de taxa de requisições excedido (Rate Limit).",
		Details: []domain.ErrorDetail{
			{Issue: issueMsg},
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func writeConcurrentJobsExceededError(w http.ResponseWriter, result *ports.RateLimitResult, plan string, maxConcurrent int) {
	retryAfter := 5
	if result != nil && result.RetryAfterSeconds > 0 {
		retryAfter = result.RetryAfterSeconds
	}

	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)

	issueMsg := fmt.Sprintf("Sua chave de API (Plano %s) atingiu a quota máxima de %d jobs simultâneos em processamento. Aguarde a conclusão de um job antes de enviar novas requisições.", plan, maxConcurrent)

	resp := domain.APIErrorResponse{
		Code:    domain.ErrCodeConcurrentJobsExceeded,
		Message: "Quota de jobs simultâneos em processamento atingida.",
		Details: []domain.ErrorDetail{
			{Issue: issueMsg},
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	_ = json.NewEncoder(w).Encode(resp)
}
