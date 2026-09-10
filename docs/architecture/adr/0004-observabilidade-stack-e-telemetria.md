# ADR 0004: Escolha da Stack de Observabilidade e Estratégia de Telemetria

* **Status:** Aceito
* **Data:** 2026-09-10
* **Autor:** Arquiteto de Software

---

## 1. Contexto
A especificação `docs/specs/observabilidade-conversao/requirements.md` estabelece os requisitos operacionais e de negócio para a **FileConvert API**, exigindo:
- **Rastreabilidade por job:** Reconstrução completa da linha do tempo por `job_id` (`upload` → `pending` → `processing` → `done`/`failed`).
- **Métricas de negócio:** Taxa de conversão por par de formato, tempo médio e p95 de conversão, e profundidade da fila de jobs.
- **Alertabilidade:** Notificação proativa para aumento de falhas, retenção na fila e timeouts de SLA (60s).
- **Restrição de Privacidade:** Proibição estrita de coleta de conteúdo de arquivo, PII ou nomes de arquivos originais.

Para atender a esses requisitos sem violar os princípios da **Arquitetura Hexagonal (Ports & Adapters)** adotada no projeto, é necessário definir a stack tecnológica e a abstração de telemetria.

---

## 2. Opções Consideradas

### Opção A: Logs não estruturados em formato texto e endpoints HTTP ad-hoc para métricas
- *Prós:* Implementação trivial e sem dependências externas.
- *Contras:* Torna impossível a agregação automatizada de métricas por formato e por percentis (p95); dificulta a correlação automatizada por `job_id`; inviabiliza auditorias e alertas automatizados em ambiente cloud-native.

### Opção B: Acoplamento direto dos SDKs (Prometheus client, Zap/Zerolog, OpenTelemetry SDK) no núcleo do domínio e casos de uso
- *Prós:* Desenvolvimento inicial rápido sem necessidade de definir contratos de interface intermediários.
- *Contras:* Viola diretamente a Arquitetura Hexagonal da aplicação; acopla a camada de domínio e regras de negócio a bibliotecas de infraestrutura; dificulta a criação de mocks para testes unitários.

### Opção C: Stack Padrão Open-Source (Prometheus + OpenTelemetry + JSON Log em stdout) desacoplada via Ports Hexagonais
- *Prós:* 
  - Segue o modelo **Twelve-Factor App** emitindo logs estruturados em JSON para `stdout` (`zerolog` ou `slog` encapsulado no adaptador).
  - Adota o padrão de mercado **Prometheus** para exportação de métricas via rota HTTP restrita `/metrics`.
  - Utiliza o padrão **OpenTelemetry (OTLP)** para rastreamento distribuído (tracing), permitindo integração com ferramentas como Jaeger ou Tempo.
  - Mantém o domínio e os casos de uso 100% isolados de detalhes de instrumentação, dependendo apenas dos novos ports `Logger`, `MetricsRecorder` e `Tracer`.
  - Garante que a sanitização de privacidade (mascaramento de PII e exclusão de conteúdo binário) ocorra de forma centralizada e auditável dentro dos adaptadores de telemetria.
- *Contras:* Requer a manutenção dos novos contratos de interface no pacote `port` e seus respectivos adaptadores no pacote `adapter/telemetry`.

---

## 3. Decisão

Adotar a **Opção C (Stack Padrão Open-Source desacoplada via Ports Hexagonais)**.

### Detalhamento da Arquitetura Escolhida:
1. **Logs Estruturados:** A aplicação emitirá logs em formato JSON padronizado para `stdout` através da implementação do port `port.Logger` (`zerolog` como adaptador). Todo log conterá obrigatoriamente os campos `"job_id"` (quando em contexto de job), `"source_format"`, `"target_format"` e `"trace_id"`.
2. **Métricas:** As métricas de negócio e infraestrutura serão registradas via port `port.MetricsRecorder` e expostas no formato Prometheus no endpoint HTTP `/metrics` através de um adaptador HTTP dedicado.
3. **Tracing:** O rastreamento de transições e latências será realizado via port `port.Tracer` com propagação de contexto através do `context.Context` de Go e cabeçalhos W3C Trace Context / envelopes de fila.
4. **Propagação de Correlação:** O `job_id` será injetado no `context.Context` no momento da criação do job pelo handler HTTP e propagado até a conclusão pelo worker.

---

## 4. Consequências

* **Positivas:**
  - **Independência de Vendor:** A aplicação pode trocar a ferramenta final de métricas, logs ou tracing (ex.: de Prometheus para Datadog/CloudWatch) alterando apenas os adaptadores, sem modificar nenhuma linha de código de domínio ou caso de uso.
  - **Sanitização Centralizada:** A regra de privacidade ("nunca registrar conteúdo de arquivo ou PII") é garantida nos adaptadores de log e métricas, impedindo vazamentos acidentais por desenvolvedores.
  - **Testabilidade:** Testes unitários de casos de uso e workers podem utilizar *NoOp* ou *Mock* para `Logger`, `MetricsRecorder` e `Tracer` sem necessitar de servidores de métricas em memória.
  - **Compatibilidade Cloud Native:** Alinhamento total com ecossistemas Kubernetes, Docker e coletores OTel.

* **Negativas / Mitigações:**
  - *Sobrecarga de Código de Abstração:* Adição de interfaces `Logger`, `MetricsRecorder` e `Tracer` no pacote `port`. (Mitigação: Manter as interfaces enxutas, com métodos fortemente tipados focados nos casos de uso do negócio).
