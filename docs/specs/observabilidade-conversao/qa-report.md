# Relatório de Garantia de Qualidade (QA Report) — Observabilidade e Telemetria

* **Data da Avaliação:** 2026-09-10
* **Papel:** Analista de QA / Dev Senior (Revisão Pós-QA)
* **Escopo Avaliado:** `internal/ports`, `internal/adapters/observability`, `internal/usecase`, `internal/delivery/http` e `cmd/`

---

## 1. Resumo da Avaliação

Foi realizada a auditoria e validação da infraestrutura de observabilidade da **FileConvert API**, cobrindo o rastreamento de jobs por `job_id`, exportação de métricas no formato Prometheus no endpoint `/metrics`, tratamento de erros em casos de falha (arquivo corrompido/timeout) e cumprimento estrito da política de privacidade e proteção de dados.

A validação contemplou:
1. **Rastreabilidade e Correlação:** Reconstrução da linha do tempo do ciclo de vida assíncrono de um job através do log JSON estruturado via `job_id`.
2. **Exposição de Métricas:** Formato textual e rótulos do endpoint `/metrics` (`fileconvert_conversions_total`, `fileconvert_conversion_duration_seconds`, `fileconvert_edge_requests_total`, `fileconvert_queue_depth`).
3. **Erros e Diagnóstico:** Verificação de context-rich error logging no estado `failed` sem exposição do binário.
4. **Privacidade:** Auditoria estrita contra vazamento de conteúdo binário de arquivos ou nomes de arquivos originais com dados pessoais (PII).

---

## 2. Cobertura de Testes e Resultados da Avaliação

| Cenário de Teste / Requisito | Tipo de Avaliação | Status | Detalhes da Validação |
| :--- | :--- | :--- | :--- |
| **Rastreabilidade por `job_id` (Linha do Tempo)** | Execução e Auditoria de Log | **PASSOU** | Filtrando o log estruturado por `"job_id": "job_xxx"`, é possível reconstruir sequencialmente todas as transições (`upload` $\rightarrow$ `pending` $\rightarrow$ `processing` $\rightarrow$ `done`/`failed`) com timestamps UTC em milissegundos. |
| **Exposição do Endpoint `/metrics`** | Validação de Endpoint HTTP | **PASSOU** | A rota `/metrics` responde HTTP 200 no formato de texto do Prometheus (`version=0.0.4`), contendo os vetores descritos na especificação. |
| **Métrica de Conversão por Par de Formato** | Validação Telemétrica | **PASSOU** | O contador `fileconvert_conversions_total` reflete corretamente as labels `source_format`, `target_format` e `status` após conclusões de jobs. |
| **Métricas de Requisições de Borda (Edge)** | Validação Telemétrica | **PASSOU** | Requisições HTTP aceitas (202) e rejeitadas (400, 413, 415) incrementam o contador `fileconvert_edge_requests_total` com o respectivo `error_code`. |
| **Log de Erro de Arquivo Corrompido** | Teste de Falha | **PASSOU** | Submeter arquivo corrompido gera o status `failed` com log contendo `error_code: "CORRUPTED_FILE"` e mensagem amigável sem expor dados internos do container. |
| **Restrição Absoluta de Privacidade** | Auditoria de Segurança | **PASSOU** | Nenhum byte de conteúdo binário do usuário, texto interno ou nome original do arquivo enviado (`header.Filename`) aparece em logs ou rótulos de métricas. |

---

## 3. Exemplo de Linha do Tempo Reconstruída por `job_id`

Abaixo está a demonstração da reconstrução da linha do tempo extraída da auditoria de logs estruturados em JSON para um job no fluxo feliz (`wav` $\rightarrow$ `mp3`):

```json
{"level":"INFO","time":"2026-09-10T18:01:22.100Z","job_id":"job_a1b2c3d4","correlation_id":"req-998877","message":"Recepção de job de áudio efetuada com sucesso (status pending)","source_format":"wav","target_format":"mp3","file_size_bytes":1048576}
{"level":"INFO","time":"2026-09-10T18:01:22.150Z","job_id":"job_a1b2c3d4","message":"Worker desenfileirou job para processamento"}
{"level":"INFO","time":"2026-09-10T18:01:22.160Z","job_id":"job_a1b2c3d4","message":"Job transicionado para status processing","source_format":"wav","target_format":"mp3"}
{"level":"INFO","time":"2026-09-10T18:01:23.580Z","job_id":"job_a1b2c3d4","message":"Job concluído com sucesso (status done)","source_format":"wav","target_format":"mp3","converted_size_bytes":524288,"duration_ms":1420}
```

---

## 4. Apuração de Lacunas Identificadas (Requisitos do `requirements.md` Não Totalmente Atendidos)

Após auditoria minuciosa de comparativo entre o `requirements.md` / `design.md` e a implementação dos adaptadores, foram apontadas as seguintes lacunas operacionais:

1. **Histograma de Durabilidade com Buckets Explícitos no Prometheus (`_bucket{le="..."}`):**
   - *Requisito:* O `design.md` estipula a exportação de histogramas com buckets predefinidos (`le="0.1"`, `le="0.5"`, `le="1.0"`, ..., `le="60.0"`) para viabilizar o cálculo do percentil 95 via `histogram_quantile(0.95, ...)`.
   - *Situação Atual:* A implementação atual do adaptador exporta os acumuladores `_sum` e `_count` (formato Summary), permitindo calcular a latência média, porém omitindo as faixas de buckets necessárias para o cálculo nativo do p95 no Prometheus.

2. **Cálculo da Métrica de Retenção na Fila (*Queue Dwell Time*):**
   - *Requisito:* O requisito 3.3 determina medir o tempo que o job permanece no estado `pending` aguardando consumo pelo worker.
   - *Situação Atual:* O port `MetricsRecorder` contém o método `RecordJobDwellTime`, porém a fila em memória atual (`ChannelJobQueue`) trafega apenas a string `jobID`. Sem a inclusão do timestamp `enqueued_at` no envelope da fila, a medição de *dwell time* não está sendo disparada pelo worker.

3. **Atualização Dinâmica de Profundidade da Fila (`fileconvert_queue_depth`):**
   - *Requisito:* Monitoramento contínuo da quantidade de jobs pendentes na fila.
   - *Situação Atual:* O método `RecordQueueDepth` está presente no adaptador, porém não há um worker/polling periódico atualizando o indicador de profundidade em tempo real.

---

## 5. Status das Correções Pós-QA (Dev Senior Review)

| Item Apontado no QA | Classificação da Causa Raiz | Status Pós-QA | Ação Realizada / Encaminhamento |
| :--- | :--- | :--- | :--- |
| **1. Buckets de Histograma (`_bucket{le="..."}`)** | Bug de Implementação | **CORRIGIDO** | O adaptador `PrometheusMetricsRecorder` foi corrigido em `internal/adapters/observability/metrics.go` para gerar as linhas de buckets acumulativas e adicionado teste de regressão em `observability_test.go`. |
| **2. Retenção na Fila (*Queue Dwell Time*)** | Lacuna de Design (Contrato do Port) | **REQUER REVISÃO DE DESIGN** | O payload da `JobQueue` trafega apenas `jobID` (string). Alterar a assinatura do port `JobQueue` para trafegar um envelope com `enqueued_at` exige revisão do Arquiteto. Registrado em `docs/specs/observabilidade-conversao/design-review-needed.md`. |
| **3. Atualização Dinâmica de Profundidade da Fila** | Bug de Implementação | **CORRIGIDO** | O adaptador `ChannelJobQueue` em `internal/adapters/queue/memory_queue.go` foi atualizado para disparar `RecordQueueDepth` dinamicamente a cada `Enqueue` e `Dequeue`. |
