package middleware

import (
	"context"
	"sync"

	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
)

var _ ports.AuthProvider = (*StaticAuthProvider)(nil)

// StaticAuthProvider implementa o port ports.AuthProvider utilizando um repositório em memória de API Keys.
type StaticAuthProvider struct {
	mu   sync.RWMutex
	keys map[string]*ports.ClientIdentity
}

// NewStaticAuthProvider cria uma nova instância do provedor estático de autenticação.
func NewStaticAuthProvider() *StaticAuthProvider {
	provider := &StaticAuthProvider{
		keys: make(map[string]*ports.ClientIdentity),
	}

	// Registrar chaves padrão para testes e desenvolvimento
	provider.RegisterKey(&ports.ClientIdentity{
		ID:     "client_free_01",
		APIKey: "fc_test_freekey123",
		Plan:   "free",
		TierLimits: ports.ClientTierLimits{
			RateLimitRPS:      10,
			RateLimitRPM:      100,
			MaxConcurrentJobs: 2,
		},
	})

	provider.RegisterKey(&ports.ClientIdentity{
		ID:     "client_pro_01",
		APIKey: "fc_live_prokey456",
		Plan:   "pro",
		TierLimits: ports.ClientTierLimits{
			RateLimitRPS:      50,
			RateLimitRPM:      1000,
			MaxConcurrentJobs: 10,
		},
	})

	provider.RegisterKey(&ports.ClientIdentity{
		ID:     "client_enterprise_01",
		APIKey: "fc_live_entkey789",
		Plan:   "enterprise",
		TierLimits: ports.ClientTierLimits{
			RateLimitRPS:      200,
			RateLimitRPM:      5000,
			MaxConcurrentJobs: 50,
		},
	})

	return provider
}

// RegisterKey adiciona ou atualiza uma chave de API na memória do provedor.
func (p *StaticAuthProvider) RegisterKey(identity *ports.ClientIdentity) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.keys[identity.APIKey] = identity
}

// Authenticate valida a chave de API fornecida.
func (p *StaticAuthProvider) Authenticate(ctx context.Context, apiKey string) (*ports.ClientIdentity, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	identity, ok := p.keys[apiKey]
	if !ok {
		return nil, domain.ErrUnauthorized
	}

	return identity, nil
}
