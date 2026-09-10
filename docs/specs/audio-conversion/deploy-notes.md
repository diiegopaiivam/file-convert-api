# Guia de Operações e Implantação Local (Deploy Notes) — FileConvert API

* **Data:** 2026-09-09
* **Papel:** Engenheiro DevOps
* **Ambiente Target:** Local (Docker / Docker Compose)

---

## 1. Avaliação de Conformidade de Infraestrutura com a Arquitetura

### 1.1. Verificação do Princípio de Isolamento
- **Worker Container (`Dockerfile.worker`)**: 
  - Construído a partir da imagem `debian:bookworm-slim`.
  - Contém as dependências nativas pré-instaladas (`ffmpeg`, `libreoffice-writer`, `imagemagick`, `poppler-utils`).
  - **Conclusão DevOps:** Atende 100% às diretrizes estabelecidas nos ADRs 0001, 0002 e 0003 do Arquiteto.

- **API Container (`Dockerfile.api`)**: 
  - Construído a partir da imagem minimalista `alpine:3.20`.
  - **Não** possui nenhuma biblioteca nativa de conversão instalada no seu file-system.
  - **Conclusão DevOps:** Garante o isolamento completo e footprint reduzido para a camada HTTP síncrona da API REST.

---

## 2. Configuração de Healthchecks no `docker-compose.yml`

Os serviços no `docker-compose.yml` contam com sondas de saúde (*healthchecks*) ativas para garantir ordem de inicialização determinística:

| Serviço | Mecanismo de Healthcheck | Frequência | Critério de Prontidão |
| :--- | :--- | :--- | :--- |
| **postgres** | `pg_isready -U fileconvert -d fileconvert` | 5s | Banco pronto para aceitar conexões SQL |
| **redis** | `redis-cli ping` | 5s | Fila / Cache respondendo com `PONG` |
| **api** | `wget http://localhost:8080/health` | 10s | Servidor HTTP pronto para receber tráfego |
| **worker** | `ffmpeg -version` | 10s | Binários nativos acessíveis e operacionais |

---

## 3. Passo a Passo para Subida e Validação Local

### Passo 1: Compilação das Imagens (Build)
Execute o comando a seguir na raiz do projeto para compilar os binários Go e montar as imagens Docker locais:

```bash
docker compose build
```

### Passo 2: Inicialização dos Containers (Up)
Suba os serviços em segundo plano (`detached mode`) aguardando a consolidação das sondas de saúde:

```bash
docker compose up -d
```

Verifique o status dos serviços e confirme se todos atingiram o status `(healthy)`:

```bash
docker compose ps
```

---

### Passo 3: Teste Manual de Conversão de Áudio (Upload → Status → Download)

1. **Submeter arquivo de áudio WAV para conversão:**
```bash
curl -X POST http://localhost:8080/v1/conversions/audio \
  -F "file=@sample.wav" \
  -F "target_format=mp3"
```
*Resposta esperada:* `202 Accepted` contendo o `"job_id": "job_xyz123"`.

2. **Consultar o status do Job (Polling):**
```bash
curl -s http://localhost:8080/v1/jobs/job_xyz123
```
*Acompanhe a transição do campo `"status"` de `"pending"`/`"processing"` para `"done"`.*

3. **Efetuar o download do arquivo MP3 convertido:**
```bash
curl -o resultado.mp3 http://localhost:8080/v1/jobs/job_xyz123/download
```

4. **Validar a integridade do arquivo gerado:**
```bash
file resultado.mp3
```
*Saída esperada:* `resultado.mp3: Audio file with ID3 version 2.4.0` ou `MPEG ADTS, layer III`.

---

### Passo 4: Inspeção de Logs dos Workers
Para monitorar a execução dos subprocessos `ffmpeg` em tempo real nos workers:

```bash
docker compose logs -f worker
```

---

### Passo 5: Encerramento do Ambiente (Down)
Para parar os serviços e remover os recursos e volumes de armazenamento local:

```bash
docker compose down -v
```
