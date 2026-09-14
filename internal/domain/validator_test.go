package domain_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/file-convert-api/internal/domain"
)

func TestIsValidConversionPair(t *testing.T) {
	validPairs := []struct {
		src    domain.Format
		target domain.Format
	}{
		{domain.FormatWAV, domain.FormatMP3},
		{domain.FormatMP3, domain.FormatWAV},
		{domain.FormatJPG, domain.FormatPDF},
		{domain.FormatPNG, domain.FormatPDF},
		{domain.FormatPDF, domain.FormatJPG},
		{domain.FormatDOC, domain.FormatPDF},
		{domain.FormatDOCX, domain.FormatPDF},
	}

	for _, pair := range validPairs {
		if !domain.IsValidConversionPair(pair.src, pair.target) {
			t.Errorf("Esperado par válido para (%s -> %s), mas retornou falso", pair.src, pair.target)
		}
	}

	invalidPairs := []struct {
		src    domain.Format
		target domain.Format
	}{
		{domain.FormatWAV, domain.FormatPDF},
		{domain.FormatDOC, domain.FormatMP3},
		{domain.FormatPDF, domain.FormatDOC},
		{domain.FormatJPG, domain.FormatWAV},
		{domain.Format("exe"), domain.FormatPDF},
		{domain.Format("txt"), domain.FormatPDF},
	}

	for _, pair := range invalidPairs {
		if domain.IsValidConversionPair(pair.src, pair.target) {
			t.Errorf("Esperado par inválido para (%s -> %s), mas retornou verdadeiro", pair.src, pair.target)
		}
	}
}

func TestValidateMagicBytes_ValidHeaders(t *testing.T) {
	tests := []struct {
		name   string
		format domain.Format
		header []byte
	}{
		{
			name:   "WAV valid header",
			format: domain.FormatWAV,
			header: []byte("RIFF\x00\x00\x00\x00WAVEfmt "),
		},
		{
			name:   "MP3 ID3 tag valid header",
			format: domain.FormatMP3,
			header: []byte("ID3\x04\x00\x00\x00\x00\x00\x00"),
		},
		{
			name:   "MP3 frame sync header",
			format: domain.FormatMP3,
			header: []byte{0xFF, 0xFB, 0x90, 0x44},
		},
		{
			name:   "JPG valid header",
			format: domain.FormatJPG,
			header: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46},
		},
		{
			name:   "PNG valid header",
			format: domain.FormatPNG,
			header: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
		},
		{
			name:   "PDF valid header",
			format: domain.FormatPDF,
			header: []byte("%PDF-1.7\n%\xE2\xE3\xCF\xD3\n"),
		},
		{
			name:   "DOC OLE2 valid header",
			format: domain.FormatDOC,
			header: []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1},
		},
		{
			name:   "DOCX ZIP valid header",
			format: domain.FormatDOCX,
			header: []byte{0x50, 0x4B, 0x03, 0x04, 0x14, 0x00, 0x06, 0x00},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := append(tt.header, []byte("...payload_data...")...)
			reader, err := domain.ValidateMagicBytes(bytes.NewReader(payload), tt.format)
			if err != nil {
				t.Fatalf("Esperado sucesso na validação de magic bytes, obtido erro: %v", err)
			}

			readBytes, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("Falha ao ler stream reconstituído: %v", err)
			}

			if !bytes.Equal(readBytes, payload) {
				t.Errorf("Stream reconstituído difere do payload original")
			}
		})
	}
}

func TestValidateMagicBytes_InvalidHeaders(t *testing.T) {
	tests := []struct {
		name   string
		format domain.Format
		header []byte
	}{
		{
			name:   "Spoofed extension - text file claiming to be WAV",
			format: domain.FormatWAV,
			header: []byte("PLAIN TEXT CONTENT INSTEAD OF RIFF WAVE"),
		},
		{
			name:   "Spoofed extension - executable claiming to be PDF",
			format: domain.FormatPDF,
			header: []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00"),
		},
		{
			name:   "Corrupted JPEG header",
			format: domain.FormatJPG,
			header: []byte{0x00, 0x00, 0x00, 0x00},
		},
		{
			name:   "Invalid PNG header",
			format: domain.FormatPNG,
			header: []byte("NOT_A_PNG_FILE_HEADER"),
		},
		{
			name:   "Invalid DOCX header",
			format: domain.FormatDOCX,
			header: []byte("INVALID_DOCX_HEADER"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.ValidateMagicBytes(bytes.NewReader(tt.header), tt.format)
			if err == nil {
				t.Fatalf("Esperado erro para arquivo com magic bytes inválidos, mas a validação passou")
			}
		})
	}
}

func TestValidateMagicBytes_EmptyFile(t *testing.T) {
	_, err := domain.ValidateMagicBytes(bytes.NewReader([]byte{}), domain.FormatWAV)
	if err == nil {
		t.Fatalf("Esperado erro ao validar arquivo vazio")
	}
}
