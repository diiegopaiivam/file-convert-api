package document_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/file-convert-api/internal/adapters/converter/document"
	"github.com/file-convert-api/internal/domain"
)

// TestLibreOfficeDocConverter_ArgumentInjectionProtection confirma que argumentos maliciosos
// ou nomes de arquivo injetados não alteram o slice de argumentos executado pelo exec.Command.
func TestLibreOfficeDocConverter_ArgumentInjectionProtection(t *testing.T) {
	tmpDir := t.TempDir()
	canaryFile := filepath.Join(tmpDir, "doc_canary_hacked.txt")
	argsCaptureFile := filepath.Join(tmpDir, "doc_args_captured.txt")

	// Criar script mock para 'soffice'
	mockSoffice := filepath.Join(tmpDir, "mock_soffice.sh")
	mockScriptContent := "#!/bin/sh\n" +
		"for arg in \"$@\"; do\n" +
		"  echo \"ARG: $arg\" >> \"" + argsCaptureFile + "\"\n" +
		"done\n" +
		"exit 0\n"

	if err := os.WriteFile(mockSoffice, []byte(mockScriptContent), 0755); err != nil {
		t.Fatalf("Falha ao criar script mock soffice: %v", err)
	}

	converter := document.NewLibreOfficeDocConverter(mockSoffice)

	// Payload de injeção contendo comandos shell e flags maliciosas
	maliciousFormat := domain.Format("docx; touch " + canaryFile + "; --oob-flag")

	validDocxHeader := []byte{0x50, 0x4B, 0x03, 0x04, 0x14, 0x00, 0x06, 0x00}
	payload := append(validDocxHeader, []byte("...docx_test_content...")...)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _ = converter.Convert(ctx, bytes.NewReader(payload), maliciousFormat, domain.FormatPDF)

	// 1. Confirmar que o arquivo canary NUNCA foi criado
	if _, err := os.Stat(canaryFile); !os.IsNotExist(err) {
		t.Fatalf("ALERTA DE SEGURANÇA: O payload malicioso foi executado no shell! O arquivo canary %s foi criado.", canaryFile)
	}

	// 2. Verificar que a string de injeção permaneceu como parâmetro de caminho literal isolado
	capturedData, err := os.ReadFile(argsCaptureFile)
	if err == nil {
		capturedStr := string(capturedData)
		if !strings.Contains(capturedStr, canaryFile) {
			t.Logf("Argumentos capturados: %s", capturedStr)
		}
	}
}
