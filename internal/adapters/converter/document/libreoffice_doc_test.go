package document_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/file-convert-api/internal/adapters/converter/document"
	"github.com/file-convert-api/internal/domain"
)

func TestLibreOfficeDocConverter_CanConvert(t *testing.T) {
	converter := document.NewLibreOfficeDocConverter("")

	validPairs := []struct {
		src    domain.Format
		target domain.Format
	}{
		{domain.FormatDOC, domain.FormatPDF},
		{domain.FormatDOCX, domain.FormatPDF},
	}

	for _, pair := range validPairs {
		if !converter.CanConvert(pair.src, pair.target) {
			t.Errorf("Esperado CanConvert(%s, %s) == true, obtido false", pair.src, pair.target)
		}
	}

	invalidPairs := []struct {
		src    domain.Format
		target domain.Format
	}{
		{domain.FormatWAV, domain.FormatMP3},
		{domain.FormatJPG, domain.FormatPDF},
		{domain.FormatPDF, domain.FormatDOCX},
	}

	for _, pair := range invalidPairs {
		if converter.CanConvert(pair.src, pair.target) {
			t.Errorf("Esperado CanConvert(%s, %s) == false, obtido true", pair.src, pair.target)
		}
	}
}

func TestLibreOfficeDocConverter_Convert_EmptyFile(t *testing.T) {
	converter := document.NewLibreOfficeDocConverter("")
	emptyStream := bytes.NewReader([]byte{})

	_, err := converter.Convert(context.Background(), emptyStream, domain.FormatDOCX, domain.FormatPDF)
	if !errors.Is(err, domain.ErrEmptyFile) {
		t.Errorf("Esperado erro ErrEmptyFile para stream vazio, obtido: %v", err)
	}
}

func TestLibreOfficeDocConverter_Convert_UnsupportedFormat(t *testing.T) {
	converter := document.NewLibreOfficeDocConverter("")
	dummyStream := bytes.NewReader([]byte("dummy content"))

	_, err := converter.Convert(context.Background(), dummyStream, domain.FormatJPG, domain.FormatPDF)
	if !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Errorf("Esperado erro ErrUnsupportedFormat para formatos não suportados, obtido: %v", err)
	}
}

func TestLibreOfficeDocConverter_Convert_InvalidMagicBytes(t *testing.T) {
	converter := document.NewLibreOfficeDocConverter("")
	// Header inválido para DOCX (não é PK header)
	invalidStream := bytes.NewReader([]byte("HEADER_INVALIDO_DOCX_TESTE"))

	_, err := converter.Convert(context.Background(), invalidStream, domain.FormatDOCX, domain.FormatPDF)
	if !errors.Is(err, domain.ErrCorruptedFile) {
		t.Errorf("Esperado erro ErrCorruptedFile para magic bytes inválidos, obtido: %v", err)
	}
}

func TestLibreOfficeDocConverter_Convert_ContextTimeout(t *testing.T) {
	converter := document.NewLibreOfficeDocConverter("")
	
	// DOCX PK Header válido
	validDocxHeader := []byte{0x50, 0x4B, 0x03, 0x04, 0x14, 0x00, 0x06, 0x00}
	validStream := bytes.NewReader(append(validDocxHeader, []byte("...docx_content...")...))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancela o contexto imediatamente

	_, err := converter.Convert(ctx, validStream, domain.FormatDOCX, domain.FormatPDF)
	if !errors.Is(err, domain.ErrJobTimeout) && !errors.Is(err, context.Canceled) {
		t.Errorf("Esperado erro de timeout/cancelamento, obtido: %v", err)
	}
}

func TestLibreOfficeDocConverter_Convert_TimeoutCancelledBeforeExec(t *testing.T) {
	converter := document.NewLibreOfficeDocConverter("")
	
	// DOC Header válido (OLE2)
	oleMagic := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	validStream := bytes.NewReader(append(oleMagic, []byte("...doc_content...")...))

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	time.Sleep(2 * time.Millisecond)
	defer cancel()

	_, err := converter.Convert(ctx, validStream, domain.FormatDOC, domain.FormatPDF)
	if err == nil {
		t.Errorf("Esperado erro de timeout para contexto expirado, obtido nil")
	}
}
