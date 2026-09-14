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

// GetTimeoutForFamily calcula o tempo limite específico da família de conversão conforme ADR 0006.
func GetTimeoutForFamily(sourceFormat, targetFormat domain.Format) time.Duration {
	// Família 1: Áudio (.wav ↔ .mp3) -> 30 segundos
	if (sourceFormat == domain.FormatWAV && targetFormat == domain.FormatMP3) ||
		(sourceFormat == domain.FormatMP3 && targetFormat == domain.FormatWAV) {
		return 30 * time.Second
	}

	// Família 2: Imagem & PDF (.jpg/.png ↔ .pdf) -> 45 segundos
	if ((sourceFormat == domain.FormatJPG || sourceFormat == domain.FormatPNG) && targetFormat == domain.FormatPDF) ||
		(sourceFormat == domain.FormatPDF && targetFormat == domain.FormatJPG) {
		return 45 * time.Second
	}

	// Família 3: Documento para PDF (.doc/.docx → .pdf) -> 60 segundos
	if (sourceFormat == domain.FormatDOC || sourceFormat == domain.FormatDOCX) && targetFormat == domain.FormatPDF {
		return 60 * time.Second
	}

	// Teto máximo do SLA global
	return 60 * time.Second
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

	// 3. Validar que o par de formatos pertence à Whitelist Fechada da v1
	if !domain.IsValidConversionPair(job.SourceFormat, job.TargetFormat) {
		return uc.failJob(ctx, job, domain.ErrCodeUnsupportedFormat, "Par de formatos de conversão não suportado na v1", domain.ErrUnsupportedFormat)
	}

	// 4. Obter o conversor no registry
	converter, err := uc.registry.GetConverter(job.SourceFormat, job.TargetFormat)
	if err != nil {
		return uc.failJob(ctx, job, domain.ErrCodeUnsupportedFormat, "Formato de arquivo não suportado", err)
	}

	// 5. Baixar o arquivo de origem do ObjectStorage
	rawSrcStream, err := uc.storage.Download(ctx, job.SourceObjectKey)
	if err != nil {
		return uc.failJob(ctx, job, domain.ErrCodeInternalError, "Falha ao recuperar arquivo original do storage", err)
	}
	defer rawSrcStream.Close()

	// 6. Validar os Magic Bytes reais do arquivo (inspeção de cabeçalho binário)
	srcStream, err := domain.ValidateMagicBytes(rawSrcStream, job.SourceFormat)
	if err != nil {
		if errors.Is(err, domain.ErrEmptyFile) {
			return uc.failJob(ctx, job, domain.ErrCodeEmptyFile, "O arquivo enviado está vazio.", err)
		}
		return uc.failJob(ctx, job, domain.ErrCodeCorruptedFile, "O cabeçalho do arquivo enviado é inválido ou corrompido para o formato declarado.", err)
	}

	// 7. Determinar timeout da família de conversão e criar contexto isolado com cancelamento forçado do subprocesso
	familyTimeout := GetTimeoutForFamily(job.SourceFormat, job.TargetFormat)
	conversionCtx, cancel := context.WithTimeout(ctx, familyTimeout)
	defer cancel()

	startTime := time.Now()
	convertedStream, err := converter.Convert(conversionCtx, srcStream, job.SourceFormat, job.TargetFormat)
	duration := time.Since(startTime)

	uc.metrics.RecordConversionDuration(ctx, job.SourceFormat, job.TargetFormat, duration)

	if err != nil {
		if errors.Is(err, domain.ErrCorruptedFile) {
			return uc.failJob(ctx, job, domain.ErrCodeCorruptedFile, "O arquivo enviado está corrompido e não pôde ser processado.", err)
		}
		if errors.Is(err, domain.ErrJobTimeout) || errors.Is(conversionCtx.Err(), context.DeadlineExceeded) {
			return uc.failJob(ctx, job, domain.ErrCodeJobTimeout, fmt.Sprintf("O tempo limite de processamento de %v foi excedido.", familyTimeout), domain.ErrJobTimeout)
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

	// 8. Upload do arquivo convertido para o ObjectStorage
	targetKey := fmt.Sprintf("converted/%s.%s", job.ID, job.TargetFormat)
	contentType := getContentType(job.TargetFormat)

	err = uc.storage.Upload(ctx, targetKey, bytes.NewReader(buf), int64(len(buf)), contentType)
	if err != nil {
		return uc.failJob(ctx, job, domain.ErrCodeInternalError, "Falha ao armazenar arquivo convertido no storage", err)
	}

	// 9. Atualizar status final para 'done'
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
