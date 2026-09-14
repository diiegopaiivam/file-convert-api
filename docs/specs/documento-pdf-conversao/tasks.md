# Lista de Tarefas de Implementação — Conversão de Documento para PDF (.doc/.docx → .pdf)

Esta é a lista sequencial de tarefas técnicas para o **Dev Senior** implementar a família de conversão de **Documentos de Texto para PDF (`.doc` e `.docx` → `.pdf`)**, em estrita observância ao [design.md](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/specs/documento-pdf-conversao/design.md), [requirements.md](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/specs/documento-pdf-conversao/requirements.md), [ADR 0003](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/adr/0003-document-pdf-conversion-libreoffice-headless-subprocess.md) e [ADR 0005](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/adr/0005-isolamento-subprocesso-conversao.md).

---

## 📋 Lista de Tarefas (Dev Senior)

### Task 1: Erros e Mapeamento no Domínio
- [x] **1.1. Adicionar Novos Erros Tipados em `internal/domain/errors.go`:**
  - Declarar `ErrPasswordProtectedDocument = errors.New("documento protegido por senha")`
  - Declarar `ErrEmptyDocumentPages = errors.New("documento sem paginas ou conteudo renderizavel")`
- [x] **1.2. Mapear Códigos de Erro Textuais:**
  - Mapear `ErrCodePasswordProtectedDocument = "PASSWORD_PROTECTED_DOCUMENT"`
  - Mapear `ErrCodeEmptyDocumentPages = "EMPTY_DOCUMENT_PAGES"`
- [x] **1.3. Validar Pares de Conversão em `internal/domain/validator.go`:**
  - Garantir que `IsValidConversionPair` aceita `(doc, pdf)` e `(docx, pdf)`.

---

### Task 2: Implementar Adaptador `LibreOfficeDocConverter`
- [x] **2.1. Criar Arquivo `internal/adapters/converter/document/doc_to_pdf.go` (ou em `internal/adapters/converter`):**
  - Implementar struct `LibreOfficeDocConverter` implementando a interface `port.Converter`.
  - Implementar o método `CanConvert(srcFormat, targetFormat domain.Format) bool`.
- [x] **2.2. Implementar Validação de Entrada e Sandbox Temporário:**
  - Validar magic bytes/extensão via `domain.ValidateMagicBytes(src, srcFormat)`.
  - Criar o diretório temporário único `/tmp/job_<id>/` e a subpasta de perfil `/tmp/job_<id>/profile/`.
  - Assegurar a limpeza via `defer os.RemoveAll(tmpDir)`.
- [x] **2.3. Implementar Invocação CLI do LibreOffice com Hardening:**
  - Configurar contexto com timeout específico de **90 segundos**: `context.WithTimeout(ctx, 90*time.Second)`.
  - Invocar `os/exec.CommandContext` com as flags exatas de hardening:
    ```bash
    libreoffice --headless --norestore --nodefault --nofirststartwizard --nolockcheck --nologo -env:UserInstallation=file:///tmp/job_<id>/profile --convert-to pdf --outdir /tmp/job_<id>/ /tmp/job_<id>/input.<srcFormat>
    ```
- [x] **2.4. Tratar Resposta, Limite de Páginas e Mapeamento de Erros:**
  - Verificar existência e tamanho do arquivo PDF gerado.
  - Interceptar erros de subprocesso e mapear para `ErrPasswordProtectedDocument` ou `ErrCorruptedFile`.
  - Validar limite máximo de páginas (interromper se $> 100$ páginas com `ErrPageLimitExceeded` ou se $0$ páginas com `ErrEmptyDocumentPages`).
  - Retornar o `io.Reader` do arquivo PDF final.

---

### Task 3: Registrar Adaptador no Startup do Worker
- [x] **3.1. Registrar Adaptador no Startup:**
  - Em `cmd/worker/main.go` (ou `bootstrap.go`), instanciar `LibreOfficeDocConverter`.
  - Registrar no `ConverterRegistry` (`registry.Register(...)`) durante a inicialização da aplicação.

---

### Task 4: Testes Unitários com Mocks/Fakes
- [x] **4.1. Criar `internal/adapters/converter/document/doc_to_pdf_test.go`:**
  - Testar `CanConvert` para os pares `(.doc, .pdf)` e `(.docx, .pdf)`.
  - Testar rejeição de formatos não suportados e arquivos vazios (0 bytes).
  - Testar rejeição de arquivo corrompido ou magic bytes inválidos.
  - Testar comportamento diante de estouro de timeout do contexto (90s).

---

### Task 5: Testes de Integração Real em Container
- [x] **5.1. Criar `internal/adapters/converter/document/doc_integration_test.go`:**
  - Testar a conversão real de um arquivo sintético `.docx` e `.doc` para `.pdf` utilizando a instalação real do LibreOffice no container Debian.
  - Validar a integridade do PDF gerado.
