# ADR 0006: Definição de Limites Numéricos de Recursos e Timeouts por Família

* **Status:** Aceito
* **Data:** 2026-09-10
* **Autor:** Arquiteto de Software

---

## 1. Contexto

A **FileConvert API** processa três famílias distintas de arquivos (`Áudio`, `Imagem/PDF` e `Documentos Office`), cada uma apresentando características computacionais, consumo de memória RAM e tempos de startup substancialmente diferentes.

- **Áudio (`.wav` ↔ `.mp3`):** Processado via binário `ffmpeg`, caracterizado por alto uso de CPU (encoding de codecs), baixo consumo de memória e inicialização de subprocesso extremamente rápida (~10-20ms).
- **Imagem e PDF (`.jpg`/`.png` ↔ `.pdf`):** Processado via `imagemagick` e `poppler-utils`, exigindo renderização de páginas e manipulação de buffers de imagem na memória.
- **Documento para PDF (`.doc`/`.docx` → `.pdf`):** Processado via `libreoffice` headless, que possui tempo de inicialização de processo mais elevado (~1-2s), alto consumo de memória RAM para carregamento do motor de layout/fontes e renderização de tabelas e gráficos.

Sem a delimitação rígida de limites numéricos por família de conversão e por container, um único arquivo malicioso ou excessivamente grande poderia causar o esgotamento de memória (Out-Of-Memory / OOM), monopolizar a CPU do host ou travar a esteira de workers assíncronos.

---

## 2. Decisão de Arquitetura: Limites Numéricos de Recursos

Ficam estabelecidos os seguintes limites numéricos e operacionais para a versão v1 da FileConvert API:

### 2.1. Tamanho Máximo de Arquivo na Borda da API
- **Limite Máximo por Upload:** **50 MB** (`52.428.800 bytes`).
- **Comportamento na Rejeição:** Requisições enviando arquivos acima de 50 MB devem ser rejeitadas síncronamente na borda HTTP antes do salvamento no storage ou enfileiramento, retornando HTTP `413 Payload Too Large` com o código `FILE_TOO_LARGE`.
- **Justificativa:** 50 MB contempla um arquivo de áudio `.wav` não-compactado de ~5 minutos em qualidade CD, documentos `.docx` contendo dezenas de imagens integradas e arquivos `.pdf` de alta resolução, mantendo o consumo de IO e rede dentro dos limites aceitáveis para o free tier e servidores de aplicação.

---

### 2.2. Timeouts Máximos por Família de Conversão (SLA & Context Timeout)

O tempo limite máximo de transição do job (do estado `pending`/`processing` até a resolução) varia por família, honrando o **SLA global máximo de 60 segundos** estipulado no RNF 4.2:

| Família de Conversão | Formatos Suportados | Ferramenta Subjacente | Timeout Máximo (`context.WithTimeout`) | Justificativa Técnica |
| :--- | :--- | :--- | :--- | :--- |
| **Família 1: Áudio** | `.wav` ↔ `.mp3` | `ffmpeg` | **30 segundos** | A conversão de áudio binário via FFmpeg é altamente eficiente em CPU. Arquivos dentro do limite de 50 MB são convertidos em menos de 5 segundos em condições normais; 30s é suficiente para cobrir picos de carga sem reter workers desnecessariamente. |
| **Família 2: Imagem & PDF** | `.jpg`/`.png` ↔ `.pdf` | `imagemagick` / `poppler` | **45 segundos** | PDFs de múltiplas páginas exigem rasterização página a página e alocação de buffers de imagem na memória, exigindo janela temporal ligeiramente superior à de áudio. |
| **Família 3: Documento → PDF** | `.doc`/`.docx` → `.pdf` | `libreoffice` (headless) | **60 segundos** | O LibreOffice headless possui overhead de inicialização de runtime JVM/UNO, cálculo de diagramação de layout, carregamento de fontes e exportação de PDF. Define o teto máximo do SLA da v1. |
| **Teto Máximo do SLA Global** | Todos os Formatos | N/A | **60 segundos (Rígido)** | Se o processamento ultrapassar 60s, o job transiciona obrigatoriamente para `failed` com `JOB_TIMEOUT`. |

---

### 2.3. Limites de Recursos de Hardware por Container Worker

Cada container da réplica de `worker` (instância de processo isolado) terá os seguintes limites de alocação de recursos definidos no orquestrador (Docker Compose / Kubernetes):

- **Limite Rígido de Memória RAM (Memory Hard Limit):** **1024 MB (1 GB)**.
  - *Memory Soft Limit / Reservation:* `512 MB`.
  - *Justificativa:* O LibreOffice em modo headless consome entre 250 MB e 450 MB de RAM ao carregar documentos complexos. Somado à montagem do sistema de arquivos temporário em memória (`tmpfs` em `/tmp` de até 512 MB), o teto de 1 GB previne que o container seja exterminado pelo OOM-Killer do Linux durante conversões concorrentes pesadas.
- **Limite de Processamento de CPU (CPU Limit):** **1.5 vCPU (1500m)**.
  - *CPU Reservation / Request:* `0.5 vCPU (500m)`.
  - *Justificativa:* Permite que o encoder LAME do FFmpeg e o renderer do LibreOffice aproveitem threads paralelas para acelerar o processamento ativo sem permitir que um único container de worker monopolize 100% dos núcleos de CPU de um nó de 2 vCPUs ou 4 vCPUs da infraestrutura em nuvem.

---

## 3. Matriz Resumida de Limites e Ações em Caso de Estouro

| Recurso / Parâmetro | Limite Numérico | Ação ao Atingir o Limite | Código de Erro / Resposta |
| :--- | :--- | :--- | :--- |
| **Tamanho de Upload** | `50 MB` | Rejeição HTTP síncrona imediata | HTTP 413 (`FILE_TOO_LARGE`) |
| **Timeout Áudio** | `30s` | Encerramento de subprocesso (`SIGKILL`) | Status `failed` (`JOB_TIMEOUT`) |
| **Timeout Imagem/PDF** | `45s` | Encerramento de subprocesso (`SIGKILL`) | Status `failed` (`JOB_TIMEOUT`) |
| **Timeout Documentos** | `60s` | Encerramento de subprocesso (`SIGKILL`) | Status `failed` (`JOB_TIMEOUT`) |
| **Memória por Worker** | `1024 MB (1GB)` | Reinício isolado do container (OOM) | Status `failed` (`INTERNAL_ERROR`) |
| **CPU por Worker** | `1.5 vCPU` | Throttling de ciclos de CPU pelo cgroups | Degradação controlada de latência |

---

## 4. Consequências

* **Positivas:**
  - **Estabilidade do Sistema:** Proteção garantida contra ataques de Negação de Serviço por consumo exaustivo de memória ou CPU (DoS/ReDoS).
  - **Uso Eficiente dos Workers:** Liberar workers mais rapidamente para a fila ao aplicar timeouts menores para famílias leves (ex.: 30s para áudio).
  - **Previsibilidade Operacional:** Permite dimensionar a quantidade de réplicas de workers com base na capacidade de RAM e CPU da VM do servidor.

* **Negativas / Mitigações:**
  - *Documentos Extremamente Extensos:* Documentos com centenas de páginas que exijam mais de 60 segundos para conversão no LibreOffice falharão com `JOB_TIMEOUT`. (Mitigação: Orientar na documentação do produto o tamanho e complexidade recomendada dos arquivos para a versão v1).
