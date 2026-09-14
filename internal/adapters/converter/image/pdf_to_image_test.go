package image_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/file-convert-api/internal/adapters/converter/image"
	"github.com/file-convert-api/internal/domain"
)

func TestPDFToImageConverter_CanConvert(t *testing.T) {
	converter := image.NewPDFToImageConverter("")

	tests := []struct {
		name         string
		src          domain.Format
		target       domain.Format
		expectedBool bool
	}{
		{
			name:         "PDF para JPG é suportado",
			src:          domain.FormatPDF,
			target:       domain.FormatJPG,
			expectedBool: true,
		},
		{
			name:         "JPG para PDF não é suportado por este adaptador",
			src:          domain.FormatJPG,
			target:       domain.FormatPDF,
			expectedBool: false,
		},
		{
			name:         "DOCX para PDF não é suportado por este adaptador",
			src:          domain.FormatDOCX,
			target:       domain.FormatPDF,
			expectedBool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := converter.CanConvert(tt.src, tt.target)
			if res != tt.expectedBool {
				t.Errorf("CanConvert(%s, %s) = %v; esperado %v", tt.src, tt.target, res, tt.expectedBool)
			}
		})
	}
}

func TestPDFToImageConverter_Convert_UnsupportedFormat(t *testing.T) {
	converter := image.NewPDFToImageConverter("")
	ctx := context.Background()
	src := bytes.NewReader([]byte("%PDF-1.4"))

	_, err := converter.Convert(ctx, src, domain.FormatJPG, domain.FormatPDF)
	if !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Fatalf("Esperado erro ErrUnsupportedFormat, obtido: %v", err)
	}
}

func TestPDFToImageConverter_Convert_EmptyFile(t *testing.T) {
	converter := image.NewPDFToImageConverter("")
	ctx := context.Background()
	src := bytes.NewReader([]byte{})

	_, err := converter.Convert(ctx, src, domain.FormatPDF, domain.FormatJPG)
	if !errors.Is(err, domain.ErrEmptyFile) {
		t.Fatalf("Esperado erro ErrEmptyFile, obtido: %v", err)
	}
}

func TestPDFToImageConverter_Convert_InvalidMagicBytes(t *testing.T) {
	converter := image.NewPDFToImageConverter("")
	ctx := context.Background()
	src := bytes.NewReader([]byte("arquivo texto sem magic bytes de PDF"))

	_, err := converter.Convert(ctx, src, domain.FormatPDF, domain.FormatJPG)
	if !errors.Is(err, domain.ErrCorruptedFile) {
		t.Fatalf("Esperado erro ErrCorruptedFile, obtido: %v", err)
	}
}

func TestPDFToImageConverter_Convert_ContextTimeout(t *testing.T) {
	converter := image.NewPDFToImageConverter("sleep")
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Microsecond)
	defer cancel()

	time.Sleep(2 * time.Millisecond)

	// Injetar magic bytes de PDF %PDF-
	pdfMagic := []byte("%PDF-1.4 header content")
	src := bytes.NewReader(pdfMagic)

	_, err := converter.Convert(ctx, src, domain.FormatPDF, domain.FormatJPG)
	if err == nil {
		t.Fatalf("Esperado erro de timeout ou contexto cancelado, obtido nil")
	}
}
