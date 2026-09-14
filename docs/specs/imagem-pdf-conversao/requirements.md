# Requisitos e Especificações Funcionais — Conversão Imagem ↔ PDF

## 1. Visão Geral e Escopo

Este documento define os requisitos funcionais, histórias de usuário, comportamentos em cenários de erro e requisitos não funcionais para a família de conversão **Imagem ↔ PDF** da **FileConvert API**.

O escopo contempla a conversão assíncrona bidirecional:
- **Imagem para PDF:** `.jpg` ou `.png` → `.pdf`
- **PDF para Imagem:** `.pdf` → `.jpg`

Toda conversão é operada no modelo **assíncrono** (`POST /v1/conversions` → `pending` → `processing` → `done`/`failed`).

---

## 2. Histórias de Usuário e Critérios de Aceite (Gherkin)

### 2.1. Conversão de Imagem para PDF (`.jpg` / `.png` → `.pdf`)

**História de Usuário:**
- **Como** consumidor da API,
- **Quero** enviar um arquivo de imagem nos formatos `.jpg` ou `.png`,
- **Para que** ele seja convertido em um documento `.pdf` mantendo a qualidade visual, proporções originais e orientação.

#### Cenário 1: Conversão de imagem JPEG para PDF com sucesso
- **Given** que o cliente possui um arquivo de imagem no formato `.jpg` válido e dentro dos limites de tamanho e resolução,
- **When** o cliente envia a requisição de conversão solicitando o formato de destino `pdf`,
- **Then** a API aceita a requisição, gera um `job_id` único e retorna status `202 Accepted` com status inicial `pending`,
- **And** quando o worker concluir a conversão, o status transiciona para `done`, disponibilizando um documento `.pdf` válido de página única contendo a imagem renderizada com proporção preservada.

#### Cenário 2: Conversão de imagem PNG com transparência para PDF com sucesso
- **Given** que o cliente possui uma imagem `.png` com canal alfa (transparência),
- **When** o cliente solicita a conversão para `pdf`,
- **Then** a API processa a imagem substituindo o fundo transparente por uma cor neutra (fundo branco padrão),
- **And** gera o documento `.pdf` resultante com o status final `done`.

**Critério de Aceite ("Quando está pronto"):**
- O job atinge o estado final `done`.
- O arquivo `.pdf` gerado abre em qualquer leitor PDF padrão sem mensagens de erro.
- A imagem ocupa a página mantendo a proporção de aspecto (*aspect ratio*) original, sem distorções visuais ou cortes não intencionais.

---

### 2.2. Conversão de PDF para Imagem (`.pdf` → `.jpg`) e Tratamento Multipáginas

**História de Usuário:**
- **Como** consumidor da API,
- **Quero** enviar um documento no formato `.pdf`,
- **Para que** cada página seja convertida em uma imagem legível no formato `.jpg`.

#### Decisão de Arquitetura e Comportamento para PDFs Multipáginas:

> [!IMPORTANT]
> **Decisão sobre Saída de PDF para JPG:**
> 1. **PDF de Página Única (1 página):** A saída é gerada diretamente como um único arquivo de imagem `.jpg` (`{job_id}.jpg`).
> 2. **PDF Multipáginas (2 a 50 páginas):** Todas as páginas são renderizadas como imagens individuais numeradas (`page-1.jpg`, `page-2.jpg`, ..., `page-N.jpg`) e empacotadas em um arquivo compactado **`.zip`** (`{job_id}.zip`).
> 
> **Justificativa:** O schema da entidade `Job` no domínio e os contratos dos ports `ObjectStorage` e `JobRepository` utilizam uma chave de objeto única (`TargetObjectKey`). Retornar um arquivo `.zip` em PDFs multipáginas preserva a integridade da arquitetura de chave única no storage e no banco de dados, permitindo que o cliente baixe todo o conjunto de páginas renderizadas em um único download.

#### Cenário 1: Conversão de PDF de página única para JPG com sucesso
- **Given** que o cliente possui um documento `.pdf` válido contendo exatamente 1 página,
- **When** o cliente solicita a conversão para `jpg`,
- **Then** a API processa o job e retorna status final `done`,
- **And** a URL de download entrega diretamente o arquivo de imagem `.jpg` correspondente à página renderizada.

#### Cenário 2: Conversão de PDF multipáginas para JPG com sucesso
- **Given** que o cliente possui um documento `.pdf` válido contendo 5 páginas,
- **When** o cliente solicita a conversão para `jpg`,
- **Then** a API renderiza todas as 5 páginas como imagens `.jpg` individuais (`page-1.jpg` a `page-5.jpg`),
- **And** empacota as imagens em um arquivo `.zip`, disponibilizando a chave do `.zip` no `TargetObjectKey` com status `done`.

**Critério de Aceite ("Quando está pronto"):**
- O job atinge o estado `done`.
- Em PDFs de 1 página: o arquivo baixado é uma imagem `.jpg` válida.
- Em PDFs multipáginas: o arquivo baixado é um `.zip` válido contendo exatamente $N$ imagens `.jpg`, sendo cada imagem correspondente a uma página do documento original na resolução estipulada.

---

## 3. Matriz de Casos de Erro

A conversão Imagem ↔ PDF deve identificar e tratar os seguintes cenários de falha:

| Caso de Erro | Estágio da Detecção | Comportamento Esperado | Código de Erro / Resposta |
| :--- | :--- | :--- | :--- |
| **Imagem Corrompida** | Processamento Assíncrono (Worker) | Falha na leitura dos magic bytes ou no parse do cabeçalho da imagem JPEG/PNG. | Status `failed` no Job com código `CORRUPTED_FILE`. |
| **PDF Protegido por Senha** | Processamento Assíncrono (Worker) | O documento PDF exige senha de criptografia para leitura/renderização. | Status `failed` no Job com código `PASSWORD_PROTECTED_PDF`. |
| **PDF Sem Páginas / Em Branco** | Processamento Assíncrono (Worker) | O arquivo PDF possui estrutura de catálogo de páginas vazia (0 páginas). | Status `failed` no Job com código `EMPTY_PDF_PAGES`. |
| **Resolução Excedida** | Validação / Processamento | A dimensão da imagem excede o teto máximo permitido de 10.000 x 10.000 pixels (100 MP). | Status `failed` no Job com código `IMAGE_RESOLUTION_EXCEEDED`. |
| **Limite de Páginas Excedido** | Processamento Assíncrono (Worker) | O PDF possui mais de 50 páginas. | Status `failed` no Job com código `PAGE_LIMIT_EXCEEDED`. |
| **Tamanho de Arquivo Excedido** | Upload (Síncrono na Borda) | O arquivo enviado no upload excede o limite global de 50 MB. | Rejeição HTTP `413 Payload Too Large` com código `FILE_TOO_LARGE`. |

---

## 4. Requisitos Não Funcionais (RNFs)

### 4.1. Resolução Padrão e Teto Máximo de Dimensão
- **Resolução de Renderização de PDF para JPG:** A renderização de páginas PDF em imagem `.jpg` deve ser executada na densidade padrão de **300 DPI** (*Dots Per Inch*), garantindo nitidez para leitura de textos pequenos.
- **Dimensão Máxima de Imagem (Pixels):** A dimensão máxima permitida para qualquer imagem de entrada ou saída é de **10.000 x 10.000 pixels** (teto de 100 Megapixels).
  - *Objetivo de Segurança:* Mitigar ataques de exaustão de memória RAM por bombas de descompressão (*Decompression Bombs / Pixel Bombs*).

### 4.2. Limite Máximo de Páginas por Job
- **Teto de Páginas por PDF:** Um job de conversão `.pdf` → `.jpg` processará no máximo **50 páginas** por documento.
- **Tratamento ao Exceder:** PDFs que contiverem mais de 50 páginas serão rejeitados na etapa de processamento assíncrono com o erro `PAGE_LIMIT_EXCEEDED`, prevenindo estouros no SLA de 60 segundos do worker.

### 4.3. Limite de Tamanho do Payload
- **Tamanho Máximo do Arquivo:** Mantido o teto global de **50 MB** por arquivo enviado (alinhado ao **ADR 0006**).
