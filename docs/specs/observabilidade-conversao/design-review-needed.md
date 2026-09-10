# Itens que Requerem Revisão de Design da Arquitetura — Observabilidade

Este documento registra as lacunas operacionais apontadas no relatório de QA que dependem de definições do Arquiteto ou alteração nos contratos de interface (`ports.md`).

---

## 1. Cálculo da Métrica de Retenção na Fila (*Queue Dwell Time*)

- **Descrição do Problema:** O requisito 3.3 de `requirements.md` e a especificação de métricas de `design.md` exigem medir o tempo de permanência (*dwell time*) que o job fica no estado `pending` aguardando consumo pelo worker.
- **Situação Atual do Código:** O port `JobQueue` (`internal/ports/queue.go`) define a assinatura do método `Enqueue(ctx context.Context, jobID string) error` e `Dequeue(ctx context.Context) (jobID string, ack func() error, err error)`, trafegando apenas o `jobID` como string.
- **Por que está fora do escopo de desenvolvimento decidir:** Para calcular o *dwell time*, o envelope da mensagem publicado na fila precisa conter o timestamp `enqueued_at` ou a struct do envelope precisa ser adicionada ao port `JobQueue`. Alterar o contrato de interface `JobQueue` em `ports.md` é uma decisão de arquitetura restrita ao Arquiteto.

---

## 2. Decisão de Infraestrutura para Drivers Reais de Fila (Postgres / Redis / RabbitMQ)

- **Descrição do Problema:** O consumo contínuo da fila e medição de profundeza por trabalhadores em produção foi validado com fakes em memória.
- **Por que está fora do escopo de desenvolvimento decidir:** A escolha do driver final de infraestrutura (RabbitMQ vs. Redis vs. Postgres) e o provisionamento do container dependem de ADR e revisão de design pelo Arquiteto.
