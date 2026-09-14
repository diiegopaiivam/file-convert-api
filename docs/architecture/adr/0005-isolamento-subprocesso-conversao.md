# ADR 0005: Restrições de Isolamento e Segurança para Subprocessos de Conversão

* **Status:** Aceito
* **Data:** 2026-09-10
* **Autor:** Arquiteto de Software

---

## 1. Contexto

A **FileConvert API** aceita uploads de arquivos arbitrários enviadas por clientes externos em diversos formatos (`.wav`, `.mp3`, `.jpg`, `.png`, `.pdf`, `.doc`, `.docx`) e os submete ao processamento assíncrono por binários e parsers de terceiros via CLI (`ffmpeg`, `libreoffice`, `imagemagick`, `poppler-utils`).

Parsers de mídias e documentos de código aberto são historicamente alvos frequentes de vulnerabilidades de segurança de alto impacto, como **Remote Code Execution (RCE)**, **Server-Side Request Forgery (SSRF)**, **Arbitrary File Read/Write** e **Heap Buffer Overflow**.

Dada a premissa de que qualquer arquivo enviado por um usuário pode ser malicioso ou elaborado para explorar um *zero-day* no conversor subjacente, é necessário definir as restrições estritas de isolamento do container e dos subprocessos de conversão.

---

## 2. Decisão de Arquitetura: Restrições de Isolamento

Para garantir o isolamento estrito da esteira de conversão, foram definidas as seguintes restrições de execução e segurança para o container do worker e para a invocação de subprocessos:

### 2.1. Execução sem Privilégios Administrativos (`USER appuser`)
- **Mecanismo:** O container do worker e todas as chamadas `os/exec.CommandContext` executam obrigatoriamente sob o usuário não-root `appuser` (`UID 1000`), sem privilégios de `sudo` ou acesso a `/root`.
- **Risco Mitigado:** Limita o alcance de qualquer execução de código arbitrária (RCE) resultante de estouro de buffer ou leitura fora dos limites (*out-of-bounds*) em parsers binários (ex.: FFmpeg ou Poppler), impedindo o atacante de obter acesso privilegiado no nível de `root` do container ou tentar escapamento (*container breakout*) para o host.

### 2.2. Remoção Completa de Capabilities do Kernel Linux (`--cap-drop=ALL`)
- **Mecanismo:** O container do worker é iniciado com a remoção total de todas as capabilities do Linux (`--cap-drop=ALL`), desativando privilégios de baixo nível do kernel como `CAP_NET_RAW`, `CAP_SYS_ADMIN`, `CAP_SYS_PTRACE`, `CAP_CHOWN` e `CAP_SETUID`.
- **Risco Mitigado:** Inviabiliza a manipulação de interfaces de rede brutas, injeção de código em outros processos via `ptrace`, alteração de proprietários de arquivos do sistema e a exploração de vulnerabilidades no kernel do host a partir do container comprometido.

### 2.3. Prevenção de Escalada de Privilégios (`--security-opt=no-new-privileges:true`)
- **Mecanismo:** Execução sob o flag de segurança `--security-opt=no-new-privileges:true` (ou `no_new_privileges: true` no Docker Compose / Kubernetes Security Context).
- **Risco Mitigado:** Impede que subprocessos filhos gerados durante a conversão (como binários auxiliares invocados pelo ImageMagick ou macros do LibreOffice) ganhem privilégios adicionais através de executáveis que possuam as flags `setuid` ou `setgid`.

### 2.4. Isolamento de Rede durante o Subprocesso de Conversão (`--network=none` / Sandbox Sem Acesso à Rede)
- **Mecanismo:** Durante o processamento da conversão, o subprocesso não possui conectividade com nenhuma rede externa ou interna. As conexões de socket de saída são bloqueadas para os binários conversores.
- **Risco Mitigado:**
  - **Vulnerabilidades do tipo ImageTragick (CVE-2016-3714) e SSRF:** Impede que manipuladores do ImageMagick ou parsers de documentos SVG/HTML acessem URLs externas ou realizem requisições para a rede interna da infraestrutura (*Server-Side Request Forgery*).
  - **Macros Maliciosas e Exfiltração de Dados:** Impede que documentos do Office (`.doc`/`.docx`) contendo macros ou scripts acionem conexões *Command & Control* (C2) para baixar payload secundários ou exfiltrar dados sensíveis fora de banda.

### 2.5. Filesystem Somente Leitura (`read_only: true`) com `tmpfs` Exclusivo para o Job
- **Mecanismo:** O sistema de arquivos raiz do container é montado como estritamente somente leitura (`read_only: true`). A escrita de arquivos durante a conversão é restrita exclusivamente a um diretório temporário montado em memória (`tmpfs` montado em `/tmp` com as flags `noexec,nosuid,nodev`).
- **Risco Mitigado:**
  - **Modificação de Binários e Persistência:** Impede que explorações de gravação arbitrária de arquivos sobrescrevam binários do sistema (`/usr/bin`, `/bin`) ou bibliotecas compartilhadas para obter persistência no container.
  - **Vazamento de Dados Inter-Jobs:** Garantia de que o diretório de trabalho de cada job reside em memória volátil de curta duração e é totalmente destruído ao finalizar a conversão, impedindo que um job subsequente leia vestígios de arquivos de outros usuários.

---

## 3. Matriz de Resumo: Restrição vs. Vulnerabilidade Mitigada

| Restrição de Segurança | Configuração de Infraestrutura | Vulnerabilidade / Risco Primário Mitigado |
| :--- | :--- | :--- |
| **Non-Root User** | `USER appuser` (UID 1000) | RCE / Controle administrativo do container. |
| **Cap-Drop All** | `--cap-drop=ALL` | Manipulação de kernel e injeção de processos. |
| **No New Privileges** | `no-new-privileges:true` | Escalada de privilégios via `setuid` / `setgid`. |
| **Network Isolation** | Sem acesso à rede no conversor | ImageTragick (CVE-2016-3714), SSRF e chamadas C2. |
| **Read-Only Root FS** | `read_only: true` + `tmpfs /tmp` | Alteração de binários, escrita arbitrária e vazamento entre jobs. |

---

## 4. Consequências

* **Positivas:**
  - **Defesa em Profundidade:** A segurança da plataforma não depende da ausência de bugs de parsing nos conversores (FFmpeg, LibreOffice, ImageMagick); mesmo em caso de exploração de um *Zero-Day*, o atacante fica contido e isolado.
  - **Conformidade com CIS Benchmarks:** Atende integralmente aos requisitos de segurança para containers Docker e Kubernetes em ambientes corporativos de alta criticidade.

* **Negativas / Mitigações:**
  - *Restrição de Diretórios de Escrita:* Ferramentas como o LibreOffice exigem diretórios temporários configurados para escrita de perfis de usuário (`/tmp`). (Mitigação: Garantir a montagem de `tmpfs` no diretório `/tmp` do container com capacidade dimensionada).
