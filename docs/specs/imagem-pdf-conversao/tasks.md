# Lista de Tarefas de Implementação — Conversão Imagem ↔ PDF

Esta é a lista sequencial de tarefas técnicas para o **Dev Senior** implementar a família de conversão **Imagem ↔ PDF** conforme o [design.md](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/specs/imagem-pdf-conversao/design.md) e [ADR 0009](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/adr/0009-conversao-imagem-pdf.md).

---

## 📋 Lista de Tarefas (Dev Senior)

### Task 1: Erros e Validações no Domínio
- [ ] **1.1. Adicionar Novos Erros Tipados em `internal/domain/errors.go`:**
  - Declarar `ErrPasswordProtectedPDF = errors.New("pdf protegido por senha")`
  - Declarar `ErrEmptyPDFPages = errors.New("pdf sem paginas legiveis")`
  - Declarar `ErrPageLimitExceeded = errors.New("pdf excede o limite maximo de 50 paginas")`
  - Declarar `ErrImageResolutionExceeded = errors.New("imagem excede a resolucao maxima de 10000x10000 pixels")`
- [ ] **1.2. Atualizar Mapeamento de Códigos de Erro:**
  - Mapear `ErrCodePasswordProtectedPDF = "PASSWORD_PROTECTED_PDF"`
  - Mapear `ErrCodeEmptyPDFPages = "EMPTY_PDF_PAGES"`
  - Mapear `ErrCodePageLimitExceeded = "PAGE_LIMIT_EXCEEDED"`
  - Mapear `ErrCodeImageResolutionExceeded = "IMAGE_RESOLUTION_EXCEEDED"`
- [ ] **1.3. Validar Par de Conversão em `internal/domain/validator.go`:**
  - Confirmar que `IsValidConversionPair` aceita `(jpg, pdf)`, `(png, pdf)` e `(pdf, jpg)`.

---

### Task 2: Implementar Adaptador `ImageToPDFConverter`
- [ ] **2.1. Criar Arquivo `internal/adapters/converter/image_to_pdf.go`:**
  - Implementar struct `ImageToPDFConverter` e o método `CanConvert(src, target Format) bool`.
- [ ] **2.2. Implementar Método `Convert`:**
  - Validar magic bytes via `domain.ValidateMagicBytes(src, srcFormat)`.
  - Criar diretório temporário isolado `/tmp/job_<id>/` com remoção via `defer os.RemoveAll(dir)`.
  - Executar o binário ImageMagick (`convert` / `magick`) com limites de memória (`-limit memory 256MB`).
  - Retornar o `io.Reader` do arquivo `.pdf` gerado.

---

### Task 3: Implementar Adaptador `PDFToImageConverter` (com suporte a ZIP)
- [ ] **3.1. Criar Arquivo `internal/adapters/converter/pdf_to_image.go`:**
  - Implementar struct `PDFToImageConverter` e o método `CanConvert(src, target Format) bool`.
- [ ] **3.2. Implementar Método `Convert`:**
  - Validar magic bytes de PDF (`%PDF-`).
  - Executar o binário `pdftoppm -jpeg -r 300 input.pdf /tmp/job_<id>/page`.
  - Mapear erros de subprocesso (senha de PDF, PDF corrompido).
- [ ] **3.3. Implementar Lógica Multipágina e ZIP:**
  - Contar o número de arquivos de página gerados (`page-1.jpg` a `page-N.jpg`).
  - Rejeitar se $N == 0$ (`ErrEmptyPDFPages`) ou $N > 50$ (`ErrPageLimitExceeded`).
  - Se $N == 1$: Retornar o stream do arquivo `.jpg` diretamente.
  - Se $N > 1$: Compactar todos os arquivos `page-i.jpg` em um arquivo `.zip` utilizando `archive/zip` e retornar o stream do `.zip`.

---

### Task 4: Registrar Adaptadores no Startup do Worker
- [ ] **4.1. Registrar Adaptadores em `cmd/worker/main.go`:**
  - Instanciar `ImageToPDFConverter` e `PDFToImageConverter`.
  - Registrar no `ConverterRegistry` (`registry.Register(...)`) antes de iniciar o loop de consumo da fila.

---

### Task 5: Testes Unitários e Validação de Integração
- [ ] **5.1. Criar Testes de Unidade em `internal/adapters/converter/image_to_pdf_test.go`:**
  - Testar conversão com imagem de teste válida `.jpg` e `.png`.
  - Testar rejeição de imagem corrompida.
- [ ] **5.2. Criar Testes de Unidade em `internal/adapters/converter/pdf_to_image_test.go`:**
  - Testar conversão de PDF de 1 página gerando `.jpg`.
  - Testar conversão de PDF multipáginas gerando pacote `.zip`.
- [ ] **5.3. Validação em Container Docker:**
  - Executar `docker compose build worker` e validar execução end-to-end.
