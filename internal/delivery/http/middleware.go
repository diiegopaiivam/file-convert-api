package http

import (
	"crypto/rand"
	"fmt"
	"net/http"

	"github.com/file-convert-api/internal/ports"
)

// ObservabilityMiddleware intercepta requisições HTTP para extrair/gerar correlation_id e injetá-lo no context.
func ObservabilityMiddleware(logger ports.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			correlationID := r.Header.Get("X-Correlation-ID")
			if correlationID == "" {
				correlationID = r.Header.Get("X-Request-ID")
			}
			if correlationID == "" {
				correlationID = generateCorrelationID()
			}

			w.Header().Set("X-Correlation-ID", correlationID)
			ctx := ports.WithCorrelationID(r.Context(), correlationID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func generateCorrelationID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("req-%x", b)
}
