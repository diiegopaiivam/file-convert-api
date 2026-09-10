package ports

import (
	"context"
	"time"

	"github.com/file-convert-api/internal/domain"
)

// Field representa um atributo estruturado chave-valor para log.
type Field struct {
	Key   string
	Value any
}

// Logger define o contrato para emissores de logs estruturados.
type Logger interface {
	Debug(ctx context.Context, msg string, fields ...Field)
	Info(ctx context.Context, msg string, fields ...Field)
	Warn(ctx context.Context, msg string, fields ...Field)
	Error(ctx context.Context, msg string, fields ...Field)

	// With retorna um novo Logger contendo os atributos base fixados.
	With(fields ...Field) Logger
}

// MetricsRecorder define a abstração para emissão de contadores, histogramas e gauges.
type MetricsRecorder interface {
	// RecordConversionResult registra a conclusão (sucesso ou falha) de uma conversão por par de formato.
	RecordConversionResult(ctx context.Context, srcFormat, targetFormat domain.Format, status domain.JobStatus)

	// RecordConversionDuration registra a duração ativa do processamento do conversor (latência).
	RecordConversionDuration(ctx context.Context, srcFormat, targetFormat domain.Format, duration time.Duration)

	// RecordJobDwellTime registra o tempo de espera do job na fila antes de ser consumido pelo worker.
	RecordJobDwellTime(ctx context.Context, srcFormat, targetFormat domain.Format, duration time.Duration)

	// RecordQueueDepth atualiza a contagem de jobs pendentes na fila.
	RecordQueueDepth(ctx context.Context, queueName string, depth int64)

	// RecordEdgeRequest registra as requisições HTTP de entrada e erros de validação síncronos na borda.
	RecordEdgeRequest(ctx context.Context, method, endpoint string, statusCode int, errorCode string)
}

// Span abstrai uma unidade de trabalho delimitada temporalmente na execução.
type Span interface {
	End()
	SetAttribute(key string, value any)
	RecordError(err error)
}

// Tracer define a interface para criação e propagação de spans.
type Tracer interface {
	StartSpan(ctx context.Context, operationName string) (context.Context, Span)
}
