# Contratos de Interface e Arquitetura Hexagonal — FileConvert API

## 1. Visão Geral da Arquitetura Hexagonal
A **FileConvert API** adota Arquitetura Hexagonal (Ports & Adapters). O núcleo da aplicação (domínio e casos de uso) é totalmente isolado de detalhes técnicos e ferramentas de conversão nativas (como `ffmpeg`, `LibreOffice`, `ImageMagick/Poppler`).

```
                          +-----------------------------------+
                          |            HTTP Handler           |
                          +-----------------+-----------------+
                                            |
                                            v
+-----------------------+         +-------------------+         +------------------------+
|      JobQueue         | <-----> |   Application /   | <-----> |     JobRepository      |
|   (Inbound/Outbound)  |         |   Domain Services |         |   (Database Adapter)   |
+-----------------------+         +---------+---------+         +------------------------+
                                            |
                                            v
                          +-----------------+-----------------+
                          |     ConverterRegistry /           |
                          |     Converter Ports               |
                          +-----------------+-----------------+
                                            |
                                            v
                          +-----------------+-----------------+
                          |      ObjectStorage Adapter        |
                          |      (S3/MinIO/Local)             |
                          +-----------------------------------+
```

---

## 2. Modelos de Domínio e Schema do Job

### 2.1. Tipos e Enums de Domínio

```go
package domain

import (
	"context"
	"io"
	"time"
)

// Format representa o formato/extensão de um arquivo suportado na conversão.
type Format string

const (
	FormatWAV  Format = "wav"
	FormatMP3  Format = "mp3"
	FormatJPG  Format = "jpg"
	FormatPNG  Format = "png"
	FormatPDF  Format = "pdf"
	FormatDOC  Format = "doc"
	FormatDOCX Format = "docx"
)

// JobStatus indica o estado atual do processamento de um job assíncrono.
type JobStatus string

const (
	StatusPending    JobStatus = "pending"
	StatusProcessing JobStatus = "processing"
	StatusDone       JobStatus = "done"
	StatusFailed     JobStatus = "failed"
)
```

### 2.2. Estrutura do Job

```go
// Job representa a entidade principal de conversão assíncrona.
type Job struct {
	ID              string     `json:"job_id"`
	SourceFormat    Format     `json:"source_format"`
	TargetFormat    Format     `json:"target_format"`
	SourceObjectKey string     `json:"source_object_key"`
	TargetObjectKey string     `json:"target_object_key,omitempty"`
	Status          JobStatus  `json:"status"`
	Error           *JobError  `json:"error,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// JobError detalha a causa de falha quando o job transiciona para o status "failed".
type JobError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details []ErrorDetail `json:"details,omitempty"`
}
```

### 2.3. Contrato Padrão de Resposta de Erro da API (HTTP Síncrono)

```go
// APIErrorResponse representa o formato JSON padronizado retornado em falhas síncronas.
type APIErrorResponse struct {
	Code      string        `json:"code"`
	Message   string        `json:"message"`
	Details   []ErrorDetail `json:"details,omitempty"`
	Timestamp string        `json:"timestamp"`
}

// ErrorDetail fornece informações contextuais específicas de um erro.
type ErrorDetail struct {
	Field string `json:"field,omitempty"`
	Issue string `json:"issue"`
}
```

#### Mapeamento de Códigos de Erro Padrão:

| Código HTTP | Code String | Descrição | Estágio |
| :--- | :--- | :--- | :--- |
| `400 Bad Request` | `EMPTY_FILE` | Arquivo enviado possui 0 bytes. | Síncrono (Upload) |
| `400 Bad Request` | `INVALID_INPUT` | Parâmetros de requisição malformados. | Síncrono (Upload) |
| `401 Unauthorized` | `UNAUTHORIZED` | API key ausente, malformada ou inválida. | Síncrono (Auth) |
| `403 Forbidden` | `FORBIDDEN` | API key sem permissão para acessar o recurso. | Síncrono (Auth) |
| `413 Payload Too Large` | `FILE_TOO_LARGE` | Arquivo excede o limite de 50 MB. | Síncrono (Upload) |
| `415 Unsupported Media Type` | `UNSUPPORTED_FORMAT` | Formato de origem/destino não suportado no escopo v1. | Síncrono (Upload) |
| `429 Too Many Requests` | `RATE_LIMIT_EXCEEDED` | Limite de requisições por segundo/minuto excedido (rejeição com `Retry-After`). | Síncrono (RateLimit) |
| `429 Too Many Requests` | `CONCURRENT_JOBS_EXCEEDED` | Quota de jobs simultâneos em processamento atingida (rejeição com `Retry-After`). | Síncrono (RateLimit) |
| N/A (`status: failed`) | `CORRUPTED_FILE` | Falha na leitura/decodificação do binário pelo worker. | Assíncrono (Worker) |
| N/A (`status: failed`) | `JOB_TIMEOUT` | Processamento excedeu o limite máximo de 60 segundos. | Assíncrono (Worker) |
| `500 Internal Error` | `INTERNAL_ERROR` | Falha inesperada de infraestrutura. | Síncrono / Assíncrono |

---

## 3. Contratos de Interface (Ports em Go)

### 3.1. `Converter` Port
Interface implementada por cada driver adaptador de conversão nativo (ex: `FFmpegAudioConverter`, `LibreOfficeDocConverter`).

```go
package port

import (
	"context"
	"io"

	"github.com/file-convert-api/domain"
)

// Converter define a interface de conversão de arquivos.
type Converter interface {
	// CanConvert verifica se a implementação é capaz de converter de srcFormat para targetFormat.
	CanConvert(srcFormat, targetFormat domain.Format) bool

	// Convert executa a conversão do stream de entrada e retorna o stream convertido.
	// O contexto deve ser utilizado para controlar timeouts (SLA de 60s).
	Convert(ctx context.Context, src io.Reader, srcFormat, targetFormat domain.Format) (io.Reader, error)
}
```

### 3.2. `ConverterRegistry`
Interface responsável pelo gerenciamento e busca dinâmica do `Converter` adequado para o par `(SourceFormat, TargetFormat)` em runtime.

```go
// ConverterRegistry gerencia os conversores disponíveis na aplicação.
type ConverterRegistry interface {
	// Register registra uma nova implementação de Converter.
	Register(converter Converter) error

	// GetConverter localiza o Converter capaz de realizar a conversão solicitada.
	// Retorna domain.ErrUnsupportedFormat caso nenhum conversor seja encontrado.
	GetConverter(srcFormat, targetFormat domain.Format) (Converter, error)
}
```

**Estratégia de Resolução em Runtime:**
1. A aplicação inicializa e registra todas as instâncias dos adaptadores de conversão no startup via `Register()`.
2. Ao receber um job, o worker solicita `GetConverter(srcFormat, targetFormat)` ao registry.
3. O registry percorre os conversores registrados e retorna a primeira implementação cujo método `CanConvert(srcFormat, targetFormat)` retorne `true`.
4. Se nenhum conversor atender ao par, um erro sintático de suporte é retornado imediatamente.

---

### 3.3. `JobRepository` Port
Port de persistência para gerenciamento do estado dos jobs.

```go
// JobRepository abstrai a persistência e consulta dos jobs de conversão.
type JobRepository interface {
	// Create persiste um novo job no estado inicial 'pending'.
	Create(ctx context.Context, job *domain.Job) error

	// GetByID busca um job pelo seu ID único.
	GetByID(ctx context.Context, id string) (*domain.Job, error)

	// UpdateStatus atualiza o status do job, chave do objeto convertido e erros associados.
	UpdateStatus(ctx context.Context, id string, status domain.JobStatus, targetObjectKey string, jobErr *domain.JobError) error
}
```

---

### 3.4. `ObjectStorage` Port
Port para armazenamento de objetos (arquivos originais enviados e arquivos convertidos gerados).

```go
// ObjectStorage abstrai o armazenamento de arquivos binários e gerenciamento de tempo de vida.
type ObjectStorage interface {
	// Upload armazena um arquivo no storage.
	Upload(ctx context.Context, key string, content io.Reader, size int64, contentType string) error

	// UploadWithTTL armazena um arquivo atribuindo um tempo de retenção/expiração pré-definido (TTL).
	UploadWithTTL(ctx context.Context, key string, content io.Reader, size int64, contentType string, ttl time.Duration) error

	// Download recupera o stream de leitura de um arquivo armazenado.
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete remove um objeto do storage.
	Delete(ctx context.Context, key string) error

	// DeleteExpired varre e remove objetos cuja retenção excedeu a janela estipulada.
	// Em adaptadores S3/MinIO com regras de Lifecycle ativas, este método atua como fallback/purga manual.
	// Em adaptadores de sistema de arquivos local, realiza a limpeza dos arquivos expirados em disco.
	DeleteExpired(ctx context.Context, retentionWindow time.Duration) (int64, error)
}
```

---

### 3.5. `JobQueue` Port
Port de mensageria/filas para processamento assíncrono.

```go
// JobQueue abstrai o enfileiramento e consumo de jobs.
type JobQueue interface {
	// Enqueue envia o ID do job para a fila de processamento assíncrono.
	Enqueue(ctx context.Context, jobID string) error

	// Dequeue retira um jobID da fila para ser processado pelo worker.
	// Retorna uma função de ack para confirmar o sucesso do processamento.
	Dequeue(ctx context.Context) (jobID string, ack func() error, err error)
}
```

---

### 3.6. `Logger` Port
Port para registro de logs estruturados e contextuais sem acoplamento direto a bibliotecas como `zerolog` ou `zap`.

```go
// Field representa um atributo estruturado chave-valor para log.
type Field struct {
	Key   string
	Value any
}

// Logger define o contrato para emissores de logs estruturados.
// Regra Estrita de Privacidade: Nenhum log pode registrar conteúdo de arquivo ou PII (ex.: nome de arquivo fornecido pelo cliente).
// Apenas metadados sanitizados (ex.: job_id, source_format, target_format, file_size_bytes, duration_ms) são permitidos.
type Logger interface {
	Debug(ctx context.Context, msg string, fields ...Field)
	Info(ctx context.Context, msg string, fields ...Field)
	Warn(ctx context.Context, msg string, fields ...Field)
	Error(ctx context.Context, msg string, fields ...Field)

	// With retorna um novo Logger contendo os atributos base fixados.
	With(fields ...Field) Logger
}
```

---

### 3.7. `MetricsRecorder` Port
Port para captura de telemetria de métricas de negócio e performance sem dependência direta do SDK do Prometheus.

```go
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
```

---

### 3.8. `Tracer` Port
Port para geração de spans de rastreamento distribuído (tracing) desacoplado do OpenTelemetry.

```go
// Span abstrai uma unidade de trabalho delimitada temporalmente na execução.
type Span interface {
	// End finaliza a duração do span.
	End()

	// SetAttribute adiciona metadados estruturados ao span.
	SetAttribute(key string, value any)

	// RecordError registra a falha/exceção associada ao span.
	RecordError(err error)
}

// Tracer define a interface para criação e propagação de spans.
type Tracer interface {
	// StartSpan inicia um novo span filho associado ao contexto fornecido.
	StartSpan(ctx context.Context, operationName string) (context.Context, Span)
}
```

---

### 3.9. Propagação de Contexto de Correlação (`job_id` e Trace Context)

Para garantir a rastreabilidade ponta a ponta sem violar o encapsulamento, os adaptadores de observabilidade utilizam chaves privadas em `context.Context` para propagar identificadores contextuais:

```go
type contextKey string

const (
	JobIDContextKey         contextKey = "job_id"
	CorrelationIDContextKey contextKey = "correlation_id"
)

// WithJobID injeta o job_id no contexto de execução.
func WithJobID(ctx context.Context, jobID string) context.Context {
	return context.WithValue(ctx, JobIDContextKey, jobID)
}

// JobIDFromContext recupera o job_id do contexto de execução, se presente.
func JobIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(JobIDContextKey).(string)
	return id, ok
}
```

---

### 3.10. `AuthProvider` Port
Port para autenticação e validação de chaves de API (`API Keys`).

```go
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
```

---

### 3.11. `RateLimiter` Port
Port para controle de vazão por chave de API/IP e limitação de jobs simultâneos em processamento.

```go
// RateLimitResult especifica o estado atual de consumo de limites de requisição ou concorrência.
type RateLimitResult struct {
	Allowed           bool          `json:"allowed"`
	Limit             int           `json:"limit"`
	Remaining         int           `json:"remaining"`
	ResetDuration     time.Duration `json:"reset_duration"`      // Duração da janela até o reset
	RetryAfterSeconds int           `json:"retry_after_seconds"` // Valor numérico para o cabeçalho HTTP Retry-After
	Reason            string        `json:"reason,omitempty"`    // "rate_limit_exceeded" ou "concurrent_jobs_exceeded"
}

// RateLimiter abstrai os algoritmos de controle de vazão (Leaky Bucket / Token Bucket) e semáforo distribuído.
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
```

---

### 3.12. Política de Retenção de Dados e Expiração no Storage
Conforme detalhado no [ADR 0008: Política de Retenção de Dados, Expiração de Storage e Privacidade de Logs e Métricas](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/adr/0008-retencao-de-dados.md):

1. **TTL dos Arquivos no Storage:**
   - **Arquivo Original (`source/`):** Deletado imediatamente via `Delete()` após o sucesso do job. Para jobs falhos/abandonados, expira via TTL de 24 horas (`24h`).
   - **Arquivo Convertido (`target/`):** Mantido no storage para download por no máximo **24 horas (`24h`)**, sendo purgado via *Bucket Lifecycle Policy* (S3/MinIO) ou pelo método `DeleteExpired()` (Filesystem local).
2. **Privacidade Operacional:**
   - **Conteúdo de arquivo:** Zero gravação ou inspeção de payloads nos adaptadores de log, métricas ou tracing.
   - **Identificadores Sanitizados:** Apenas `object_key` internos (UUID v4) e metadados estruturados são mantidos.

---

### 3.13. Arquitetura de Conversão Imagem ↔ PDF e Suporte a ZIP
Conforme estabelecido no [ADR 0009: Arquitetura de Conversão Imagem ↔ PDF e Tratamento de Saída Multipágina em ZIP](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/adr/0009-conversao-imagem-pdf.md):

1. **Desacoplamento via Subprocessos:**
   - `.jpg`/`.png` → `.pdf`: Implementado via `ImageToPDFConverter` utilizando o binário ImageMagick (`convert`).
   - `.pdf` → `.jpg`: Implementado via `PDFToImageConverter` utilizando o binário `pdftoppm` do pacote `poppler-utils` em **300 DPI**.
2. **Saída Multipágina em Arquivo ZIP:**
   - Para documentos PDF contendo de 2 a 50 páginas, o adaptador `PDFToImageConverter` gera as imagens individuais numeradas (`page-1.jpg`, `page-2.jpg`, ..., `page-N.jpg`) e as compacta em um único arquivo **`.zip`**.
   - O arquivo `.zip` é gravado no `ObjectStorage` sob a chave `targets/<job_id>.zip` e atribuído ao campo `TargetObjectKey`.
   - **Garantia Arquitetural:** Preserva 100% dos contratos das interfaces `JobRepository` e `ObjectStorage` sem requerer alterações no schema da entidade `domain.Job`.

---

## 4. Registro de Ambiguidades Tecnicamente Identificadas no `requirements.md`

Conforme as diretrizes de governança de arquitetura, foram identificadas as seguintes ambiguidades no `requirements.md` que exigem esclarecimento futuro:

1. **Mecanismo de Webhook vs. Polling:**
   - *Ambiguidade:* O documento de requisitos menciona que o cliente "(ou recebe webhook)", porém não especifica o contrato do payload do webhook, cabeçalhos de assinatura de segurança, política de retentativa (*backoff/retry*) ou endpoint de cadastro da URL de callback.
   - *Decisão de Arquitetura:* Na versão v1, a aplicação suportará prioritariamente o modelo de **Polling** (`GET /v1/jobs/{job_id}`). A infraestrutura de webhook será mantida como funcionalidade pendente de especificação dedicada antes da sua implementação.

2. **Resultado de PDF para JPG com Múltiplas Páginas:**
   - *Ambiguidade:* Na conversão `.pdf` → `.jpg`, o requisito prevê "disponibilizando a imagem resultante (ou pacote de imagens das páginas)". Para PDFs com mais de 1 página, gerar múltiplos arquivos `.jpg` conflita com o campo único `TargetObjectKey` no schema do Job.
   - *Decisão de Arquitetura:* Conforme ratificado no [ADR 0009](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/adr/0009-conversao-imagem-pdf.md), quando um PDF multipáginas for convertido para JPG, a saída será empacotada em um arquivo compactado `.zip` contendo as imagens numeradas (`page-1.jpg`, `page-2.jpg`), permitindo a manutenção do contrato de chave de objeto única no `TargetObjectKey`.
