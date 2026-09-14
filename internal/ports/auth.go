package ports

import (
	"context"
	"time"
)

// ClientTierLimits define as quotas de uso associadas ao plano do cliente.
type ClientTierLimits struct {
	RateLimitRPS      int `json:"rate_limit_rps"`       // Requisições por segundo permitidas
	RateLimitRPM      int `json:"rate_limit_rpm"`       // Requisições por minuto permitidas
	MaxConcurrentJobs int `json:"max_concurrent_jobs"` // Quota máxima de jobs simultâneos ativas
}

// ClientIdentity representa os atributos da entidade autenticada.
type ClientIdentity struct {
	ID         string           `json:"client_id"`
	APIKey     string           `json:"api_key"`
	Plan       string           `json:"plan"` // e.g., "free", "pro", "enterprise"
	TierLimits ClientTierLimits `json:"tier_limits"`
}

// AuthProvider define o contrato para autenticação e validação de chaves de API na borda HTTP.
type AuthProvider interface {
	// Authenticate valida a chave de API fornecida (via header Authorization: Bearer ou X-API-Key).
	// Retorna domain.ErrUnauthorized caso a chave seja inválida, revogada ou expirada.
	Authenticate(ctx context.Context, apiKey string) (*ClientIdentity, error)
}

// RateLimitResult especifica o estado atual de consumo de limites de requisição ou concorrência.
type RateLimitResult struct {
	Allowed           bool          `json:"allowed"`
	Limit             int           `json:"limit"`
	Remaining         int           `json:"remaining"`
	ResetDuration     time.Duration `json:"reset_duration"`      // Duração da janela até o reset
	RetryAfterSeconds int           `json:"retry_after_seconds"` // Valor numérico para o cabeçalho HTTP Retry-After
	Reason            string        `json:"reason,omitempty"`    // "rate_limit_exceeded" ou "concurrent_jobs_exceeded"
}

// RateLimiter abstrai os algoritmos de controle de vazão (Token Bucket) e semáforo distribuído.
type RateLimiter interface {
	// AllowKey verifica se a chave de API respeita o limite de taxa de requisições configurado (RPS/RPM).
	AllowKey(ctx context.Context, apiKey string, limitRPS int) (*RateLimitResult, error)

	// AllowIP verifica se o endereço IP do cliente respeita o limite de requisições de borda (ex.: proteção DoS).
	AllowIP(ctx context.Context, ip string, limitRPS int) (*RateLimitResult, error)

	// AcquireConcurrentJob tenta alocar uma vaga na quota de jobs simultâneos do cliente (status pending/processing).
	// Retorna Allowed=false com Reason="concurrent_jobs_exceeded" se a cota for atingida.
	AcquireConcurrentJob(ctx context.Context, apiKey string, maxConcurrent int) (*RateLimitResult, error)

	// ReleaseConcurrentJob decrementa o contador de jobs simultâneos ativos quando o job é finalizado (done/failed).
	ReleaseConcurrentJob(ctx context.Context, apiKey string) error
}
