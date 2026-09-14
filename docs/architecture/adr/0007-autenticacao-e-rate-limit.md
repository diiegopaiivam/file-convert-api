# ADR 0007: Modelo de Autenticação, Rate Limiting e Gestão de Quotas por Chave e IP

* **Status:** Aceito
* **Data:** 2026-09-10
* **Autor:** Arquiteto de Software

---

## 1. Contexto

A **FileConvert API** é uma plataforma multi-tenant de conversão assíncrona de arquivos de alto desempenho. O processamento de conversão (especialmente envolvendo binários nativos como `ffmpeg` e `LibreOffice`) consome volumes significativos de CPU, memória RAM e I/O de disco.

Sem mecanismos estritos de autenticação, controle de taxa de requisições (rate limiting) e gerenciamento de quotas de execução simultânea:
1. Um único cliente malicioso ou com bug de repetição (retry-storm) poderia saturar a infraestrutura de workers e esgotar a memória/CPU dos containers, resultando em Negação de Serviço (DoS/DDoS) para todos os outros usuários.
2. Clientes sem autenticação poderiam abusar dos endpoints de borda para consumo indevido de recursos.
3. Não haveria previsibilidade para alocação justa de capacidade (*fair use*) entre diferentes planos de uso (ex.: *Free*, *Pro*, *Enterprise*).

---

## 2. Decisão de Arquitetura

Fica estabelecida a arquitetura de **Autenticação por API Key**, **Rate Limiting em Dois Níveis (Chave e IP)** e **Quota Distribuída de Jobs Simultâneos por Chave**, abstraída através dos Ports Hexagonais `AuthProvider` e `RateLimiter`.

---

### 2.1. Modelo de Autenticação (`AuthProvider` Port)

- **Identificador de Acesso (API Key):** Toda requisição aos endpoints protegidos da API deve conter uma chave de API válida.
- **Formato da Chave:** Chaves de API utilizam o prefixo de ambiente padronizado:
  - `fc_live_<hash_criptografico_32_bytes>` para ambiente de produção.
  - `fc_test_<hash_criptografico_32_bytes>` para ambiente de testes/sandbox.
- **Forma de Envio HTTP (Headers):** A chave de API pode ser transmitida via:
  1. `Authorization: Bearer <api_key>` *(Formato primário recomendado)*
  2. `X-API-Key: <api_key>` *(Formato alternativo)*
- **Port de Autenticação:** A resolução e validação da chave são abstraídas pela interface `AuthProvider`:
  ```go
  type AuthProvider interface {
      Authenticate(ctx context.Context, apiKey string) (*ClientIdentity, error)
  }
  ```
- **Falha de Autenticação:** Requisições sem chave ou com chaves inválidas/expiradas/revogadas serão rejeitadas síncronamente na borda HTTP com `401 Unauthorized` e o código de erro `UNAUTHORIZED`.

---

### 2.2. Controle de Vazão em Dois Níveis (`RateLimiter` Port)

O controle de taxa de requisições opera em duas camadas independentes executadas pelo middleware da borda HTTP:

1. **Rate Limit por Endereço IP (Proteção de Borda / Unauthenticated Rate Limit):**
   - Aplicado a todas as requisições recebidas (identificadas via IP do cliente ou cabeçalho `X-Forwarded-For` validado pelo proxy/Nginx).
   - Protege o servidor contra ataques de força bruta, scraping e DoS antes do processamento de autenticação.
2. **Rate Limit por Chave de API (Authenticated Rate Limit):**
   - Aplicado após a autenticação bem-sucedida da chave de API.
   - Restringe o número de requisições por segundo (RPS) e por minuto (RPM) de acordo com o plano (*tier*) do cliente.

**Algoritmo e Persistência:**
- O `RateLimiter` utiliza o algoritmo **Token Bucket** (ou *Sliding Window Log*) com estado persistido e sincronizado via **Redis** central, garantindo controle atômico e consistente entre múltiplas instâncias da API.

---

### 2.3. Quota de Jobs Simultâneos por Chave (Concurrent Job Quota)

Limitar apenas a taxa de requisições por segundo (RPS) não impede que um cliente envie, em uma pequena janela de tempo, dezenas de requisições legítimas de abertura de job contendo arquivos pesados (ex.: 50 MB cada), saturando a fila e os workers por vários minutos.

Para garantir fair-use e isolamento de tenants:
- **Semáforo Distribuído de Jobs Ativos:** A API mantém no Redis a contagem em tempo real de jobs nos status `pending` e `processing` associados a cada `api_key`.
- **Validação na Criação do Job (`POST /v1/jobs`):**
  1. O middleware invoca `RateLimiter.AcquireConcurrentJob(ctx, apiKey, maxConcurrent)`.
  2. Se a contagem atual de jobs ativos da chave for menor que a quota do seu plano, a vaga é reservada e o job é criado.
  3. Se a quota for atingida, a requisição é rejeitada síncronamente com HTTP `429 Too Many Requests` e código `CONCURRENT_JOBS_EXCEEDED`.
- **Liberação da Quota:** Quando o worker conclui o processamento de um job (transicionando para `done` ou `failed`), a infraestrutura executa `RateLimiter.ReleaseConcurrentJob(ctx, apiKey)`, decrementando o semáforo.

---

### 2.4. Matriz de Limites Numéricos por Plano (Tiers)

Ficam estipulados os seguintes limites padrão para a v1 da aplicação:

| Plano / Categoria | Rate Limit por IP (Borda) | Rate Limit por Chave (RPS / RPM) | Quota de Jobs Simultâneos por Chave | Limite Máx. de Arquivo |
| :--- | :--- | :--- | :--- | :--- |
| **Não Autenticado (IP Default)** | **5 req/s** (burst: 10) | N/A | N/A (Bloqueado) | 50 MB |
| **Plano Free** | 5 req/s | **10 req/s** (100 req/min) | **2 jobs simultâneos** | 50 MB |
| **Plano Pro** | 10 req/s | **50 req/s** (1.000 req/min) | **10 jobs simultâneos** | 50 MB |
| **Plano Enterprise** | 20 req/s | **200 req/s** (5.000 req/min) | **50 jobs simultâneos** | 50 MB |

---

### 2.5. Resposta HTTP Padronizada em Estouro de Limite (`429 Too Many Requests`)

Quando uma requisição for rejeitada por estouro de Rate Limit ou por Quota de Jobs Simultâneos:

1. **Status Code HTTP:** `429 Too Many Requests`
2. **Cabeçalho Obrigatório `Retry-After`:**
   - O cabeçalho HTTP `Retry-After: <segundos>` **deve** ser incluído na resposta, indicando ao cliente quantos segundos ele deve aguardar antes de tentar novamente. Exemplo: `Retry-After: 5`.
3. **Cabeçalhos Padrão de Telemetria de Rate Limit:**
   - `X-RateLimit-Limit: <limite_maximo>`
   - `X-RateLimit-Remaining: <requisicoes_restantes>`
   - `X-RateLimit-Reset: <timestamp_epoch_segundos>`

#### Schema do JSON de Erro para Rate Limit de Requisições:
```json
{
  "code": "RATE_LIMIT_EXCEEDED",
  "message": "Limite de taxa de requisições excedido para a chave de API.",
  "details": [
    {
      "issue": "Sua chave de API excedeu a taxa limite permitida de 10 req/s. Aguarde o tempo indicado em Retry-After."
    }
  ],
  "timestamp": "2026-09-10T15:51:14Z"
}
```

#### Schema do JSON de Erro para Excesso de Quota de Jobs Simultâneos:
```json
{
  "code": "CONCURRENT_JOBS_EXCEEDED",
  "message": "Quota de jobs simultâneos em processamento atingida.",
  "details": [
    {
      "issue": "Sua chave de API possui 2 jobs em processamento (limite máximo do plano Free). Aguarde a conclusão de um job para enviar novas requisições."
    }
  ],
  "timestamp": "2026-09-10T15:51:14Z"
}
```

---

## 3. Matriz Resumida de Códigos e Comportamentos

| Cenário de Erro | Status HTTP | Code String | Cabeçalho `Retry-After` | Descrição / Ação |
| :--- | :--- | :--- | :--- | :--- |
| **API Key Ausente / Inválida** | `401 Unauthorized` | `UNAUTHORIZED` | Ausente | Requisição negada por ausência ou falha de credencial. |
| **Estouro de Rate Limit (IP)** | `429 Too Many Requests` | `RATE_LIMIT_EXCEEDED` | Presente (ex: `Retry-After: 1`) | IP enviou muitas requisições por segundo. |
| **Estouro de Rate Limit (Key)** | `429 Too Many Requests` | `RATE_LIMIT_EXCEEDED` | Presente (ex: `Retry-After: 5`) | Chave de API excedeu RPS/RPM do seu plano. |
| **Quota de Jobs Concorrentes** | `429 Too Many Requests` | `CONCURRENT_JOBS_EXCEEDED` | Presente (ex: `Retry-After: 10`) | Chave atingiu limite de jobs simultâneos ativos. |

---

## 4. Consequências

* **Positivas:**
  - **Segurança e Estabilidade:** Proteção eficaz contra cenários de exaustão de recursos, DoS e tempestades de retentativa.
  - **Isolamento Multi-Tenant:** Garantia de que clientes do plano Free não comprometem a capacidade computacional disponível para clientes Pro e Enterprise.
  - **Conformidade com Padrões Web:** Utilização dos cabeçalhos RFC/IETF (`Retry-After` e `X-RateLimit-*`) facilita o desenvolvimento de SDKs e a integração transparente de clientes HTTP.
  - **Arquitetura Desacoplada:** O uso de Ports (`AuthProvider` e `RateLimiter`) permite substituir os motores subjacentes (ex.: trocar validação em Postgres por JWT/OAuth2 ou Redis por Memcached) sem alterar o código de domínio ou handlers HTTP.

* **Negativas / Mitigações:**
  - **Dependência de Armazenamento de Estado (Redis):** O controle distribuído de rate limit e semáforo depende da alta disponibilidade do Redis.
  - *Mitigação:* Em caso de falha completa do cluster do Redis, o adaptador do `RateLimiter` deverá operar em modo *fail-open* temporário com log/alerta de severidade `CRITICAL`, evitando derrubar requisições legítimas dos clientes enquanto a infraestrutura recupera o serviço de cache/estado.
