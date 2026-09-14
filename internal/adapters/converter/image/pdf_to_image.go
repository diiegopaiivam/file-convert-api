package image

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
)

var _ ports.Converter = (*PDFToImageConverter)(nil)

const MaxPDFPagesAllowed = 50

// PDFToImageConverter realiza a conversão de PDF (.pdf) para JPG (.jpg) utilizando pdftoppm do pacote poppler-utils.
type PDFToImageConverter struct {
	pdftoppmPath string
}

// NewPDFToImageConverter cria uma nova instância do adaptador PDFToImageConverter.
func NewPDFToImageConverter(pdftoppmPath string) *PDFToImageConverter {
	if pdftoppmPath == "" {
		pdftoppmPath = "pdftoppm"
	}
	return &PDFToImageConverter{
		pdftoppmPath: pdftoppmPath,
	}
}

// CanConvert verifica se o par (srcFormat, targetFormat) é suportado por este adaptador.
func (c *PDFToImageConverter) CanConvert(srcFormat, targetFormat domain.Format) bool {
	return srcFormat == domain.FormatPDF && targetFormat == domain.FormatJPG
}

// Convert realiza a conversão de PDF para imagem JPG em 300 DPI, empacotando em .zip se contiver múltiplas páginas.
func (c *PDFToImageConverter) Convert(ctx context.Context, src io.Reader, srcFormat, targetFormat domain.Format) (io.Reader, error) {
	if !c.CanConvert(srcFormat, targetFormat) {
		return nil, domain.ErrUnsupportedFormat
	}

	// Validar magic bytes de PDF (%PDF-)
	validatedStream, err := domain.ValidateMagicBytes(src, domain.FormatPDF)
	if err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "pdf-to-image-*")
	if err != nil {
		return nil, fmt.Errorf("%w: falha ao criar diretório temporário: %v", domain.ErrInternalError, err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input.pdf")
	outputPrefix := filepath.Join(tmpDir, "page")

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

	// Executar pdftoppm renderizando páginas JPEG em 300 DPI
	args := []string{"-jpeg", "-r", "300", inputPath, outputPrefix}
	cmd := exec.CommandContext(ctx, c.pdftoppmPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err = cmd.Run()
	if ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled {
			return nil, domain.ErrJobTimeout
		}
	}

	outStr := strings.ToLower(stderr.String())
	if err != nil || strings.Contains(outStr, "error") || strings.Contains(outStr, "password") {
		if strings.Contains(outStr, "password") || strings.Contains(outStr, "encrypted") {
			return nil, domain.ErrPasswordProtectedPDF
		}
		if err != nil {
			return nil, fmt.Errorf("%w: pdftoppm falhou: %s (%v)", domain.ErrCorruptedFile, stderr.String(), err)
		}
	}

	// Localizar todas as páginas de imagem geradas em tmpDir (ex: page-1.jpg, page-01.jpg, page-2.jpg)
	pageFiles, err := filepath.Glob(filepath.Join(tmpDir, "page-*.jpg"))
	if err != nil {
		return nil, fmt.Errorf("%w: falha ao listar páginas geradas: %v", domain.ErrInternalError, err)
	}

	if len(pageFiles) == 0 {
		return nil, domain.ErrEmptyPDFPages
	}

	if len(pageFiles) > MaxPDFPagesAllowed {
		return nil, domain.ErrPageLimitExceeded
	}

	// Ordenar arquivos por número de página extraído do nome
	sortPageFiles(pageFiles)

	// Caso 1: PDF de 1 única página -> Retorna imagem JPG diretamente
	if len(pageFiles) == 1 {
		imgBytes, err := os.ReadFile(pageFiles[0])
		if err != nil {
			return nil, fmt.Errorf("%w: falha ao ler página renderizada: %v", domain.ErrInternalError, err)
		}
		return bytes.NewReader(imgBytes), nil
	}

	// Caso 2: PDF multipáginas (2 a 50) -> Compacta todas as imagens em um único arquivo .zip
	zipBuf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(zipBuf)

	for i, pageFile := range pageFiles {
		imgBytes, err := os.ReadFile(pageFile)
		if err != nil {
			return nil, fmt.Errorf("%w: falha ao ler página %d para o zip: %v", domain.ErrInternalError, i+1, err)
		}

		fileNameInZip := fmt.Sprintf("page-%d.jpg", i+1)
		fileWriter, err := zipWriter.Create(fileNameInZip)
		if err != nil {
			return nil, fmt.Errorf("%w: falha ao criar entrada no arquivo zip: %v", domain.ErrInternalError, err)
		}

		if _, err := fileWriter.Write(imgBytes); err != nil {
			return nil, fmt.Errorf("%w: falha ao escrever imagem no arquivo zip: %v", domain.ErrInternalError, err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("%w: falha ao fechar arquivo zip: %v", domain.ErrInternalError, err)
	}

	return bytes.NewReader(zipBuf.Bytes()), nil
}

func sortPageFiles(files []string) {
	sort.Slice(files, func(i, j int) bool {
		numI := extractPageNumber(files[i])
		numJ := extractPageNumber(files[j])
		return numI < numJ
	})
}

func extractPageNumber(path string) int {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	parts := strings.Split(name, "-")
	if len(parts) >= 2 {
		numStr := parts[len(parts)-1]
		if num, err := strconv.Atoi(numStr); err == nil {
			return num
		}
	}
	return 0
}
