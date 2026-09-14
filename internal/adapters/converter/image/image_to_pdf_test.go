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

func TestImageToPDFConverter_CanConvert(t *testing.T) {
	converter := image.NewImageToPDFConverter("")

	tests := []struct {
		name         string
		src          domain.Format
		target       domain.Format
		expectedBool bool
	}{
		{
			name:         "JPG para PDF é suportado",
			src:          domain.FormatJPG,
			target:       domain.FormatPDF,
			expectedBool: true,
		},
		{
			name:         "PNG para PDF é suportado",
			src:          domain.FormatPNG,
			target:       domain.FormatPDF,
			expectedBool: true,
		},
		{
			name:         "PDF para JPG não é suportado por este adaptador",
			src:          domain.FormatPDF,
			target:       domain.FormatJPG,
			expectedBool: false,
		},
		{
			name:         "WAV para MP3 não é suportado por este adaptador",
			src:          domain.FormatWAV,
			target:       domain.FormatMP3,
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

func TestImageToPDFConverter_Convert_UnsupportedFormat(t *testing.T) {
	converter := image.NewImageToPDFConverter("")
	ctx := context.Background()
	src := bytes.NewReader([]byte("conteudo"))

	_, err := converter.Convert(ctx, src, domain.FormatPDF, domain.FormatJPG)
	if !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Fatalf("Esperado erro ErrUnsupportedFormat, obtido: %v", err)
	}
}

func TestImageToPDFConverter_Convert_EmptyFile(t *testing.T) {
	converter := image.NewImageToPDFConverter("")
	ctx := context.Background()
	src := bytes.NewReader([]byte{})

	_, err := converter.Convert(ctx, src, domain.FormatJPG, domain.FormatPDF)
	if !errors.Is(err, domain.ErrEmptyFile) {
		t.Fatalf("Esperado erro ErrEmptyFile, obtido: %v", err)
	}
}

func TestImageToPDFConverter_Convert_InvalidMagicBytes(t *testing.T) {
	converter := image.NewImageToPDFConverter("")
	ctx := context.Background()
	src := bytes.NewReader([]byte("texto puro que nao e JPG nem PNG"))

	_, err := converter.Convert(ctx, src, domain.FormatJPG, domain.FormatPDF)
	if !errors.Is(err, domain.ErrCorruptedFile) {
		t.Fatalf("Esperado erro ErrCorruptedFile, obtido: %v", err)
	}
}

func TestImageToPDFConverter_Convert_ContextTimeout(t *testing.T) {
	converter := image.NewImageToPDFConverter("sleep")
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Microsecond)
	defer cancel()

	time.Sleep(2 * time.Millisecond)

	// Injetar SOI marker do JPG para passar no magic bytes check
	jpgMagic := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	src := bytes.NewReader(jpgMagic)

	_, err := converter.Convert(ctx, src, domain.FormatJPG, domain.FormatPDF)
	if err == nil {
		t.Fatalf("Esperado erro de timeout ou contexto cancelado, obtido nil")
	}
}
