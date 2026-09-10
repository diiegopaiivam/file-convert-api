# Guia de Implantação e Infraestrutura (Deploy Notes) — Observabilidade

* **Data:** 2026-09-10
* **Papel:** Engenheiro DevOps
* **Escopo:** Docker Compose, Prometheus, Grafana, Alertas e Gestão de Capacidade em Nuvem (Oracle Cloud Free Tier)

---

## 1. Visão Geral da Arquitetura de Infraestrutura

A stack de observabilidade da **FileConvert API** é provisionada como um ecossistema containerizado via Docker Compose, composto pelos seguintes serviços integrados:

```
                                    +-----------------------+
                                    |    Nginx / Client     |
                                    +-----------+-----------+
                                                |
                                                v
+-----------------------+           +-----------+-----------+
|    Worker Container   |           |   API HTTP Container  | <--- Scrape /metrics (15s)
+-----------+-----------+           +-----------+-----------+           |
            |                                   |                         |
            +-----------------+-----------------+                         |
                              |                                           |
                              v                                           v
                    +---------+---------+                       +---------+---------+
                    |  Redis / Postgres |                       | Prometheus Container|
                    +-------------------+                       +---------+---------+
                                                                          |
                                                                          v
                                                                +---------+---------+
                                                                | Grafana Container |
                                                                |   (Porta 3000)    |
                                                                +-------------------+
```

---

## 2. Componentes e Credenciais de Acesso

| Serviço | Porta Exposta | URL Interna / Acesso | Credenciais Padrão / Observação |
| :--- | :--- | :--- | :--- |
| **API HTTP** | `8080` | `http://localhost:8080` | Endpoint telemétrico: `/metrics` |
| **Prometheus** | `9090` | `http://localhost:9090` | Scraping automático a cada 15 segundos |
| **Grafana** | `3000` | `http://localhost:3000` | Usuário: `admin` \| Senha: `fileconvert` |

---

## 3. Configuração de Coleta e Alertas do Prometheus

### 3.1. Scraping Target (`prometheus.yml`)
O Prometheus faz o *scrape* contínuo do endpoint `/metrics` publicado pela API HTTP:

```yaml
scrape_configs:
  - job_name: 'fileconvert-api'
    metrics_path: '/metrics'
    static_configs:
      - targets: ['api:8080']
```

### 3.2. Regras de Alerta Provisionadas (`alert.rules.yml`)
As regras de alerta cobrem as anomalias funcionais estipuladas no documento de requisitos:

1. **`HighFormatPairFailureRate` (Crítico):** Disparado se a taxa de erro para um par de formatos ultrapassar **5%** em um janela de 5 minutos.
2. **`QueueStagnantOrGrowing` (Crítico):** Disparado se a fila tiver mais de 50 jobs e zero conclusões nos últimos 3 minutos.
3. **`RecurringJobTimeouts` (Aviso):** Disparado caso ocorram $\ge 3$ falhas com `JOB_TIMEOUT` (excedendo 60s) em 10 minutos.
4. **`HighEdgeRejectionRate` (Aviso):** Disparado caso a taxa de rejeição HTTP na borda (erros 4xx/5xx) ultrapasse **20%** em 5 minutos.

---

## 4. Provisionamento Automático do Grafana

O container do Grafana inclui provisionamento automático (*Infrastructure as Code*) declarativo:

- **Datasource:** Configurado para conectar automaticamente ao Prometheus interno (`http://prometheus:9090`).
- **Dashboard Pré-carregado (`FileConvert API — Visão Geral`):**
  - *Painel 1:* Taxa de conversão por par de formato em tempo real (`req/s`) segmentado por status (`done` vs `failed`).
  - *Painel 2:* Latência de processamento ativo no **Percentil 95 (p95)** utilizando a função `histogram_quantile(0.95, ...)`.
  - *Painel 3:* Indicador visual (Gauge com alerta de cor) da profundidade da fila de jobs pendentes (`fileconvert_queue_depth`).
  - *Painel 4:* Gráfico de requisições de borda HTTP categorizadas por código de retorno e código de erro (`error_code`).

---

## 5. Gestão de Capacidade e Retenção de Disco (Oracle Cloud / VM Storage Limits)

> [!WARNING]
> **RESTRIÇÃO DE ARMAZENAMENTO EM NUVEM (FREE TIER):**
> VMs na nuvem (como instâncias Arm/x86 da Oracle Cloud Free Tier) possuem limites restritos de disco (geralmente entre 40GB e 200GB compartilhados com o SO). Sem políticas rigorosas de retenção, arquivos de log e métricas TSDB causam rápido esgotamento de disco (*Disk Full Outage*).

Para mitigar o risco de estouro de armazenamento em produção, foram aplicadas duas estratégias estritas de contenção de disco no `docker-compose.yml`:

### 5.1. Limite e Rotação de Logs do Docker (`json-file`)
Todos os serviços do Compose (`api`, `worker`, `prometheus`, `grafana`, `postgres`, `redis`, `nginx`) utilizam a política de rotação de log do Docker:

```yaml
logging:
  driver: "json-file"
  options:
    max-size: "10m"
    max-file: "3"
```
* **Impacto:** O acumulado total de logs em disco por container é estritamente limitado a **30 MB** (3 arquivos de 10 MB). Ao atingir o limite, o Docker descarta os logs mais antigos automaticamente.

### 5.2. Política de Retenção de Métricas do Prometheus (TSDB)
O container do Prometheus é inicializado com limites explícitos de tempo e tamanho da base de dados de séries temporais:

```bash
--storage.tsdb.retention.time=15d
--storage.tsdb.retention.size=10GB
```
* **Impacto:** O histórico de métricas retém no máximo **15 dias** de dados ou **10 GB** de espaço em disco, o que ocorrer primeiro.

---

## 6. Comandos Operacionais para Implantação e Manutenção

### Inicializar a Stack Completa:
```bash
docker compose -f deploy/docker-compose.yml up -d --build
```

### Verificar o Status dos Containers e Healthchecks:
```bash
docker compose -f deploy/docker-compose.yml ps
```

### Monitorar o Uso de Disco da VM:
```bash
df -h /var/lib/docker
du -sh deploy/observability/data
```

### Recarregar Configurações do Prometheus Sem Reiniciar:
```bash
curl -X POST http://localhost:9090/-/reload
```
