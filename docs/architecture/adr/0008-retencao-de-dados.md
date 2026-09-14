# ADR 0008: Política de Retenção de Dados, Expiração de Storage e Privacidade de Logs e Métricas

* **Status:** Aceito
* **Data:** 2026-09-14
* **Autor:** Arquiteto de Software

---

## 1. Contexto

A **FileConvert API** processa binários de usuários contendo arquivos de áudio, imagem e documento que podem armazenar informações sigilosas, dados corporativos ou dados pessoais (PII - *Personally Identifiable Information*).

Para garantir conformidade legal com regulamentações de privacidade e proteção de dados (como LGPD e GDPR), além de manter custos operacionais de infraestrutura sob controle e prevenir acúmulo desnecessário de objetos no armazenamento temporário:
1. É necessário estabelecer uma política clara de tempo de vida (TTL - *Time-to-Live*) para arquivos binários recebidos e gerados pela aplicação.
2. É imprescindível instituir uma regra arquitetural inviolável quanto à proibição de vazamento de conteúdo de arquivo ou PII nos subsistemas de observabilidade (logs, métricas e tracing).
3. O contrato da interface hexagonal `ObjectStorage` precisa comportar primitivas de expiração e expurgo para garantir suporte tanto a storages baseados em nuvem com regras de Lifecycle nativas (ex.: AWS S3, MinIO) quanto a adaptadores para sistemas de arquivos locais (*Local File System Storage*).

---

## 2. Decisão de Arquitetura

Fica estabelecida a **Política de Retenção de Dados e Privacidade de Observabilidade**, com as seguintes diretrizes e adaptações no modelo de arquitetura hexagonal:

---

### 2.1. Política de Retenção e TTL de Arquivos Binários

1. **Arquivo Original (Source Object / Staging):**
   - **Remoção Pós-Conversão:** O arquivo binário enviado pelo cliente **deve** ser removido do storage imediatamente após o término do job de conversão (status `done`), liberando o espaço de armazenamento temporário.
   - **TTL Máximo de Expiração (Jobs Falhos ou Abandonados):** Caso o job transicione para `failed` ou permaneça sem processamento por interrupção catastrófica, o arquivo original terá um **TTL limite fixado em 24 horas (`24h`)**. Após este período, ele será expurgado permanentemente.

2. **Arquivo Convertido (Target Object / Download):**
   - **TTL Máximo de Retenção:** Arquivos convertidos gerados com sucesso permanecerão disponíveis para download pelo cliente por uma **janela máxima de 24 horas (`24h`)** contadas a partir do término do processamento (`created_at`/`updated_at`).
   - **Expurgo Automático:** Transcorrida a janela de 24 horas, o arquivo convertido é automaticamente deletado do storage. Tentativas de download após 24 horas retornarão HTTP `404 Not Found` com código `FILE_EXPIRED`.

3. **Jobs e Metadados de Auditoria:**
   - O registro do job no banco de dados (`JobRepository`) contendo apenas o seu ID, status, formatos e erros (sem o binário) pode ser mantido por até 30 dias para fins de auditoria técnica e contabilização de uso/quota.

---

### 2.2. Regra Estrita de Privacidade em Logs, Métricas e Tracing

1. **Proibição Absoluta de Conteúdo e Payload:**
   - Nenhum log, métrica ou span de rastreamento (tracing) poderá, sob qualquer hipótese, registrar o conteúdo binário, texto extraído, payload bruto ou amostragem de dados de arquivos do usuário.
2. **Proibição de PII e Nomes de Arquivo Originais:**
   - Nomes de arquivo originais fornecidos pelos clientes no upload (ex.: `contrato_joao_silva_2026.docx`) **não podem ser gravados nos logs ou tags de observabilidade**, pois frequentemente contêm PII.
   - A aplicação utilizará estritamente a chave interna gerada `object_key` (UUID v4) para referência nos sistemas de storage e logs.
3. **Metadados Permitidos:**
   - A observabilidade restringe-se estritamente aos metadados operacionais e de negócio sanitizados:
     - `job_id` (UUID v4)
     - `source_format` e `target_format` (ex.: `wav`, `mp3`, `pdf`, `jpg`)
     - `file_size_bytes` (tamanho numérico do arquivo)
     - `duration_ms` (tempo de execução do conversor)
     - `status` (`pending`, `processing`, `done`, `failed`)
     - `error_code` (código de erro padronizado, ex.: `CORRUPTED_FILE`)

---

### 2.3. Impacto no Port Hexagonal `ObjectStorage`

Para suportar o retencionamento dinâmico e o expurgo automático de objetos expirados sem violar a abstração hexagonal:

1. **Assinatura Atualizada do Port `ObjectStorage`:**
   ```go
   type ObjectStorage interface {
       // Upload armazena um arquivo e retorna a chave única do objeto armazenado.
       Upload(ctx context.Context, key string, content io.Reader, size int64, contentType string) error

       // UploadWithTTL armazena um arquivo atribuindo-lhe um tempo de retenção/expiração pré-definido.
       UploadWithTTL(ctx context.Context, key string, content io.Reader, size int64, contentType string, ttl time.Duration) error

       // Download recupera o stream de leitura de um arquivo armazenado.
       Download(ctx context.Context, key string) (io.ReadCloser, error)

       // Delete remove um objeto do storage explicitamente.
       Delete(ctx context.Context, key string) error

       // DeleteExpired varre e remove objetos cuja retenção excedeu a janela estipulada.
       // Em adaptadores S3/MinIO com Object Lifecycle Rules ativas, este método pode ser NoOp ou invocar a API de purga.
       // Em adaptadores Local FileSystem Storage, executa a varredura e remoção dos arquivos no disco.
       DeleteExpired(ctx context.Context, retentionWindow time.Duration) (int64, error)
   }
   ```

2. **Estratégia dos Adaptadores (`Adapters`):**
   - **Adaptador S3 / MinIO:** A expiração de 24h é delegada prioritariamente para as *Bucket Lifecycle Configuration Rules* nativas da nuvem/MinIO (`Expiration: Days = 1`), reduzindo a sobrecarga da aplicação.
   - **Adaptador Local Filesystem (Desenvolvimento / Standalone):** O método `DeleteExpired` varre o diretório de destino local limpando arquivos cuja data de modificação (`mtime`) seja superior a 24 horas, acionado por uma rotina agendada (Cron/Ticker) na camada de infraestrutura.

---

## 3. Matriz de Retenção por Recurso

| Recurso | Local / Componente | Janela de Retenção (TTL) | Mecanismo de Expurgo |
| :--- | :--- | :--- | :--- |
| **Arquivo Original enviado** | `ObjectStorage` (`source/`) | Imediato pós-sucesso ou max 24h | `Delete()` explícito no worker ou S3 Lifecycle |
| **Arquivo Convertido gerado** | `ObjectStorage` (`target/`) | **24 horas** | S3 Lifecycle Rule (24h) ou `DeleteExpired()` |
| **Registro do Job (DB)** | `JobRepository` | 30 dias | Tabela de log de auditoria / DB Cleanup job |
| **Logs Estruturados** | `stdout` / Aggregator | Conforme política do log aggregator | Log rotation / OpenSearch lifecycle policy |
| **Métricas** | Prometheus Server | Conforme retenção do Prometheus (ex.: 15 dias) | Prometheus retention configuration |

---

## 4. Consequências

* **Positivas:**
  - **Conformidade Legal & LGPD/GDPR:** Garantia de privacidade minimizando retenção de dados e impedindo vazamentos de PII em logs de telemetria.
  - **Gestão Eficiente de Recursos:** Eliminação automática de arquivos temporários evita estouro de capacidade de disco e custos desnecessários em Object Storage S3.
  - **Flexibilidade no Port Hexagonal:** Permite que a aplicação utilize tanto facilidades de Lifecycle nativas em nuvem quanto mecanismos de limpeza em ambiente de desenvolvimento local via `DeleteExpired`.

* **Negativas / Mitigações:**
  - **Indisponibilidade de Downloads Tardios:** Arquivos convertidos ficam indisponíveis após 24h.
  - *Mitigação:* A resposta das rotas da API e o schema do Job devem conter clareza sobre a janela de disponibilidade (`expires_at`), alertando o cliente para realizar o download imediatamente após a conversão.
