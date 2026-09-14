# ADR 0009: Arquitetura de Conversão Imagem ↔ PDF e Tratamento de Saída Multipágina em ZIP

* **Status:** Aceito
* **Data:** 2026-09-14
* **Autor:** Arquiteto de Software

---

## 1. Contexto

A especificação de requisitos [docs/specs/imagem-pdf-conversao/requirements.md](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/specs/imagem-pdf-conversao/requirements.md) estabelece os critérios para a família de conversão **Imagem ↔ PDF**:
1. `.jpg` / `.png` → `.pdf`
2. `.pdf` → `.jpg`

Esta família apresenta um desafio de modelagem no padrão Hexagonal (Ports & Adapters):
- Uma conversão de imagem para PDF resulta em um único documento PDF.
- Porém, um documento PDF de entrada pode conter múltiplas páginas (ex.: 5 páginas). Na conversão `.pdf` → `.jpg`, a renderização gráfica produz $N$ arquivos de imagem individuais (uma imagem para cada página).
- O modelo de domínio `domain.Job` e os contratos dos ports `ObjectStorage` e `JobRepository` definem que a saída do job é identificada por um campo único: `TargetObjectKey string`.

É necessário definir a estratégia de execução dos subprocessos nativos e resolver a representação de múltiplos arquivos sem introduzir *breaking changes* na estrutura de dados e interfaces existentes.

---

## 2. Decisão de Arquitetura

Ficam estabelecidas as seguintes diretrizes de arquitetura para a família Imagem ↔ PDF:

---

### 2.1. Estratégia de Execução dos Subprocessos

1. **Direção 1: Imagem para PDF (`.jpg` / `.png` → `.pdf`)**
   - **Adaptador:** `ImageToPDFConverter` implementando a interface `port.Converter`.
   - **Ferramenta de Subprocesso:** Binário nativo **ImageMagick** (`convert` / `magick`).
   - **Execução:** O adaptador invoca o subprocesso passando o stream de entrada e gerando o PDF resultante com preservação de proporções e densidade de cor.
   - **Comando base:** `convert -limit memory 256MB -limit map 512MB input_img output.pdf`

2. **Direção 2: PDF para Imagem (`.pdf` → `.jpg`)**
   - **Adaptador:** `PDFToImageConverter` implementando a interface `port.Converter`.
   - **Ferramenta de Subprocesso:** Binário nativo **`pdftoppm`** (do pacote `poppler-utils`).
   - **Execução:** O adaptador invoca `pdftoppm` renderizando as páginas em formato JPEG com densidade de **300 DPI**.
   - **Comando base:** `pdftoppm -jpeg -r 300 input.pdf /tmp/job_xxx/page`

---

### 2.2. Representação do Resultado Multipágina no Job e ObjectStorage

Para responder à conversão de PDFs com múltiplas páginas mantendo 100% de compatibilidade com o schema atual do Job e os contratos de ports:

1. **PDF de Página Única (1 página):**
   - A imagem renderizada é salva diretamente no `ObjectStorage` sob a chave `targets/<job_id>.jpg`.
   - O campo `TargetObjectKey` da entidade `Job` recebe o valor `"targets/<job_id>.jpg"`.

2. **PDF Multipáginas (2 a 50 páginas):**
   - Todas as $N$ páginas renderizadas (`page-1.jpg`, `page-2.jpg`, ..., `page-N.jpg`) são empacotadas dentro de um único arquivo de arquivo compactado **`.zip`** em memória/diretório temporário.
   - O arquivo `.zip` resultante é armazenado no `ObjectStorage` sob a chave `targets/<job_id>.zip`.
   - O campo `TargetObjectKey` recebe `"targets/<job_id>.zip"`.

3. **Preservação de Schema e Contratos:**
   - **Zero Breaking Changes:** A escolha de empacotar múltiplos arquivos em um único `.zip` mantém o tipo `TargetObjectKey string` intacto no `domain.Job`, no `JobRepository` e no `ObjectStorage`.
   - O cliente ao consultar `GET /v1/jobs/{job_id}` obtém a URL de download `/v1/jobs/{job_id}/download` que entregará o arquivo `.jpg` (se 1 página) ou o arquivo `.zip` (se multipáginas).

---

### 2.3. Registro e Resolução no `ConverterRegistry`

Os dois adaptadores são registrados no startup da aplicação no `ConverterRegistry`:
- `ImageToPDFConverter`: Método `CanConvert(src, target)` retorna `true` para `(jpg, pdf)` e `(png, pdf)`.
- `PDFToImageConverter`: Método `CanConvert(src, target)` retorna `true` para `(pdf, jpg)`.

---

## 3. Matriz de Mapeamento de Erros de Processamento

| Condição de Erro | Mapeamento no Subprocesso | Ação do Adaptador | Código no Job |
| :--- | :--- | :--- | :--- |
| **Imagem Corrompida** | Exit code > 0 no ImageMagick | Captura erro de decode e falha job | `CORRUPTED_FILE` |
| **PDF Protegido por Senha** | Exit code do `pdftoppm` indicando encriptação | Detecta mensagem "Command Line Error: Incorrect password" | `PASSWORD_PROTECTED_PDF` |
| **PDF Sem Páginas / Em Branco** | Zero arquivos `page-*.jpg` gerados pelo `pdftoppm` | Detecta ausência de páginas renderizadas | `EMPTY_PDF_PAGES` |
| **Excesso de Páginas (> 50)** | Contagem de páginas extraídas > 50 | Cancela o empacotamento e falha o job | `PAGE_LIMIT_EXCEEDED` |
| **Dimensão > 10.000 x 10.000 px** | Inspeção do tamanho de imagem gerada | Rejeita imagem que exceda o teto de 100 MP | `IMAGE_RESOLUTION_EXCEEDED` |

---

## 4. Consequências

* **Positivas:**
  - **Arquitetura Preservada:** Nenhuma alteração nos contratos dos ports `JobRepository` ou `ObjectStorage` é necessária.
  - **Fidelidade de Renderização:** Utilização de ferramentas industriais consolidadas (`pdftoppm` e `ImageMagick`) garante 300 DPI e alta fidelidade visual.
  - **Simplicidade para Clientes Integradores:** O cliente baixa um único pacote `.zip` em casos de PDFs multipáginas sem necessitar de requisições individuais por página.

* **Negativas / Mitigações:**
  - *Uso de Disco Temporário:* PDFs multipáginas exigem salvar temporariamente as imagens em `/tmp` antes de compactar em `.zip`.
  - *Mitigação:* Utilização de diretórios temporários delimitados (`/tmp/job_<id>/`) com limpeza garantida via `defer os.RemoveAll(...)` no fim da execução do adaptador.
