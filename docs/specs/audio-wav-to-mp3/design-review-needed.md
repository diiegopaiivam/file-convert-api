# Itens que Requerem Revisão de Design da Arquitetura — Conversão de Áudio (.wav ↔ .mp3)

Este documento registra as lacunas apontadas no relatório de QA da feature de conversão de áudio que dependem de definições do Arquiteto ou alteração nos contratos de transporte e infraestrutura.

---

## 1. Validação do Limite Síncrono de Tamanho de Arquivo (50 MB - `FILE_TOO_LARGE`)

- **Descrição do Problema:** O requisito RNF 4.1 de `v1-overview/requirements.md` determina que qualquer upload de arquivo superior a 50 MB deve ser rejeitado síncronamente na recepção HTTP com o código `FILE_TOO_LARGE`.
- **Por que está fora do escopo de desenvolvimento decidir:** O port `Converter` opera sobre `io.Reader` sem conhecimento do limite de upload HTTP da API. A definição do middleware global de limite no transporte HTTP exige padronização pelo Arquiteto.

---

## 2. Endpoints HTTP de Entrada e Consulta (`POST /v1/conversions/audio`, `GET /v1/jobs/{job_id}`)

- **Descrição do Problema:** A integração das rotas de transporte HTTP em ambiente de produção containerizado com roteamento e barramento de mensagens.
- **Por que está fora do escopo de desenvolvimento decidir:** A escolha do roteador HTTP de produção e injeção dos adaptadores em `cmd/api` e `cmd/worker` depende de alinhamento de arquitetura.

---

## 3. Integração Real da Fila com Infraestrutura (Redis/Postgres)

- **Descrição do Problema:** A infraestrutura de mensageria assíncrona foi testada com fakes em memória.
- **Por que está fora do escopo de desenvolvimento decidir:** A escolha do driver de infraestrutura e estratégia de persistência depende de ADR.
