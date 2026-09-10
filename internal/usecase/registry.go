package usecase

import (
	"sync"

	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
)

var _ ports.ConverterRegistry = (*DefaultConverterRegistry)(nil)

// DefaultConverterRegistry é a implementação padrão da interface ports.ConverterRegistry.
type DefaultConverterRegistry struct {
	mu         sync.RWMutex
	converters []ports.Converter
}

// NewDefaultConverterRegistry cria um novo registro de conversores em memória.
func NewDefaultConverterRegistry() *DefaultConverterRegistry {
	return &DefaultConverterRegistry{
		converters: make([]ports.Converter, 0),
	}
}

// Register adiciona um novo adaptador Converter ao registro.
func (r *DefaultConverterRegistry) Register(converter ports.Converter) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.converters = append(r.converters, converter)
	return nil
}

// GetConverter retorna o primeiro Converter registrado que suporte a conversão do par (srcFormat, targetFormat).
func (r *DefaultConverterRegistry) GetConverter(srcFormat, targetFormat domain.Format) (ports.Converter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, conv := range r.converters {
		if conv.CanConvert(srcFormat, targetFormat) {
			return conv, nil
		}
	}

	return nil, domain.ErrUnsupportedFormat
}
