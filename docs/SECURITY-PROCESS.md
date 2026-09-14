# Processo de Governança de Segurança e Resposta a CVEs — FileConvert API

## 1. Visão Geral de DevSecOps

A **FileConvert API** processa arquivos binários não confiáveis de terceiros utilizando ferramentas de conversão nativas (`ffmpeg`, `LibreOffice`, `ImageMagick`, `Poppler`). Por executarem em ambiente isolado de infraestrutura (conforme [ADR 0005: Isolamento de Subprocesso de Conversão](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/adr/0005-isolamento-subprocesso-conversao.md)), esses binários representam a principal superfície de ataque contra a aplicação.

Para mitigar riscos de execução remota de código (RCE), estouro de buffer e negação de serviço, esta política estabelece a automação de auditoria contínua e a proibição absoluta do uso da tag `latest` ou pacotes flutuantes sem versão explícita.

---

## 2. Automação de Segurança no Pipeline de CI/CD

O workflow [.github/workflows/security.yml](file:///Users/digopaiva/Documents/projects/file-convert-api/.github/workflows/security.yml) executa verificações automatizadas de segurança em cada Pull Request e em cada commit na branch principal (`main`/`master`).

```
 +------------------+     +------------------------+     +-------------------------+
 |   Pull Request   | --> |  Job 1: govulncheck    | --> | Valida chamadas Go      |
 |  ou Commit Main  |     +------------------------+     | no grafo de executavel  |
 +------------------+                                +-------------------------+
          |
          v
 +------------------------+     +-------------------------+
 |  Job 2: Trivy Scan     | --> | Falha CI em CVEs        |
 |  (API & Worker Imgs)   |     | CRITICAL/HIGH (fixed)   |
 +------------------------+     +-------------------------+
```

### 2.1. Auditoria de Código e Dependências Go (`govulncheck`)
- **Ferramenta:** `golang.org/x/vuln/cmd/govulncheck`
- **Escopo:** Analisa a árvore de dependências registradas no `go.mod` e `go.sum` cruzando com a base oficial de dados de vulnerabilidade da linguagem Go (*Go Vulnerability Database*).
- **Diferencial:** Diferente de scanners baseados apenas em versões, o `govulncheck` analisa o Grafo de Chamadas (*Call Graph*) para confirmar se a função vulnerável é de fato invocada pelo código da aplicação.

### 2.2. Varredura de Vulnerabilidades em Imagens de Contêiner (`Trivy`)
- **Ferramenta:** `aquasecurity/trivy-action`
- **Estágio:** Pré-deploy (executado imediatamente após a compilação das imagens `fileconvert-api` e `fileconvert-worker`).
- **Regra de Bloqueio:** O pipeline é interrompido com falha (`exit-code: 1`) se for identificada qualquer vulnerabilidade de severidade **`CRITICAL`** ou **`HIGH`** que já possua correção oficial disponibilizada pela distribuição (`ignore-unfixed: true`).

### 2.3. Atualizações Automatizadas com Dependabot
- **Configuração:** [.github/dependabot.yml](file:///Users/digopaiva/Documents/projects/file-convert-api/.github/dependabot.yml)
- **Frequência:** Semanal (todas as segundas-feiras).
- **Escopos Monitorados:**
  1. `gomod`: Módulos de código Go.
  2. `docker`: Imagens base nos arquivos Dockerfile (`deploy/docker/`).
  3. `github-actions`: Actions utilizadas nos workflows de CI.

---

## 3. Política de Fixação de Versões em Contêineres

### 3.1. Diretriz Inviolável de Travamento
- **Uso do `latest`:** Estritamente **PROIBIDO** em qualquer arquivo `Dockerfile` ou `docker-compose.yml`.
- **Binários no Worker (`deploy/docker/Dockerfile.worker`):** Todas as ferramentas de conversão nativas devem ter suas versões travadas via variáveis de ambiente da distribuição Debian Bookworm:

```dockerfile
ENV FFMPEG_VERSION="7:5.1.*" \
    LIBREOFFICE_VERSION="1:7.5.*" \
    IMAGEMAGICK_VERSION="8:6.9.*" \
    POPPLER_VERSION="22.12.*"
```

---

## 4. Procedimento Operacional Padrão (SOP) para Resposta a CVEs

Quando um novo CVE for publicado para uma das ferramentas binárias (`ffmpeg`, `LibreOffice`, `ImageMagick`, `poppler-utils`) ou quando o scanner **Trivy**/Dependabot alertar uma vulnerabilidade crítica/alta:

### Passo 1: Triagem e Avaliação de Impacto (T+2h)
1. Verificar a pontuação **CVSS v3** da vulnerabilidade.
2. Identificar se o CVE afeta os formatos de conversão suportados na v1 (`.wav`, `.mp3`, `.jpg`, `.png`, `.pdf`, `.doc`, `.docx`).
3. Checar se a distribuição Debian Security já disponibilizou o pacote atualizado de correção (`deb12uX`).

### Passo 2: Atualização do Dockerfile do Worker (T+6h)
1. Abrir uma branch de segurança no repositório: `git checkout -b fix/security-cve-YYYY-XXXX`.
2. Atualizar a variável da versão no [deploy/docker/Dockerfile.worker](file:///Users/digopaiva/Documents/projects/file-convert-api/deploy/docker/Dockerfile.worker) apontando para o patch seguro liberado pelo Debian.
   Exemplo:
   ```diff
   - ENV FFMPEG_VERSION="7:5.1.6-0+deb12u1"
   + ENV FFMPEG_VERSION="7:5.1.6-0+deb12u2"
   ```

### Passo 3: Validação Local e Regressão de Conversão (T+12h)
1. Compilar a imagem localmente:
   ```bash
   docker build -t fileconvert-worker:test -f deploy/docker/Dockerfile.worker .
   ```
2. Executar a suíte de testes unitários e de integração validando se os motores de conversão continuam operacionais:
   ```bash
   go test -v ./...
   ```
3. Testar manualmente a conversão dos 3 pilares binários (Áudio via `ffmpeg`, Documento via `LibreOffice`, Imagem via `ImageMagick/Poppler`).

### Passo 4: Submissão de PR e Deploy em Produção (T+24h)
1. Submeter o Pull Request para a branch `main`.
2. Garantir aprovação verde nos jobs `govulncheck` e `Trivy` no GitHub Actions.
3. Executar o deploy atualizando os containers na instância OCI.

---

## 5. Matriz de Resposta a Incidentes por Severidade

| Severidade CVE | Score CVSS | SLA de Resolução | Ação Obrigatória |
| :--- | :--- | :--- | :--- |
| **CRITICAL** | 9.0 - 10.0 | **24 horas** | Bloqueio de CI, atualização imediata de versão e deploy de emergência. |
| **HIGH** | 7.0 - 8.9 | **48 horas** | Atualização de versão no Dockerfile e aprovação de PR prioritária. |
| **MEDIUM** | 4.0 - 6.9 | 7 dias | Atualização tratada no ciclo semanal via Dependabot. |
| **LOW** | 0.1 - 3.9 | 30 dias | Acompanhamento e atualização no ciclo normal de manutenção. |
