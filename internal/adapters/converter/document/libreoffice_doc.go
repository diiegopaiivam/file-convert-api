package document

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
)

var _ ports.Converter = (*LibreOfficeDocConverter)(nil)

const MaxDocumentPagesAllowed = 100

// LibreOfficeDocConverter realiza a conversão de documentos (.doc / .docx → .pdf) via subprocesso LibreOffice.
type LibreOfficeDocConverter struct {
	sofficePath string
}

// NewLibreOfficeDocConverter cria uma nova instância do adaptador LibreOfficeDocConverter.
func NewLibreOfficeDocConverter(sofficePath string) *LibreOfficeDocConverter {
	if sofficePath == "" {
		sofficePath = "soffice"
	}
	return &LibreOfficeDocConverter{
		sofficePath: sofficePath,
	}
}

// CanConvert verifica se o par de formatos é suportado (.doc → .pdf ou .docx → .pdf).
func (c *LibreOfficeDocConverter) CanConvert(srcFormat, targetFormat domain.Format) bool {
	validDocToPdf := (srcFormat == domain.FormatDOC && targetFormat == domain.FormatPDF)
	validDocxToPdf := (srcFormat == domain.FormatDOCX && targetFormat == domain.FormatPDF)
	return validDocToPdf || validDocxToPdf
}

// Convert executa a conversão chamando o subprocesso do LibreOffice com perfil de usuário temporário isolado.
func (c *LibreOfficeDocConverter) Convert(ctx context.Context, src io.Reader, srcFormat, targetFormat domain.Format) (io.Reader, error) {
	if !c.CanConvert(srcFormat, targetFormat) {
		return nil, domain.ErrUnsupportedFormat
	}

	// 1. Validar magic bytes de entrada para o formato declarado
	validatedStream, err := domain.ValidateMagicBytes(src, srcFormat)
	if err != nil {
		return nil, err
	}

	// 2. Criar diretório temporário para isolar os arquivos do job
	tmpDir, err := os.MkdirTemp("", "doc-convert-*")
	if err != nil {
		return nil, fmt.Errorf("%w: falha ao criar diretório temporário: %v", domain.ErrInternalError, err)
	}
	defer os.RemoveAll(tmpDir)

	// 3. Criar perfil temporário de usuário descartável para o LibreOffice (-env:UserInstallation)
	// Isso impede acúmulo de estado/locks entre jobs e garante isolamento concorrente
	userProfileDir, err := os.MkdirTemp("", "libreoffice-profile-*")
	if err != nil {
		return nil, fmt.Errorf("%w: falha ao criar perfil temporário do LibreOffice: %v", domain.ErrInternalError, err)
	}
	defer os.RemoveAll(userProfileDir)

	inputPath := filepath.Join(tmpDir, fmt.Sprintf("input.%s", srcFormat))
	outputPath := filepath.Join(tmpDir, "input.pdf")

	// 4. Escrever stream de entrada no arquivo temporário
	inputFile, err := os.Create(inputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: falha ao criar arquivo temporário de entrada: %v", domain.ErrInternalError, err)
	}

	n, err := io.Copy(inputFile, validatedStream)
	_ = inputFile.Close()
	if err != nil {
		return nil, fmt.Errorf("%w: falha ao copiar stream de entrada: %v", domain.ErrInternalError, err)
	}

	if n == 0 {
		return nil, domain.ErrEmptyFile
	}

	// Formatar a URI do perfil temporário
	userProfileURI := "file://" + filepath.ToSlash(userProfileDir)

	// 5. Parâmetros de execução otimizados e isolados do LibreOffice:
	// -env:UserInstallation -> Perfil de usuário isolado e descartável por job
	// --headless            -> Execução sem interface gráfica
	// --norestore           -> Não restaurar sessões de documentos anteriores
	// --nodefault           -> Não carregar templates padrão
	// --nofirststartwizard  -> Desativar wizard de primeira inicialização
	// --nolockcheck         -> Desabilitar checagem de lock de arquivos
	// --nologo              -> Desativar tela de splash
	// --convert-to pdf      -> Formato de destino (PDF)
	// --outdir              -> Diretório de saída temporário do job
	args := []string{
		"-env:UserInstallation=" + userProfileURI,
		"--headless",
		"--norestore",
		"--nodefault",
		"--nofirststartwizard",
		"--nolockcheck",
		"--nologo",
		"--convert-to", "pdf",
		"--outdir", tmpDir,
		inputPath,
	}

	// SLA específico do conversor de documento: 90 segundos
	ctxTimeout, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctxTimeout, c.sofficePath, args...)
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout

	err = cmd.Run()

	if ctxTimeout.Err() != nil {
		if ctxTimeout.Err() == context.DeadlineExceeded || ctxTimeout.Err() == context.Canceled {
			return nil, domain.ErrJobTimeout
		}
	}

	outStr := strings.ToLower(stderr.String() + " " + stdout.String())
	if err != nil || strings.Contains(outStr, "password") || strings.Contains(outStr, "encrypted") {
		if strings.Contains(outStr, "password") || strings.Contains(outStr, "encrypted") || strings.Contains(outStr, "passphrase") {
			return nil, domain.ErrPasswordProtectedDocument
		}
		if err != nil {
			return nil, fmt.Errorf("%w: libreoffice falhou: %s (%v)", domain.ErrCorruptedFile, stderr.String(), err)
		}
	}

	// 6. Verificar a existência e leitura do PDF gerado
	outputBytes, err := os.ReadFile(outputPath)
	if err != nil || len(outputBytes) == 0 {
		return nil, domain.ErrEmptyDocumentPages
	}

	// 7. Validar limites de páginas no PDF gerado
	pageCount := countPDFPages(outputBytes)
	if pageCount == 0 {
		return nil, domain.ErrEmptyDocumentPages
	}

	if pageCount > MaxDocumentPagesAllowed {
		return nil, domain.ErrPageLimitExceeded
	}

	return bytes.NewReader(outputBytes), nil
}

// countPDFPages conta o número de objetos de página no PDF binário gerado.
func countPDFPages(pdfData []byte) int {
	pageRegex := regexp.MustCompile(`/Type\s*/Page[^s]`)
	matches := pageRegex.FindAll(pdfData, -1)
	if len(matches) > 0 {
		return len(matches)
	}
	// Fallback para PDFs minimalistas de 1 página
	if bytes.Contains(pdfData, []byte("%PDF-")) && len(pdfData) > 0 {
		return 1
	}
	return 0
}
