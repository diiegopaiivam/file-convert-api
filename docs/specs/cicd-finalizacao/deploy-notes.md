# Notas de Implantação e Especificação DevOps: Esteira Completa de CI/CD

* **Projeto:** FileConvert API
* **Data:** 2026-10-14
* **Responsável:** DevOps Lead
* **Status:** Aprovado & Em Produção

---

## 1. Visão Geral da Arquitetura de Integração e Entrega Contínua

A esteira de CI/CD da **FileConvert API** foi projetada para garantir **alta velocidade de feedback**, **auditoria estrita de segurança e vulnerabilidades em contêineres**, **geração determinística de imagens multi-arquitetura (`amd64` e `arm64`)** e **deploy automatizado seguro via SSH na Oracle Cloud Infrastructure (OCI)**.

### Matriz de Workflows e Ordem de Dependência (Bloqueio)

| Ordem | Workflow | Arquivo | Gatilhos | Objetivo Principal | Bloqueia Downstream? |
| :---: | :--- | :--- | :--- | :--- | :--- |
| **1** | **CI / Fast Feedback** | [ci.yml](file:///.github/workflows/ci.yml) | `push`, `pull_request` | `go vet`, `golangci-lint`, `go test -race`, compilação binária de sanidade. | **SIM:** Falha rápida antes de qualquer job pesado de contêiner. |
| **2** | **Security Audit & Trivy** | [security.yml](file:///.github/workflows/security.yml) | `push`, `pull_request` | `govulncheck` (Go call-graph) e `Trivy` (scans das imagens `api` e `worker`). | **SIM:** Falha em CVEs de severidade `CRITICAL` ou `HIGH`. |
| **3** | **Build & Push GHCR** | [build-and-push.yml](file:///.github/workflows/build-and-push.yml) | Conclusão com sucesso de **CI** e **Security** na branch `main` | Build multi-arch (`linux/amd64`, `linux/arm64`) e push para o GitHub Container Registry. | **SIM:** Só roda se CI e Security estiverem 100% verdes no commit. |
| **4** | **CD / Deploy OCI** | [deploy.yml](file:///.github/workflows/deploy.yml) | Conclusão com sucesso de **Build & Push** na branch `main` | Conexão SSH segura na VM OCI para `docker compose pull && docker compose up -d`. | **FINAL:** Não executa se o build falhar ou se originado de forks. |

---

## 2. Diagrama de Fluxo e Dependência entre Pipelines

```mermaid
flowchart TD
    subgraph Trigger ["Gatilho do Desenvolvedor"]
        PULL_REQ["Pull Request (PR) para main"]
        PUSH_MAIN["Merge / Push direto na branch main"]
    end

    subgraph CI ["Estágio 1: Validação de Qualidade de Código (ci.yml)"]
        direction TB
        VET["go vet ./..."]
        LINT["golangci-lint (v1.57.2)"]
        TEST["go test -v -race ./..."]
        SANITY["Sanity Build (cmd/api & cmd/worker)"]
        VET --> LINT --> TEST --> SANITY
    end

    subgraph SEC ["Estágio 2: Auditoria e Varredura de Contêineres (security.yml)"]
        direction TB
        GOVULN["govulncheck ./... (AST/Call-Graph)"]
        BUILD_SCAN["Compilação de Imagens para Varredura Local"]
        TRIVY_API["Trivy Scan: API Image (CRITICAL/HIGH)"]
        TRIVY_WORKER["Trivy Scan: Worker Image (CRITICAL/HIGH)"]
        BUILD_SCAN --> TRIVY_API
        BUILD_SCAN --> TRIVY_WORKER
    end

    subgraph GATE1 ["Gate 1: Verificação de Pré-Requisitos na Branch main"]
        COND1{"Commit na main?<br/>CI passou?<br/>Security passou?"}
    end

    subgraph BUILD ["Estágio 3: Compilação Multi-Arquitetura e Publicação (build-and-push.yml)"]
        direction TB
        BUILDX["Docker Buildx (linux/amd64, linux/arm64)"]
        TAGS["Tags: :latest, :<sha_curto>, :<sha_completo>"]
        GHCR["Push seguro para ghcr.io (GITHUB_TOKEN)"]
        BUILDX --> TAGS --> GHCR
    end

    subgraph GATE2 ["Gate 2: Validação de Segurança de Deploy"]
        COND2{"Build & Push concluiu com sucesso?<br/>Origem: Repositório Oficial (Não Fork)?"}
    end

    subgraph CD ["Estágio 4: Implantação Contínua na Nuvem (deploy.yml)"]
        direction TB
        MASK["Mascaramento de Segredos (::add-mask::)"]
        SSH_CONN["Conexão SSH Segura via appleboy/ssh-action"]
        COMPOSE_PULL["docker compose pull (Novas Imagens GHCR)"]
        COMPOSE_UP["docker compose up -d --remove-orphans"]
        PRUNE["docker image prune -af (Gestão de Armazenamento)"]
        MASK --> SSH_CONN --> COMPOSE_PULL --> COMPOSE_UP --> PRUNE
    end

    PULL_REQ --> CI
    PULL_REQ --> SEC
    PUSH_MAIN --> CI
    PUSH_MAIN --> SEC

    CI --> GATE1
    SEC --> GATE1

    GATE1 -- Sim (Ambos Verdes) --> BUILD
    GATE1 -- Não (Falha em Teste ou CVE) --> BLOCK_BUILD["⛔ Build Cancelado / Bloqueado"]

    BUILD --> GATE2

    GATE2 -- Sim (Sucesso na main) --> CD
    GATE2 -- Não (Falha de Imagem ou Fork) --> BLOCK_DEPLOY["⛔ Deploy Cancelado / Bloqueado"]
```

---

## 3. Especificação Detalhada dos Pipelines

### 3.1. `ci.yml` — Fail Fast & Qualidade de Código
* **Propósito:** Oferecer feedback ultrarrápido (execução em segundos) antes de qualquer etapa cara de compilação de imagens Docker.
* **Componentes:**
  1. `go vet ./...`: Detecta anomalias de sintaxe e concorrência suspeitas.
  2. `golangci-lint`: Executa o linter estático oficial com a versão `v1.57.2`.
  3. `go test -v -race ./...`: Executa a suíte completa de testes de unidade e integração ativando a flag `-race` para verificação de concorrência e condições de corrida de memória.
  4. `go build -v -o /dev/null ./cmd/api` e `./cmd/worker`: Garante que ambos os pontos de entrada compilam sem quebras.

### 3.2. `security.yml` — Auditoria Estática e Varredura de Imagens
* **Propósito:** Blindagem da cadeia de suprimentos de software (*Supply Chain Security*).
* **Componentes:**
  1. `govulncheck`: Varre vulnerabilidades conhecidas no código Go e dependências transitivas.
  2. `Trivy Scanner (API e Worker)`:
     * Analisa tanto pacotes de sistema operacional (Debian/Alpine) quanto bibliotecas compiladas.
     * **Critério de Falha:** Bloqueia o pipeline (`exit-code: 1`) para vulnerabilidades de severidade **`CRITICAL`** ou **`HIGH`** com correção disponível (`ignore-unfixed: true`).

### 3.3. `build-and-push.yml` — Build Multi-Arch e Publicação no GHCR
* **Propósito:** Empacotar e distribuir as imagens de produção.
* **Plataformas:** `linux/amd64` e `linux/arm64` (compatível com arquiteturas Intel/AMD e ARM Ampere da Oracle Cloud Free Tier).
* **Controle de Pré-Requisitos:** Utiliza a API do GitHub via CLI `gh run list --commit` para confirmar que **CI** e **Security** foram concluídos com `success` no commit exato antes de iniciar o build.
* **Tags Geradas no GHCR:**
  * `ghcr.io/<owner>/file-convert-api-api:latest`
  * `ghcr.io/<owner>/file-convert-api-api:<short_sha>`
  * `ghcr.io/<owner>/file-convert-api-worker:latest`
  * `ghcr.io/<owner>/file-convert-api-worker:<short_sha>`

### 3.4. `deploy.yml` — Continuous Deployment via SSH Hardened
* **Propósito:** Atualizar a stack de produção na VM da Oracle Cloud sem intervenção manual.
* **Mecanismos de Segurança:**
  * **Isolamento de Forks:** Validação condicional `github.event.workflow_run.head_repository.full_name == github.repository` impede que PRs ou forks acionem o deploy ou tenham acesso a credenciais.
  * **Restrição de Branch:** Execução restrita à branch `main`.
  * **Mascaramento:** Aplicação da diretiva `::add-mask::` nos segredos de infraestrutura no início do job.
  * **Zero-Downtime:** Execução de `docker compose up -d --remove-orphans` assegura que os contêineres só sejam reciclados após o download bem-sucedido das novas imagens.

---

## 4. Gestão de Segredos do Repositório (GitHub Actions Secrets)

Para que o pipeline de deploy opere, configure os seguintes segredos em **Settings > Secrets and variables > Actions**:

| Nome do Segredo | Descrição | Exemplo de Valor |
| :--- | :--- | :--- |
| `OCI_VM_HOST` | Endereço IP público ou FQDN da instância OCI | `150.136.200.45` ou `api.fileconvert.example.com` |
| `OCI_VM_USER` | Usuário de administração SSH da VM | `ubuntu` |
| `OCI_VM_SSH_KEY` | Chave privada OpenSSH autorizada no host OCI | `-----BEGIN OPENSSH PRIVATE KEY-----...` |
| `GITHUB_TOKEN` | Token nativo do GitHub Actions | *Gerado automaticamente com permissão `packages: write`* |

> [!CAUTION]
> **Políticas de Acesso SSH:**
> 1. Conforme estabelecido no [cloud-init.sh](file:///Users/digopaiva/Documents/projects/file-convert-api/deploy/oracle-cloud/cloud-init.sh#L37-L39), o acesso SSH na porta 22 é estritamente restrito via firewall e Security List da Oracle Cloud ao IP do operador ou VPN bastion.
> 2. A chave privada `OCI_VM_SSH_KEY` **nunca** deve conter passphrase se utilizada em automações não interativas do CI/CD.

---

## 5. Manutenção Automatizada de Dependências (`dependabot.yml`)

O arquivo [.github/dependabot.yml](file:///.github/dependabot.yml) estabelece verificações semanais (segundas-feiras às 00:00 UTC) em três frentes:

1. **`gomod` (`/`):** Dependências do Go (`go.mod` e `go.sum`).
2. **`docker` (`/deploy/docker`):** Imagens base de contêineres (`golang:1.22-alpine`, `alpine:3.20`, `debian:bookworm-slim`).
3. **`github-actions` (`/`):** Atualizações de versões e patches de segurança das Actions utilizadas nos workflows.

---

## 6. Procedimento Operacional Padrão (SOP): Rollback em Produção

Se uma nova versão introduzir regressão em produção, execute uma das seguintes opções de rollback:

### Opção 1: Rollback via GitHub Actions (Re-dispatch)
1. Acesse a aba **Actions** no repositório GitHub.
2. Selecione o workflow **CD / Deploy to Oracle Cloud VM**.
3. Clique em **Run workflow** selecionando o commit estável anterior na branch `main`.

### Opção 2: Rollback Imediato via Terminal na VM
Conecte-se à VM e aponte para a tag SHA anterior no compose ou ambiente:
```bash
ssh -i ~/.ssh/id_rsa_oci ubuntu@<OCI_VM_HOST>
cd /opt/file-convert-api/deploy
# Ajustar temporariamente para a tag SHA estável
docker compose down
# Iniciar a versão com tag específica
docker compose up -d
```
