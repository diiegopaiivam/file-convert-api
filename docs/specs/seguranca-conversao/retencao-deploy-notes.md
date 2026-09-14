# Notas de Implantação DevOps: Agendamento de Limpeza de Storage e Rotação de Logs — FileConvert API

## 1. Visão Geral e Objetivos de Infraestrutura

Este documento especifica a estratégia de implantação e operação DevOps para garantir a execução da **Política de Retenção de Dados** definida no [ADR 0008: Política de Retenção de Dados, Expiração de Storage e Privacidade de Logs e Métricas](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/adr/0008-retencao-de-dados.md).

No ambiente de produção hospedado na **Oracle Cloud Infrastructure (OCI) Always Free Tier**, os recursos de disco e armazenamento são limitados (volume de boot padrão de 45-50 GB compartilhado entre sistema operacional, runtime Docker, banco de dados Postgres, cache Redis, Prometheus e armazenamento de arquivos).

Esta arquitetura DevOps atinge dois objetivos cruciais:
1. **Expurgo Automático de Storage (TTL 24h):** Remoção garantida de arquivos binários (originais e convertidos) que excederem o tempo limite de retenção de 24 horas (`1440 minutos`), evitando acúmulo no volume de dados.
2. **Prevenção de Esgotamento de Disco da VM (Log & TSDB Hardening):** Controle rigoroso do tamanho de logs de containers via Docker `json-file` driver com compressão e limitação da retenção de séries temporais no Prometheus.

---

## 2. Serviço Agendado de Limpeza de Storage (`storage-cleanup`)

### 2.1. Arquitetura do Container de Limpeza

A limpeza do volume de armazenamento compartilhado (`/data/storage`) é realizada por um container dedicado e isolado em [deploy/docker-compose.yml](file:///Users/digopaiva/Documents/projects/file-convert-api/deploy/docker-compose.yml):

```yaml
  storage-cleanup:
    image: alpine:3.19
    container_name: fileconvert-storage-cleanup
    volumes:
      - ../storage:/data/storage:rw
    command: >
      sh -c "
      echo 'Iniciando servico de limpeza de storage com rotina cron (a cada 1 hora)...';
      echo '0 * * * * find /data/storage -type f -mmin +1440 -delete' > /etc/crontabs/root &&
      crond -f -l 2
      "
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
        compress: "true"
    restart: unless-stopped
```

### 2.2. Regra de Agendamento (Cron Schedule)

- **Frequência de Execução:** A cada hora cheia (`0 * * * *`).
- **Comando de Purga:** `find /data/storage -type f -mmin +1440 -delete`
  - `-type f`: Seleciona apenas arquivos regulares (ignora diretórios).
  - `-mmin +1440`: Identifica arquivos cuja última modificação ocorreu há mais de 1440 minutos (24 horas).
  - `-delete`: Remove os arquivos correspondentes atomicamente no sistema de arquivos.

---

## 3. Política de Rotação e Expiramento de Logs (Docker Daemon)

Sem limitação de tamanho, os arquivos de log do Docker (`json-file`) gravados em `/var/lib/docker/containers/*/*.log` no host podem expandir indefinidamente até lotar a partição principal do sistema operacional na VM.

### 3.1. Configuração por Serviço no Docker Compose

Todos os serviços declarados no [deploy/docker-compose.yml](file:///Users/digopaiva/Documents/projects/file-convert-api/deploy/docker-compose.yml) (`api`, `worker`, `prometheus`, `grafana`, `redis`, `postgres`, `nginx`, `storage-cleanup`) utilizam a seguinte diretiva padronizada:

```yaml
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
        compress: "true"
```

### 3.2. Dimensionamento e Teto de Armazenamento de Logs

- **`max-size: "10m"`:** Cada arquivo de log individual atinge no máximo 10 Megabytes antes de ser rotacionado.
- **`max-file: "3"`:** Mantém no máximo 3 arquivos de log por container (1 ativo + 2 arquivos históricos rotacionados).
- **`compress: "true"`:** Aplica compressão `gzip` nos arquivos de log inativos (reduzindo o espaço em disco de ~10 MB para ~1 MB por arquivo compactado).
- **Teto Máximo por Container:** ~12 MB de ocupação real de disco.
- **Teto Máximo da Aplicação (8 Serviços):** ~96 MB totais reservados para logs em todo o ecossistema Docker.

---

## 4. Retenção de Métricas de Telemetria (Prometheus TSDB)

O banco de dados de séries temporais (TSDB) do Prometheus é limitado para impedir que o armazenamento de métricas consuma a partição de dados da VM OCI.

### 4.1. Parâmetros de Linha de Comando no `docker-compose.yml`

```yaml
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--storage.tsdb.retention.time=7d'
      - '--storage.tsdb.retention.size=5GB'
```

- **`--storage.tsdb.retention.time=7d`:** Dados de métricas com mais de 7 dias são purgados automaticamente.
- **`--storage.tsdb.retention.size=5GB`:** Teto máximo absoluto de armazenamento em disco para o TSDB. Ao atingir 5 GB, os blocos mais antigos são removidos independentemente do tempo.

---

## 5. Operação, Monitoramento e Manutenção do Host OCI

### 5.1. Comandos de Inspeção de Ocupação de Disco

O operador DevOps pode verificar a saúde da partição e do Docker executando via SSH no host da VM:

```bash
# Verificar espaço livre em disco no SO
df -h /

# Verificar espaço ocupado por containers, volumes e imagens Docker
docker system df

# Verificar tamanho dos logs de todos os containers ativos
du -sh /var/lib/docker/containers/*/*.log
```

### 5.2. Execução Manual de Emergência (Limpeza Forçada)

Caso o disco atinja estado crítico (>90% de ocupação):

```bash
# 1. Purga imediata de arquivos no storage com mais de 24h
docker exec fileconvert-storage-cleanup find /data/storage -type f -mmin +1440 -delete

# 2. Remoção de imagens não utilizadas e cache de build do Docker
docker system prune -f

# 3. Forçar rotação/Truncar logs de containers (se necessário em emergência)
truncate -s 0 /var/lib/docker/containers/*/*.log
```

---

## 6. Matriz de Limites e Proteção no OCI Free Tier

| Componente | Recurso / Alvo | Mecanismo de Controle | Limite Máximo Ocupado |
| :--- | :--- | :--- | :--- |
| **Storage Binário** | `/data/storage` | Container `storage-cleanup` (Cron 1h) | Variável (máx 24h de arquivos ativos) |
| **Logs de Containers** | `/var/lib/docker/containers/` | Docker `json-file` + `compress: true` | ~96 MB totais |
| **Métricas Prometheus** | Volume `prometheus-data` | `--storage.tsdb.retention.size` | **5 GB** |
| **Banco de Dados PG** | Volume `postgres-data` | Tabela de jobs com expurgo de histórico | ~2 GB |
