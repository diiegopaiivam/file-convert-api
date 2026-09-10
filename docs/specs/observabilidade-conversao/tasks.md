# Plano de Tarefas — Observabilidade e Telemetria

Este documento contém a quebra detalhada das tarefas necessárias para implementar a infraestrutura de observabilidade da **FileConvert API** conforme os requisitos e o design de arquitetura.

---

## Fase 1: Abstração de Interfaces e Correlação de Contexto (`ports`)

- [x] **1.1. Adicionar Interfaces de Telemetria no Pacote `ports`**
  - **Descrição:** Definir as interfaces `Logger`, `MetricsRecorder`, `Tracer` e structs de contexto (`Field`) em `internal/ports/telemetry.go`.
  - **Critério de Aceite:** Interfaces compilando sem dependências de pacotes externos como Prometheus ou Zerolog.
  - **Arquivo:** `internal/ports/telemetry.go`

- [x] **1.2. Implementar Utilitários de Propagação de Contexto (`job_id`)**
  - **Descrição:** Criar funções auxiliares `WithJobID(ctx, jobID)` e `JobIDFromContext(ctx)` para gerenciar a injeção do `job_id` no `context.Context`.
  - **Critério de Aceite:** Teste unitário garantindo que a chave de contexto é privada e recupera corretamente o valor de string.
  - **Arquivo:** `internal/ports/context.go`

---

## Fase 2: Implementação dos Adaptadores de Telemetria (`adapters/observability`)

- [x] **2.1. Criar Adaptador de Logging Estruturado (`JSONLogger`)**
  - **Descrição:** Implementar a interface `ports.Logger` utilizando `log/slog`, configurando a saída JSON para `stdout`. Extrair automaticamente `job_id` e `correlation_id` do contexto fornecido em cada nível de log (`Info`, `Error`, etc.).
  - **Critério de Aceite:** Todos os logs gerados saem em formato JSON estruturado contendo a chave `"job_id"` quando presente no contexto.
  - **Arquivo:** `internal/adapters/observability/logger.go`

- [x] **2.2. Criar Adaptador de Métricas Prometheus (`PrometheusMetricsRecorder`)**
  - **Descrição:** Implementar a interface `ports.MetricsRecorder`. Registrar os contadores (`fileconvert_conversions_total`, `fileconvert_edge_requests_total`), histogramas (`fileconvert_conversion_duration_seconds`, `fileconvert_job_queue_dwell_time_seconds`) e gauges (`fileconvert_queue_depth`).
  - **Critério de Aceite:** Registro no Prometheus padrão sem panics em concorrência; rota `/metrics` expondo todos os vetores descritos na especificação.
  - **Arquivo:** `internal/adapters/observability/metrics.go`

- [x] **2.3. Criar Adaptador NoOp / Mock para Testes Unitários**
  - **Descrição:** Criar implementações nulas (`NoOpLogger`, `NoOpMetricsRecorder`, `NoOpTracer`) para facilitar a injeção de dependências em testes unitários sem poluir a saída dos testes.
  - **Critério de Aceite:** Testes unitários do domínio executando limpos sem falhas de ponteiro nulo.
  - **Arquivo:** `internal/adapters/observability/noop.go`

---

## Fase 3: Instrumentação do Servidor HTTP e da Fila (`delivery/http` e `middleware`)

- [x] **3.1. Criar Middleware HTTP de Observabilidade e Borda**
  - **Descrição:** Desenvolver middleware HTTP para capturar tempo de resposta das requisições, gerar `correlation_id`, registrar métricas de borda via `RecordEdgeRequest` (erros 400, 413, 415, 500) e injetar contexto de rastreamento.
  - **Critério de Aceite:** Todas as requisições HTTP retornam com cabeçalho `X-Correlation-ID` e disparam contadores de requisições de borda.
  - **Arquivo:** `internal/delivery/http/middleware.go`

- [x] **3.2. Exposição da Rota `/metrics` no Servidor HTTP**
  - **Descrição:** Registrar o endpoint `/metrics` no router do servidor HTTP expondo as métricas acumuladas no formato Prometheus.
  - **Critério de Aceite:** Endpoint `/metrics` retornando HTTP 200 no formato de texto do Prometheus.
  - **Arquivo:** `internal/delivery/http/router.go`

---

## Fase 4: Instrumentação de Workers e Conversores Assíncronos

- [x] **4.1. Instrumentar o Ciclo de Vida do Worker e Usecase**
  - **Descrição:** Atualizar o loop de consumo do worker e o usecase `ProcessJobUseCase` para registrar transições de estado (`processing` → `done`/`failed`), medindo a latência ativa do conversor (`RecordConversionDuration`) e contabilizando resultados (`RecordConversionResult`).
  - **Critério de Aceite:** Logs do worker contêm `"job_id"`, `"source_format"`, `"target_format"` e métricas são incrementadas ao final de cada job.
  - **Arquivo:** `internal/usecase/process_job.go` e `cmd/worker/main.go`

- [x] **4.2. Auditoria e Sanitização de Privacidade**
  - **Descrição:** Revisar todos os pontos de log e tratamento de erros do conversor nativo (FFmpeg, LibreOffice, Poppler) garantindo que nenhum caminho de arquivo do usuário, PII ou conteúdo seja exposto.
  - **Critério de Aceite:** Confirmação por código de que são emitidos apenas metadados técnicos sanitizados (`source_format`, `target_format`, `file_size_bytes`, `duration_ms`, `error_code`).
  - **Arquivo:** `internal/adapters/observability/` e `internal/usecase/`

---

## Fase 5: Validação e Testes E2E

- [x] **5.1. Implementar Testes Unitários de Observabilidade**
  - **Descrição:** Escrever testes unitários acionando os adaptadores `JSONLogger` e `PrometheusMetricsRecorder`, testando o endpoint `/metrics` e a propagação de context.
  - **Critério de Aceite:** Sucesso na execução dos testes de observabilidade.
  - **Arquivo:** `internal/adapters/observability/observability_test.go`
