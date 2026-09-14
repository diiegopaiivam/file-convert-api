package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/file-convert-api/internal/adapters/http/middleware"
	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
)

func TestAuthMiddleware_MissingKey(t *testing.T) {
	authProvider := middleware.NewStaticAuthProvider()
	authMiddleware := middleware.AuthMiddleware(authProvider)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := authMiddleware(nextHandler)

	req := httptest.NewRequest("GET", "/v1/conversions/job-123", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("Esperado status 401 Unauthorized, obtido: %d", rec.Code)
	}

	var errResp domain.APIErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("Falha ao decodificar JSON de erro: %v", err)
	}

	if errResp.Code != domain.ErrCodeUnauthorized {
		t.Errorf("Código de erro esperado %s, obtido: %s", domain.ErrCodeUnauthorized, errResp.Code)
	}
}

func TestAuthMiddleware_InvalidKey(t *testing.T) {
	authProvider := middleware.NewStaticAuthProvider()
	authMiddleware := middleware.AuthMiddleware(authProvider)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := authMiddleware(nextHandler)

	req := httptest.NewRequest("GET", "/v1/conversions/job-123", nil)
	req.Header.Set("Authorization", "Bearer fc_test_invalidkey999")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("Esperado status 401 Unauthorized, obtido: %d", rec.Code)
	}

	var errResp domain.APIErrorResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Code != domain.ErrCodeUnauthorized {
		t.Errorf("Código de erro esperado %s, obtido: %s", domain.ErrCodeUnauthorized, errResp.Code)
	}
}

func TestAuthMiddleware_ValidKey_BearerAndHeader(t *testing.T) {
	authProvider := middleware.NewStaticAuthProvider()
	authMiddleware := middleware.AuthMiddleware(authProvider)

	var capturedClient *ports.ClientIdentity
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, ok := ports.ClientIdentityFromContext(r.Context())
		if ok {
			capturedClient = client
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := authMiddleware(nextHandler)

	// 1. Validar via Authorization: Bearer fc_test_freekey123
	req1 := httptest.NewRequest("GET", "/v1/conversions/job-123", nil)
	req1.Header.Set("Authorization", "Bearer fc_test_freekey123")
	rec1 := httptest.NewRecorder()

	handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("Esperado status 200 OK, obtido: %d", rec1.Code)
	}
	if capturedClient == nil || capturedClient.Plan != "free" {
		t.Errorf("ClientIdentity não foi injetado corretamente no contexto")
	}

	// 2. Validar via X-API-Key: fc_live_prokey456
	req2 := httptest.NewRequest("GET", "/v1/conversions/job-456", nil)
	req2.Header.Set("X-API-Key", "fc_live_prokey456")
	rec2 := httptest.NewRecorder()

	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("Esperado status 200 OK, obtido: %d", rec2.Code)
	}
	if capturedClient == nil || capturedClient.Plan != "pro" {
		t.Errorf("ClientIdentity Pro não foi injetado corretamente no contexto")
	}
}

func TestRateLimitMiddleware_ExceedIPLimit(t *testing.T) {
	rateLimiter := middleware.NewRedisRateLimiter("invalid-addr:6379") // Usa fallback in-memory
	rateLimitMiddleware := middleware.RateLimitMiddleware(rateLimiter, 2) // Limite de 2 req/s por IP

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := rateLimitMiddleware(nextHandler)

	// Fazer 2 requisições permitidas
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/v1/conversions/job", nil)
		req.RemoteAddr = "192.168.1.50:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Requisição %d deveria passar, obtido: %d", i+1, rec.Code)
		}
	}

	// Terceira requisição deve estourar o rate limit de IP (429)
	req := httptest.NewRequest("GET", "/v1/conversions/job", nil)
	req.RemoteAddr = "192.168.1.50:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("Esperado status 429 Too Many Requests, obtido: %d", rec.Code)
	}

	retryAfter := rec.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Errorf("Cabeçalho Retry-After é obrigatório na resposta 429")
	}

	var errResp domain.APIErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("Falha ao decodificar erro JSON: %v", err)
	}

	if errResp.Code != domain.ErrCodeRateLimitExceeded {
		t.Errorf("Código de erro esperado %s, obtido: %s", domain.ErrCodeRateLimitExceeded, errResp.Code)
	}
}

func TestRateLimitMiddleware_ExceedKeyLimit(t *testing.T) {
	authProvider := middleware.NewStaticAuthProvider()
	authMiddleware := middleware.AuthMiddleware(authProvider)

	rateLimiter := middleware.NewRedisRateLimiter("invalid-addr:6379")
	rateLimitMiddleware := middleware.RateLimitMiddleware(rateLimiter, 100)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Encadeia AuthMiddleware + RateLimitMiddleware
	handler := authMiddleware(rateLimitMiddleware(nextHandler))

	// Registrar cliente com limite de 1 RPS
	authProvider.RegisterKey(&ports.ClientIdentity{
		ID:     "client_strict",
		APIKey: "fc_test_strict1",
		Plan:   "free",
		TierLimits: ports.ClientTierLimits{
			RateLimitRPS:      1,
			RateLimitRPM:      10,
			MaxConcurrentJobs: 5,
		},
	})

	// Primeira requisição OK
	req1 := httptest.NewRequest("GET", "/v1/conversions/job-1", nil)
	req1.Header.Set("Authorization", "Bearer fc_test_strict1")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("Requisição 1 esperada 200 OK, obtido: %d", rec1.Code)
	}

	// Verificar cabeçalhos X-RateLimit-*
	if rec1.Header().Get("X-RateLimit-Limit") != "1" {
		t.Errorf("X-RateLimit-Limit esperado '1', obtido: %s", rec1.Header().Get("X-RateLimit-Limit"))
	}

	// Segunda requisição estoura rate limit por chave (429)
	req2 := httptest.NewRequest("GET", "/v1/conversions/job-2", nil)
	req2.Header.Set("Authorization", "Bearer fc_test_strict1")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("Requisição 2 esperada 429 Too Many Requests, obtido: %d", rec2.Code)
	}

	if rec2.Header().Get("Retry-After") == "" {
		t.Errorf("Cabeçalho Retry-After ausente na resposta 429")
	}

	var errResp domain.APIErrorResponse
	_ = json.Unmarshal(rec2.Body.Bytes(), &errResp)
	if errResp.Code != domain.ErrCodeRateLimitExceeded {
		t.Errorf("Código de erro esperado %s, obtido: %s", domain.ErrCodeRateLimitExceeded, errResp.Code)
	}
}

func TestRateLimitMiddleware_ExceedConcurrentJobsQuota(t *testing.T) {
	authProvider := middleware.NewStaticAuthProvider()
	authMiddleware := middleware.AuthMiddleware(authProvider)

	rateLimiter := middleware.NewRedisRateLimiter("invalid-addr:6379")
	rateLimitMiddleware := middleware.RateLimitMiddleware(rateLimiter, 100)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	handler := authMiddleware(rateLimitMiddleware(nextHandler))

	// Registrar cliente com max 1 job simultâneo
	authProvider.RegisterKey(&ports.ClientIdentity{
		ID:     "client_single_job",
		APIKey: "fc_test_single1",
		Plan:   "free",
		TierLimits: ports.ClientTierLimits{
			RateLimitRPS:      100,
			RateLimitRPM:      1000,
			MaxConcurrentJobs: 1,
		},
	})

	// Primeiro POST /v1/jobs -> Consegue alocar vaga de job simultâneo
	req1 := httptest.NewRequest("POST", "/v1/jobs", nil)
	req1.Header.Set("Authorization", "Bearer fc_test_single1")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusCreated {
		t.Fatalf("Primeiro POST esperado 201 Created, obtido: %d", rec1.Code)
	}

	// Segundo POST /v1/jobs -> Excede quota de jobs simultâneos (429)
	req2 := httptest.NewRequest("POST", "/v1/jobs", nil)
	req2.Header.Set("Authorization", "Bearer fc_test_single1")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("Segundo POST esperado 429 Too Many Requests, obtido: %d", rec2.Code)
	}

	if rec2.Header().Get("Retry-After") == "" {
		t.Errorf("Cabeçalho Retry-After ausente na resposta 429")
	}

	var errResp domain.APIErrorResponse
	_ = json.Unmarshal(rec2.Body.Bytes(), &errResp)
	if errResp.Code != domain.ErrCodeConcurrentJobsExceeded {
		t.Errorf("Código de erro esperado %s, obtido: %s", domain.ErrCodeConcurrentJobsExceeded, errResp.Code)
	}
}
