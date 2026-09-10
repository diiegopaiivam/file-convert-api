# Design de Arquitetura — Observabilidade e Telemetria

## 1. Visão Geral da Solução

A arquitetura de observabilidade da **FileConvert API** foi projetada para prover rastreabilidade total de jobs assíncronos, coleta de métricas de negócio por par de formato e alertas proativos de saúde operacional, sem violar os princípios da Arquitetura Hexagonal ou colocar em risco a privacidade dos arquivos dos usuários.

O sistema abstrai o registro de logs, métricas e traces através dos ports `Logger`, `MetricsRecorder` e `Tracer`, garantindo que os casos de uso de conversão e workers dependam apenas de contratos em Go, enquanto as integrações concretas (Prometheus, OpenTelemetry, Zerolog) residem exclusivamente na camada de adaptadores (`adapter/telemetry`).

---

## 2. Componentes e Fluxo de Execução Telemétrica

### 2.1. Componentes Envolvidos
- **HTTP Observability Middleware:** Intercepta requisições HTTP de entrada, gera/propaga o `correlation_id`, mede latência HTTP e registra métricas de borda (`RecordEdgeRequest`).
- **Context Propagator (`WithJobID` / `JobIDFromContext`):** Gerencia a injeção e extração do `job_id` e `trace_id` no `context.Context` de Go.
- **Queue Envelope Telemetry Decorator:** Serializa os metadados de rastreamento (`job_id`, `trace_parent`) no envelope das mensagens da `JobQueue` e os reconstrói ao desenfileirar no worker.
- **`zerolog` Adapter (`adapter/telemetry/logger`):** Implementa `port.Logger`, formatando logs estruturados em JSON para `stdout` com os campos contextuais extraídos do `context.Context`.
- **Prometheus Adapter (`adapter/telemetry/metrics`):** Implementa `port.MetricsRecorder`, registrando contadores, histogramas e gauges no registro padrão do Prometheus e expondo a rota `/metrics`.
- **OpenTelemetry Adapter (`adapter/telemetry/tracer`):** Implementa `port.Tracer`, gerenciando a criação e finalização de spans distribuídos.

### 2.2. Diagrama de Sequência de Propagação e Telemetria

```
Client / HTTP Request        HTTP Handler         JobQueue          Audio/Doc Worker       Metrics/Logger Adapters
        |                         |                  |                     |                         |
        |--- POST /conversions -->|                  |                     |                         |
        |    (gerar job_id)       |-- Inject Context |                     |                         |
        |                         |   (job_id)       |                     |                         |
        |                         |-- Enqueue Job -->|                     |                         |
        |                         |   (with trace)   |                     |                         |
        |                         |-- Record Edge Metric ------------------------------------------>|
        |<-- 202 Accepted (job_id)-|                  |                     |                         |
        |                         |                  |-- Dequeue Job ----->|                         |
        |                         |                  |   (rebuild context) |                         |
        |                         |                  |                     |-- Log ("job_processing")-->|
        |                         |                  |                     |-- Start Span ---------->|
        |                         |                  |                     |-- Run Converter (FFmpeg)|
        |                         |                  |                     |-- Record Dwell Time --->|
        |                         |                  |                     |-- Record Duration ----->|
        |                         |                  |                     |-- Record Status (done)->|
```

---

## 3. Estratégia de Correlação e Rastreabilidade

### 3.1. Injeção e Extração do `job_id` no Contexto
No momento em que a requisição de conversão é aceita e o `job_id` é gerado, a camada de aplicação injeta o identificador no contexto de execução:

```go
ctx = port.WithJobID(ctx, job.ID)
```

Toda chamada aos adaptadores `Logger`, `MetricsRecorder` e `Tracer` recebe o `ctx`. O adaptador de logging insere automaticamente o `job_id` no payload JSON retornado:

```json
{
  "level": "info",
  "timestamp": "2026-09-10T18:00:05.123Z",
  "job_id": "job_123456789",
  "source_format": "wav",
  "target_format": "mp3",
  "message": "Conversão finalizada com sucesso",
  "duration_ms": 1420
}
```

### 3.2. Propagação através da Fila de Mensagens (`JobQueue`)
Para manter a rastreabilidade entre o servidor HTTP e os workers assíncronos, o payload ou cabeçalho do envelope de mensagem publicado na `JobQueue` conterá:

```json
{
  "job_id": "job_123456789",
  "correlation_id": "req-987654321",
  "enqueued_at": "2026-09-10T18:00:00.000Z"
}
```

Ao efetuar o `Dequeue`, o worker calcula o *queue dwell time* (`now - enqueued_at`) e registra a métrica via `RecordJobDwellTime` antes de iniciar a conversão.

---

## 4. Catálogo de Métricas de Negócio (Prometheus Specification)

As métricas exportadas no endpoint `/metrics` seguem a nomenclatura e estrutura abaixo:

### 4.1. `fileconvert_conversions_total`
- **Tipo:** Counter
- **Descrição:** Número total de jobs de conversão processados pelo worker.
- **Labels:** `source_format`, `target_format`, `status` (`done`, `failed`).
- **Exemplo de Saída Prometheus:**
  ```prometheus
  fileconvert_conversions_total{source_format="wav",target_format="mp3",status="done"} 142
  fileconvert_conversions_total{source_format="docx",target_format="pdf",status="failed"} 3
  ```

### 4.2. `fileconvert_conversion_duration_seconds`
- **Tipo:** Histogram
- **Descrição:** Latência em segundos do tempo de processamento ativo pelo conversor nativo.
- **Labels:** `source_format`, `target_format`.
- **Buckets Padrão:** `[0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 20.0, 30.0, 45.0, 60.0]`
- **Exemplo de Saída Prometheus:**
  ```prometheus
  fileconvert_conversion_duration_seconds_bucket{source_format="wav",target_format="mp3",le="5.0"} 138
  fileconvert_conversion_duration_seconds_sum{source_format="wav",target_format="mp3"} 245.5
  fileconvert_conversion_duration_seconds_count{source_format="wav",target_format="mp3"} 142
  ```

### 4.3. `fileconvert_job_queue_dwell_time_seconds`
- **Tipo:** Histogram
- **Descrição:** Tempo (em segundos) que um job permaneceu no estado `pending` na fila antes do worker iniciar a conversão.
- **Labels:** `source_format`, `target_format`.
- **Buckets Padrão:** `[0.05, 0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0]`

### 4.4. `fileconvert_queue_depth`
- **Tipo:** Gauge
- **Descrição:** Número atual de jobs pendentes aguardando consumo na fila.
- **Labels:** `queue_name`.
- **Exemplo de Saída Prometheus:**
  ```prometheus
  fileconvert_queue_depth{queue_name="conversion_jobs"} 12
  ```

### 4.5. `fileconvert_edge_requests_total`
- **Tipo:** Counter
- **Descrição:** Número total de requisições HTTP de entrada tratadas pela API.
- **Labels:** `method`, `endpoint`, `status_code`, `error_code`.
- **Exemplo de Saída Prometheus:**
  ```prometheus
  fileconvert_edge_requests_total{method="POST",endpoint="/v1/conversions/audio",status_code="202",error_code="NONE"} 150
  fileconvert_edge_requests_total{method="POST",endpoint="/v1/conversions/audio",status_code="413",error_code="FILE_TOO_LARGE"} 5
  ```

---

## 5. Garantia Arquitetural de Privacidade

A conformidade com o requisito de privacidade é garantida estruturalmente pelos seguintes pontos de controle:

1. **Assinatura de Interfaces Tipadas:** As interfaces no pacote `port` não aceitam buffers de arquivos, streams de payload ou instâncias de `domain.Job` brutas com dados pessoais. Aceitam apenas os tipos primitivos `sourceFormat`, `targetFormat`, `status`, `duration` e `job_id`.
2. **Filtro de Nome de Arquivo:** Os adaptadores de logging são proibidos de registrar o campo `filename` do cabeçalho `multipart/form-data`. É registrado exclusivamente o tamanho em bytes (`file_size_bytes`) e os formatos declarados.
3. **Mapeamento Estrito de Erros:** Erros retornados ao cliente ou gravados em logs utilizam exclusivamente a struct `JobError` com códigos enumerados (`CORRUPTED_FILE`, `JOB_TIMEOUT`), omitindo dados internos de sistema de arquivos ou trechos do arquivo binário.

---

## 6. Especificação das Regras de Alerta (Prometheus Rules)

Abaixo estão especificadas as regras de alerta configuráveis no Alertmanager para satisfazer os critérios de alerta definidos no `requirements.md`:

```yaml
groups:
  - name: fileconvert_alerts
    rules:
      - alert: HighFormatPairFailureRate
        expr: |
          (
            sum(rate(fileconvert_conversions_total{status="failed"}[5m])) by (source_format, target_format)
            /
            sum(rate(fileconvert_conversions_total[5m])) by (source_format, target_format)
          ) > 0.05
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Taxa de falha elevada para conversão {{ $labels.source_format }} -> {{ $labels.target_format }}"
          description: "A taxa de erro para o par {{ $labels.source_format }} -> {{ $labels.target_format }} ultrapassou 5% nos últimos 5 minutos."

      - alert: QueueStagnantOrGrowing
        expr: fileconvert_queue_depth > 50 and rate(fileconvert_conversions_total[3m]) == 0
        for: 3m
        labels:
          severity: critical
        annotations:
          summary: "Fila de conversão represada sem consumo de workers"
          description: "A fila possui mais de 50 jobs pendentes e nenhuma conversão foi concluída nos últimos 3 minutos."

      - alert: RecurringJobTimeouts
        expr: increase(fileconvert_conversions_total{status="failed", error_code="JOB_TIMEOUT"}[10m]) >= 3
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Estouro de SLA recorrente (Timeouts > 60s)"
          description: "Ocorreram {{ $value }} falhas de timeout nos últimos 10 minutos."
```
