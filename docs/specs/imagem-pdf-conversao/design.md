# Desenho de Arquitetura e Design Técnico — Conversão Imagem ↔ PDF

## 1. Visão Geral e Arquitetura Hexagonal

Este documento especifica o design de arquitetura técnica para a implementação dos adaptadores de conversão de **Imagem ↔ PDF** na **FileConvert API**.

Seguindo o padrão de **Arquitetura Hexagonal (Ports & Adapters)** estabelecido em [docs/architecture/ports.md](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/ports.md) e ratificado no [ADR 0009](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/adr/0009-conversao-imagem-pdf.md):
- A lógica de conversão é totalmente isolada dos casos de uso de negócio.
- O núcleo da aplicação interage unicamente com a interface `port.Converter` e resolve os adaptadores adequados via `port.ConverterRegistry`.
- Dois novos adaptadores serão criados no pacote `internal/adapters/converter`:
  1. `ImageToPDFConverter`: Trata a conversão `.jpg`, `.png` → `.pdf`.
  2. `PDFToImageConverter`: Trata a conversão `.pdf` → `.jpg`.

---

## 2. Diagrama de Componentes e Fluxo de Execução

```
+---------------------------------------------------------------------------------+
|                                 Worker Loop                                     |
+---------------------------------------------------------------------------------+
                                       |
                                       v
                    +------------------------------------+
                    |   JobQueue.Dequeue() -> jobID      |
                    +------------------+-----------------+
                                       |
                                       v
                    +------------------------------------+
                    |  JobRepository.GetByID(jobID)      |
                    +------------------+-----------------+
                                       |
                                       v
                    +------------------------------------+
                    | ConverterRegistry.GetConverter()   |
                    | (srcFormat, targetFormat)          |
                    +------------------+-----------------+
                                       |
            +--------------------------+--------------------------+
            | (src=jpg/png, target=pdf)                           | (src=pdf, target=jpg)
            v                                                     v
+-----------------------+                             +-----------------------+
|  ImageToPDFConverter  |                             |  PDFToImageConverter  |
|  (ImageMagick convert)|                             |  (poppler pdftoppm)   |
+-----------+-----------+                             +-----------+-----------+
            |                                                     |
            | Transforma imagem em PDF                            | Renderiza paginas 300 DPI
            |                                                     | Se N=1 -> page-1.jpg
            |                                                     | Se N>1 -> empacota .zip
            +--------------------------+--------------------------+
                                       |
                                       v
                    +------------------------------------+
                    |   ObjectStorage.Upload(targetKey)  |
                    +------------------+-----------------+
                                       |
                                       v
                    +------------------------------------+
                    | JobRepository.UpdateStatus(done)   |
                    +------------------------------------+
```

---

## 3. Especificação Técnica dos Adaptadores

### 3.1. `ImageToPDFConverter` (`.jpg`/`.png` → `.pdf`)

- **Pacote:** `internal/adapters/converter`
- **Arquivo:** `image_to_pdf.go`
- **Interface Implementada:** `ports.Converter`

#### Lógica de Execução:
1. **CanConvert(srcFormat, targetFormat):**
   - Retorna `true` se `(srcFormat == "jpg" || srcFormat == "png") && targetFormat == "pdf"`.
2. **Convert(ctx, srcReader, srcFormat, targetFormat):**
   - Valida os magic bytes da imagem de entrada via `domain.ValidateMagicBytes(...)`.
   - Cria um diretório temporário isolado `/tmp/job_<job_id>/`.
   - Escreve o stream de entrada no arquivo `/tmp/job_<job_id>/input.<srcFormat>`.
   - Invocação do subprocesso isolado via `os/exec.CommandContext`:
     ```bash
     convert -limit memory 256MB -limit map 512MB /tmp/job_<job_id>/input.<srcFormat> /tmp/job_<job_id>/output.pdf
     ```
   - Abre o arquivo PDF gerado `/tmp/job_<job_id>/output.pdf` e retorna o seu `io.Reader`.
   - Garante a remoção do diretório temporário via `defer os.RemoveAll(tmpDir)`.

---

### 3.2. `PDFToImageConverter` (`.pdf` → `.jpg`)

- **Pacote:** `internal/adapters/converter`
- **Arquivo:** `pdf_to_image.go`
- **Interface Implementada:** `ports.Converter`

#### Lógica de Execução:
1. **CanConvert(srcFormat, targetFormat):**
   - Retorna `true` se `srcFormat == "pdf" && targetFormat == "jpg"`.
2. **Convert(ctx, srcReader, srcFormat, targetFormat):**
   - Valida os magic bytes de PDF (`%PDF-`) via `domain.ValidateMagicBytes(...)`.
   - Cria um diretório temporário isolado `/tmp/job_<job_id>/`.
   - Escreve o arquivo PDF recebido em `/tmp/job_<job_id>/input.pdf`.
   - Invocação do binário `pdftoppm`:
     ```bash
     pdftoppm -jpeg -r 300 /tmp/job_<job_id>/input.pdf /tmp/job_<job_id>/page
     ```
   - O `pdftoppm` gera arquivos de imagem no padrão `/tmp/job_<job_id>/page-1.jpg`, `page-2.jpg`, ..., `page-N.jpg`.
   - **Validação de Páginas:**
     - Se $N == 0$: Retorna erro `domain.ErrEmptyPDFPages`.
     - Se $N > 50$: Retorna erro `domain.ErrPageLimitExceeded`.
   - **Processamento de Saída:**
     - **Caso $N == 1$ (Página Única):** Retorna o leitor `io.Reader` contendo o binário direto do arquivo `page-1.jpg`.
     - **Caso $N > 1$ (Multipáginas):**
       - Cria um buffer ou arquivo de arquivo `.zip` temporário utilizando o pacote de stdlib `archive/zip`.
       - Percorre os $N$ arquivos em ordem numérica (`page-1.jpg` a `page-N.jpg`), adicionando cada arquivo no zip com o nome `page-1.jpg`, `page-2.jpg`, etc.
       - Fecha o gravador zip e retorna o `io.Reader` do arquivo `.zip` final.

---

## 4. Mapeamento de Erros e Falhas de Subprocesso

O adaptador intercepta erros de execução do subprocesso e converte para exceções tipadas de domínio:

```go
func mapSubprocessError(output string, err error) error {
	outStr := strings.ToLower(output)
	if strings.Contains(outStr, "password") || strings.Contains(outStr, "encrypted") {
		return domain.ErrPasswordProtectedPDF
	}
	if strings.Contains(outStr, "corrupt") || strings.Contains(outStr, "no header") {
		return domain.ErrCorruptedFile
	}
	return fmt.Errorf("%w: %v - output: %s", domain.ErrInternalError, err, output)
}
```

---

## 5. Garantias de Segurança e Recursos

1. **Timeout Controlado por Contexto:** A execução dos subprocessos utiliza `exec.CommandContext(ctx, ...)` respeitando o SLA de 60 segundos do contexto do worker.
2. **Políticas de Memória do ImageMagick:** Uso explicito de `-limit memory 256MB -limit map 512MB` para impedir consumo abusivo de RAM.
3. **Limpeza de Arquivos Temporários:** Todo diretório criado em `/tmp/job_<id>/` é sumariamente removido após a leitura do stream de saída.
