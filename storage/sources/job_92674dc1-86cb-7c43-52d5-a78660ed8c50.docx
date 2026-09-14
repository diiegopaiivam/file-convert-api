# Requisitos e Especificações Funcionais — Conversão de Documento para PDF (.doc/.docx → .pdf)

## 1. Visão Geral e Escopo

Este documento define os requisitos funcionais, histórias de usuário, comportamentos em cenários de erro e requisitos não funcionais para a família de conversão **Documento para PDF** (`.doc` e `.docx` → `.pdf`) da **FileConvert API**.

O escopo contempla a conversão assíncrona dos dois formatos principais de processadores de texto Microsoft Word:
- **Word 97-2003 Binary Document:** `.doc` → `.pdf`
- **Office OpenXML Document:** `.docx` → `.pdf`

Toda conversão é operada no modelo **assíncrono** (`POST /v1/conversions` → `pending` → `processing` → `done`/`failed`).

---

## 2. Histórias de Usuário e Critérios de Aceite (Gherkin)

### 2.1. Conversão de Documento DOCX para PDF (`.docx` → `.pdf`)

**História de Usuário:**
- **Como** consumidor da API,
- **Quero** enviar um arquivo de documento de texto no formato moderno `.docx`,
- **Para que** ele seja convertido em um documento `.pdf` preservando o layout visual, tipografia, tabelas e elementos gráficos originais.

#### Cenário 1: Conversão de documento DOCX para PDF com sucesso
- **Given** que o cliente possui um arquivo `.docx` válido e dentro dos limites de tamanho e páginas,
- **When** o cliente envia a requisição de conversão solicitando o formato de destino `pdf`,
- **Then** a API aceita a requisição, gera um `job_id` único e retorna status `202 Accepted` com status inicial `pending`,
- **And** quando o worker concluir a conversão, o status transiciona para `done`, disponibilizando um documento `.pdf` válido para download.

---

### 2.2. Conversão de Documento DOC Legado para PDF (`.doc` → `.pdf`)

**História de Usuário:**
- **Como** consumidor da API,
- **Quero** enviar um arquivo de documento de texto no formato binário legado `.doc` (Word 97-2003),
- **Para que** ele seja convertido para o formato padrão imutável `.pdf` garantindo compatibilidade com leitores modernos.

#### Cenário 1: Conversão de documento DOC legado para PDF com sucesso
- **Given** que o cliente possui um arquivo binário `.doc` válido,
- **When** o cliente solicita a conversão para `pdf`,
- **Then** a API processa o job de forma assíncrona,
- **And** ao finalizar o processamento, o status muda para `done`, disponibilizando um documento `.pdf` com fidelidade ao documento original.

---

### 2.3. Critérios de Aceite Globais ("Quando está pronto")

- O job atinge o estado final `done`.
- O documento `.pdf` gerado abre sem alertas ou erros de integridade em leitores PDF padrão.
- **Fidelidade de Layout:** As margens de página, estrutura de colunas, alinhamento de parágrafos, tamanhos e famílias de fonte, tabelas, cabeçalhos, rodapés, paginação e imagens embutidas são preservados fielmente em relação ao documento `.doc`/`.docx` de origem.

---

## 3. Matriz de Casos de Erro

A conversão de Documentos para PDF deve identificar e tratar os seguintes cenários de falha:

| Caso de Erro | Estágio da Detecção | Comportamento Esperado | Código de Erro / Resposta |
| :--- | :--- | :--- | :--- |
| **Arquivo Vazio (0 bytes)** | Upload (Síncrono na Borda) | Rejeitar a requisição imediatamente no momento do upload, sem criar job. | Rejeição HTTP `400 Bad Request` com código `EMPTY_FILE`. |
| **Formato Não Suportado** | Validação de Entrada | Rejeitar arquivos que não possuam magic bytes/extensão referentes a `.doc` ou `.docx`. | Rejeição HTTP `400 Bad Request` com código `UNSUPPORTED_FORMAT`. |
| **Arquivo Corrompido** | Processamento Assíncrono (Worker) | Falha durante a interpretação da estrutura do arquivo `.doc`/`.docx` pelo motor de conversão. | Status `failed` no Job com código `CORRUPTED_FILE`. |
| **Documento com Macros (VBA)** | Processamento Assíncrono (Worker) | **Macros desativadas/ignoradas sem execução.** A conversão gera o PDF normalmente a partir do conteúdo estático do documento. | Processamento concluído com status `done` (conteúdo estático renderizado). |
| **Documento Protegido por Senha** | Processamento Assíncrono (Worker) | O documento exige senha de abertura/leitura de conteúdo para visualização ou exportação. | Status `failed` no Job com código `PASSWORD_PROTECTED_DOCUMENT`. |
| **Documento Sem Conteúdo Renderizável** | Processamento Assíncrono (Worker) | O documento possui 0 páginas ou estrutura sem elementos de texto/gráficos renderizáveis. | Status `failed` no Job com código `EMPTY_DOCUMENT_PAGES`. |
| **Limite de Páginas Excedido** | Processamento Assíncrono (Worker) | O documento possui mais de 100 páginas. | Status `failed` no Job com código `PAGE_LIMIT_EXCEEDED`. |
| **Tamanho de Arquivo Excedido** | Upload (Síncrono na Borda) | O arquivo enviado no upload excede o limite global de 50 MB. | Rejeição HTTP `413 Payload Too Large` com código `FILE_TOO_LARGE`. |
| **Timeout de Execução** | Processamento Assíncrono (Worker) | O tempo de renderização do documento excede a janela limite do conversor (90s). | Status `failed` no Job com código `JOB_TIMEOUT`. |

---

### 3.1. Detalhamento e Justificativa da Regra para Documentos com Macros (VBA)

> [!IMPORTANT]
> **Decisão sobre Arquivos com Macros (VBA/Scripting):**
> **Comportamento Adotado:** As macros presentes em documentos `.doc` e `.docx` (como `.docm` ou arquivos contendo código VBA) devem ser **totalmente ignoradas e desativadas durante a conversão**, gerando o PDF contendo exclusivamente a representação visual e estática do documento.
> 
> **Justificativa Técnica e de Segurança:**
> 1. **Defesa em Profundidade (ADR 0005):** A infraestrutura do worker executa o subprocesso em um container completamente isolado, com rede totalmente bloqueada (`--network=none`), sistema de arquivos somente leitura (`read_only: true`), usuário sem privilégios (`USER appuser`), sem capabilities (`--cap-drop=ALL`) e com prevenção de escalada de privilégios (`no-new-privileges:true`). Mesmo se uma macro contivesse instruções maliciosas, ela seria incapaz de se comunicar com servidores C2 ou alterar o sistema.
> 2. **Experiência do Usuário e Usabilidade:** Muitos modelos corporativos legítimos de relatórios, contratos e formulários utilizam macros para automação interna de preenchimento. Rejeitar o arquivo impediria que usuários convertessem esses documentos para PDF.
> 3. **Processamento Headless Seguro:** O conversor em modo headless executa por padrão com flags de desativação de scripts interativos e macros (`--norestore`), renderizando com segurança apenas a camada estática de texto e layout.

---

## 4. Requisitos Não Funcionais (RNFs)

### 4.1. Timeout Específico do Conversor
- **SLA de Subprocesso:** O limite máximo de tempo para a execução da conversão de um documento `.doc`/`.docx` para `.pdf` é de **90 segundos**.
- **Justificativa em Relação a Outras Famílias:** Conversores de áudio (`ffmpeg`) e imagem (`ImageMagick`/`pdftoppm`) operam em streaming de dados direto e possuem inicialização leve. O motor de renderização de documentos Office em modo headless necessita carregar bibliotecas de diagramação de texto, calcular quebras de página dinâmicas (*reflow* de texto) e mapear famílias de fontes. Uma janela de 90 segundos garante resiliência para documentos extensos (até 100 páginas) e com tabelas complexas sem causar falsos estouros de timeout.

### 4.2. Limite Máximo de Páginas por Documento
- **Teto de Páginas:** Um job desta família processará documentos de no máximo **100 páginas**.
- **Tratamento ao Exceder:** Documentos com mais de 100 páginas serão interrompidos na fase de validação/renderização inicial, transicionando o job para `failed` com o código de erro `PAGE_LIMIT_EXCEEDED`.

### 4.3. Suporte a Fontes e Mapeamento Tipográfico
- **Mapeamento de Fontes:** O ambiente de execução do worker deve conter instaladas as principais coleções de fontes livres e corporativas (ex.: `fonts-liberation`, `fonts-dejavu`, `ttf-mscorefonts-installer`) para minimizar discrepâncias de paginação provocadas por substituição (*font fallback*).

### 4.4. Isolamento e Segurança de Execução
- A conversão de documentos deve cumprir integralmente o plano de isolamento definido no **ADR 0005** (`USER appuser`, `--network=none`, `read_only: true` com `tmpfs /tmp`, `--cap-drop=ALL` e `no-new-privileges:true`).
