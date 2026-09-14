# Guia de Implantação e Isolamento de Segurança (Deploy Notes)

* **Data:** 2026-09-10
* **Papel:** Engenheiro DevOps
* **Escopo:** Docker Compose, Hardening de Containers Worker, Isolamento de Subprocessos e Defesa em Profundidade (ADR 0005)

---

## 1. Visão Geral das Medidas de Hardening

Em conformidade com as diretrizes de segurança definidas no **ADR 0005 (Restrições de Isolamento e Segurança para Subprocessos de Conversão)**, a infraestrutura do serviço `worker` foi completamente blindada no `docker-compose.yml` para conter potenciais explorações de *Zero-Day* em conversores binários nativos (`ffmpeg`, `libreoffice`, `imagemagick`, `poppler-utils`).

A estratégia aplica os princípios de **Least Privilege** e **Zero Trust** no nível do container.

---

## 2. Configurações de Segurança Aplicadas no `docker-compose.yml`

As seguintes diretivas foram aplicadas ao serviço `worker`:

```yaml
worker:
  build:
    context: ..
    dockerfile: deploy/docker/Dockerfile.worker
  tmpfs:
    - /tmp:rw,noexec,nosuid,size=512m
  read_only: true
  cap_drop:
    - ALL
  security_opt:
    - no-new-privileges:true
```

### Detalhamento da Implementação:

1. **Remoção Total de Capabilities (`cap_drop: [ALL]`):**
   - Cancela todas as permissões administrativas do kernel Linux para o container, impedindo injeção de código em processos (`ptrace`), manipulação de interfaces de rede (`CAP_NET_RAW`) ou alteração de privilégios de sistema (`CAP_SYS_ADMIN`).

2. **Bloqueio de Escalada de Privilégios (`security_opt: ["no-new-privileges:true"]`):**
   - Impede que executáveis gerados dentro do container ganhem novos privilégios via sinalizações `setuid` ou `setgid`.

3. **Sistema de Arquivos Somente Leitura (`read_only: true`):**
   - O sistema de arquivos raiz (`/`) do container é imutável. Um atacante que obtenha execução remota de código não consegue gravar arquivos em `/usr/bin`, `/bin`, `/lib` ou modificar executáveis do sistema.

4. **Diretório de Trabalho em Memória Volátil (`tmpfs: /tmp:rw,noexec,nosuid,size=512m`):**
   - Os arquivos de trabalho do job de conversão são processados exclusivamente no diretório temporário `/tmp`, montado em memória RAM.
   - A flag `noexec` impede a execução direta de binários gravados em `/tmp`.
   - A flag `nosuid` desativa a interpretação de bits de permissão executável.
   - O limite de tamanho em memória (`size=512m`) evita ataques de Negação de Serviço por consumo excessivo de memória RAM.

---

## 3. Isolamento de Rede e Mitigação de SSRF

### 3.1. Comunicação Interna
O worker exige conectividade exclusiva com a rede interna do Docker Compose para:
- Consumir e responder jobs no **Redis** (`redis:6379`).
- Atualizar o estado do job no banco **PostgreSQL** (`postgres:5432`).
- Baixar e gravar arquivos no volume de armazenamento compartilhado (`/data/storage`).

### 3.2. Bloqueio de Rede Externa durante o Subprocesso (Mitigação de SSRF / ImageTragick)
Para impedir que parsers (como ImageMagick ou LibreOffice) realizem chamadas para a internet pública (exploração de SSRF, exfiltração de dados fora de banda ou conexões de *Command & Control*):

- **Regra de Egress no Host (Recomendação de Produção):** Em ambiente de produção na nuvem (ex.: Oracle Cloud), o tráfego de saída (*egress*) dos containers de worker para faixas de IP públicas de internet deve ser bloqueado na tabela `iptables` ou via *Security Lists / Network Security Groups* da nuvem:
  ```bash
  # Bloquear tráfego de saída do container worker para redes públicas externos (exceto rede interna Docker 172.18.0.0/16)
  iptables -A FORWARD -s 172.18.0.0/16 ! -d 172.18.0.0/16 -j DROP
  ```

---

## 4. Roteiro de Verificação Operacional do Hardening

Para validar que as restrições foram aplicadas corretamente na infraestrutura, execute os seguintes testes de verificação no host:

### Teste 1: Confirmar remoção de Capabilities do Linux
```bash
docker inspect file-convert-api-worker-1 --format '{{json .HostConfig.CapDrop}}'
```
* **Resultado Esperado:** `["ALL"]`

### Teste 2: Confirmar no-new-privileges
```bash
docker inspect file-convert-api-worker-1 --format '{{json .HostConfig.SecurityOpt}}'
```
* **Resultado Esperado:** `["no-new-privileges:true"]`

### Teste 3: Validar Filesystem Somente Leitura (Read-Only Root FS)
Tentar criar um arquivo fora de `/tmp`:
```bash
docker exec -it file-convert-api-worker-1 touch /usr/local/bin/test_file
```
* **Resultado Esperado:** `touch: cannot touch '/usr/local/bin/test_file': Read-only file system`

### Teste 4: Validar Escrita e Isolamento no `tmpfs` em `/tmp`
Tentar criar um arquivo em `/tmp`:
```bash
docker exec -it file-convert-api-worker-1 touch /tmp/test_file
```
* **Resultado Esperado:** Arquivo criado com sucesso no diretório de memória volátil `/tmp`.
