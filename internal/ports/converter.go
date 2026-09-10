package ports

import (
	"context"
	"io"

	"github.com/file-convert-api/internal/domain"
)

// Converter define a interface de conversão de arquivos.
type Converter interface {
	// CanConvert verifica se a implementação é capaz de converter de srcFormat para targetFormat.
	CanConvert(srcFormat, targetFormat domain.Format) bool

	// Convert executa a conversão do stream de entrada e retorna o stream convertido.
	Convert(ctx context.Context, src io.Reader, srcFormat, targetFormat domain.Format) (io.Reader, error)
}

// ConverterRegistry gerencia os conversores disponíveis na aplicação.
type ConverterRegistry interface {
	// Register registra uma nova implementação de Converter.
	Register(converter Converter) error

	// GetConverter localiza o Converter capaz de realizar a conversão solicitada.
	GetConverter(srcFormat, targetFormat domain.Format) (Converter, error)
}
