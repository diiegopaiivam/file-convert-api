package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/file-convert-api/internal/adapters/observability"
	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
)

// ProcessJobUseCase orquestra o ciclo de vida do processamento assíncrono de conversão de um Job.
type ProcessJobUseCase struct {
	jobRepo  ports.JobRepository
	storage  ports.ObjectStorage
	registry ports.ConverterRegistry
	logger   ports.Logger
	metrics  ports.MetricsRecorder
}

// NewProcessJobUseCase cria uma nova instância de ProcessJobUseCase.
func NewProcessJobUseCase(
	jobRepo ports.JobRepository,
	storage ports.ObjectStorage,
	registry ports.ConverterRegistry,
) *ProcessJobUseCase {
	return &ProcessJobUseCase{
		jobRepo:  jobRepo,
		storage:  storage,
		registry: registry,
		logger:   observability.NewNoOpLogger(),
		metrics:  observability.NewNoOpMetricsRecorder(),
	}
}

// WithLogger associa um Logger customizado ao usecase.
func (uc *ProcessJobUseCase) WithLogger(logger ports.Logger) *ProcessJobUseCase {
	if logger != nil {
		uc.logger = logger
	}
	return uc
}

// WithMetrics associa um MetricsRecorder customizado ao usecase.
func (uc *ProcessJobUseCase) WithMetrics(metrics ports.MetricsRecorder) *ProcessJobUseCase {
	if metrics != nil {
		uc.metrics = metrics
	}
	return uc
}

// Execute executa o processamento completo de um job a partir do seu ID.
func (uc *ProcessJobUseCase) Execute(ctx context.Context, jobID string) error {
	ctx = ports.WithJobID(ctx, jobID)

	// 1. Buscar o Job pelo ID
	job, err := uc.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		uc.logger.Error(ctx, "Falha ao buscar job no repositório", ports.Field{Key: "error", Value: err.Error()})
		return fmt.Errorf("falha ao buscar job %s: %w", jobID, err)
	}

	if job == nil {
		uc.logger.Warn(ctx, "Job não encontrado para execução")
		return domain.ErrJobNotFound
	}

	// 2. Transicionar status para 'processing'
	if err := uc.jobRepo.UpdateStatus(ctx, job.ID, domain.StatusProcessing, "", nil); err != nil {
		uc.logger.Error(ctx, "Falha ao atualizar status do job para processing", ports.Field{Key: "error", Value: err.Error()})
		return fmt.Errorf("falha ao atualizar status para processing: %w", err)
	}

	uc.logger.Info(ctx, "Job transicionado para status processing",
		ports.Field{Key: "source_format", Value: job.SourceFormat},
		ports.Field{Key: "target_format", Value: job.TargetFormat},
	)

	// 3. Obter o conversor no registry
	converter, err := uc.registry.GetConverter(job.SourceFormat, job.TargetFormat)
	if err != nil {
		return uc.failJob(ctx, job, domain.ErrCodeUnsupportedFormat, "Formato de arquivo não suportado", err)
	}

	// 4. Baixar o arquivo de origem do ObjectStorage
	srcStream, err := uc.storage.Download(ctx, job.SourceObjectKey)
	if err != nil {
		return uc.failJob(ctx, job, domain.ErrCodeInternalError, "Falha ao recuperar arquivo original do storage", err)
	}
	defer srcStream.Close()

	// 5. Executar a conversão através do Converter
	startTime := time.Now()
	convertedStream, err := converter.Convert(ctx, srcStream, job.SourceFormat, job.TargetFormat)
	duration := time.Since(startTime)

	uc.metrics.RecordConversionDuration(ctx, job.SourceFormat, job.TargetFormat, duration)

	if err != nil {
		if errors.Is(err, domain.ErrCorruptedFile) {
			return uc.failJob(ctx, job, domain.ErrCodeCorruptedFile, "O arquivo enviado está corrompido e não pôde ser processado.", err)
		}
		if errors.Is(err, domain.ErrJobTimeout) {
			return uc.failJob(ctx, job, domain.ErrCodeJobTimeout, "O tempo limite de processamento de 60 segundos foi excedido.", err)
		}
		if errors.Is(err, domain.ErrEmptyFile) {
			return uc.failJob(ctx, job, domain.ErrCodeEmptyFile, "O arquivo enviado está vazio.", err)
		}
		if errors.Is(err, domain.ErrUnsupportedFormat) {
			return uc.failJob(ctx, job, domain.ErrCodeUnsupportedFormat, "Formato não suportado.", err)
		}
		return uc.failJob(ctx, job, domain.ErrCodeInternalError, "Falha interna durante a conversão do arquivo.", err)
	}

	// Ler o resultado da conversão para determinar o tamanho do upload
	buf, err := io.ReadAll(convertedStream)
	if err != nil {
		return uc.failJob(ctx, job, domain.ErrCodeInternalError, "Falha ao ler o resultado da conversão", err)
	}

	// 6. Upload do arquivo convertido para o ObjectStorage
	targetKey := fmt.Sprintf("converted/%s.%s", job.ID, job.TargetFormat)
	contentType := getContentType(job.TargetFormat)

	err = uc.storage.Upload(ctx, targetKey, bytes.NewReader(buf), int64(len(buf)), contentType)
	if err != nil {
		return uc.failJob(ctx, job, domain.ErrCodeInternalError, "Falha ao armazenar arquivo convertido no storage", err)
	}

	// 7. Atualizar status final para 'done'
	if err := uc.jobRepo.UpdateStatus(ctx, job.ID, domain.StatusDone, targetKey, nil); err != nil {
		uc.logger.Error(ctx, "Falha ao atualizar status do job para done", ports.Field{Key: "error", Value: err.Error()})
		return fmt.Errorf("falha ao atualizar status para done: %w", err)
	}

	uc.metrics.RecordConversionResult(ctx, job.SourceFormat, job.TargetFormat, domain.StatusDone)
	uc.logger.Info(ctx, "Job concluído com sucesso (status done)",
		ports.Field{Key: "source_format", Value: job.SourceFormat},
		ports.Field{Key: "target_format", Value: job.TargetFormat},
		ports.Field{Key: "converted_size_bytes", Value: len(buf)},
		ports.Field{Key: "duration_ms", Value: duration.Milliseconds()},
	)

	return nil
}

func (uc *ProcessJobUseCase) failJob(ctx context.Context, job *domain.Job, code, msg string, cause error) error {
	jobErr := &domain.JobError{
		Code:    code,
		Message: msg,
		Details: []domain.ErrorDetail{
			{Issue: cause.Error()},
		},
	}
	_ = uc.jobRepo.UpdateStatus(ctx, job.ID, domain.StatusFailed, "", jobErr)

	uc.metrics.RecordConversionResult(ctx, job.SourceFormat, job.TargetFormat, domain.StatusFailed)
	uc.logger.Error(ctx, "Job transicionado para status failed",
		ports.Field{Key: "source_format", Value: job.SourceFormat},
		ports.Field{Key: "target_format", Value: job.TargetFormat},
		ports.Field{Key: "error_code", Value: code},
		ports.Field{Key: "error_message", Value: msg},
	)
	return cause
}

func getContentType(format domain.Format) string {
	switch format {
	case domain.FormatMP3:
		return "audio/mpeg"
	case domain.FormatWAV:
		return "audio/wav"
	case domain.FormatPDF:
		return "application/pdf"
	case domain.FormatJPG:
		return "image/jpeg"
	case domain.FormatPNG:
		return "image/png"
	default:
		return "application/octet-stream"
	}
}
