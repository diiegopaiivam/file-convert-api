package image

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
)

var _ ports.Converter = (*ImageToPDFConverter)(nil)

// ImageToPDFConverter realiza a conversão de imagem (.jpg, .png) para PDF (.pdf) utilizando o ImageMagick.
type ImageToPDFConverter struct {
	convertPath string
}

// NewImageToPDFConverter cria uma nova instância do adaptador ImageToPDFConverter.
func NewImageToPDFConverter(convertPath string) *ImageToPDFConverter {
	if convertPath == "" {
		convertPath = "convert"
	}
	return &ImageToPDFConverter{
		convertPath: convertPath,
	}
}

// CanConvert verifica se o par (srcFormat, targetFormat) é suportado por este adaptador.
func (c *ImageToPDFConverter) CanConvert(srcFormat, targetFormat domain.Format) bool {
	isValidSrc := (srcFormat == domain.FormatJPG || srcFormat == domain.FormatPNG)
	return isValidSrc && targetFormat == domain.FormatPDF
}

// Convert executa o subprocesso ImageMagick convert para transformar a imagem de entrada em PDF.
func (c *ImageToPDFConverter) Convert(ctx context.Context, src io.Reader, srcFormat, targetFormat domain.Format) (io.Reader, error) {
	if !c.CanConvert(srcFormat, targetFormat) {
		return nil, domain.ErrUnsupportedFormat
	}

	// Validar magic bytes do formato de entrada (.jpg ou .png)
	validatedStream, err := domain.ValidateMagicBytes(src, srcFormat)
	if err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "image-convert-*")
	if err != nil {
		return nil, fmt.Errorf("%w: falha ao criar diretório temporário: %v", domain.ErrInternalError, err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, fmt.Sprintf("input.%s", srcFormat))
	outputPath := filepath.Join(tmpDir, "output.pdf")

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

	// Executar binário ImageMagick com limites de recurso de memória
	args := []string{"-limit", "memory", "256MB", "-limit", "map", "512MB", inputPath, outputPath}
	cmd := exec.CommandContext(ctx, c.convertPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err = cmd.Run()
	if ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled {
			return nil, domain.ErrJobTimeout
		}
	}

	if err != nil {
		return nil, fmt.Errorf("%w: imagemagick falhou com erro: %s (%v)", domain.ErrCorruptedFile, stderr.String(), err)
	}

	outputBytes, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: falha ao ler arquivo PDF convertido: %v", domain.ErrInternalError, err)
	}

	return bytes.NewReader(outputBytes), nil
}
