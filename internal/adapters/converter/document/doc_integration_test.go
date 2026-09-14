package document_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/file-convert-api/internal/adapters/converter/document"
	"github.com/file-convert-api/internal/domain"
)

func TestIntegration_DOCXToPDF(t *testing.T) {
	sofficeBin := findLibreOfficeBinary()
	if sofficeBin == "" {
		t.Skip("Binário LibreOffice/soffice não encontrado no PATH. Pulando teste de integração real.")
	}

	docxBytes, err := createSyntheticDOCXBytes(false)
	if err != nil {
		t.Fatalf("Falha ao criar arquivo sintético DOCX: %v", err)
	}

	converter := document.NewLibreOfficeDocConverter(sofficeBin)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resultStream, err := converter.Convert(ctx, bytes.NewReader(docxBytes), domain.FormatDOCX, domain.FormatPDF)
	if err != nil {
		t.Fatalf("Falha na conversão de DOCX para PDF: %v", err)
	}

	pdfBytes, err := io.ReadAll(resultStream)
	if err != nil {
		t.Fatalf("Falha ao ler stream PDF retornado: %v", err)
	}

	if len(pdfBytes) < 5 || !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		t.Fatalf("O arquivo retornado não possui cabeçalho PDF válido (%%PDF-). Tamanho: %d bytes", len(pdfBytes))
	}
}

func TestIntegration_DOCXWithMacro_MacroIgnoredAndConvertedToPDF(t *testing.T) {
	sofficeBin := findLibreOfficeBinary()
	if sofficeBin == "" {
		t.Skip("Binário LibreOffice/soffice não encontrado no PATH. Pulando teste de integração de macro.")
	}

	// Criar arquivo DOCX contendo vbaProject.bin (macro VBA embutida)
	docxMacroBytes, err := createSyntheticDOCXBytes(true)
	if err != nil {
		t.Fatalf("Falha ao criar arquivo sintético DOCX com macro: %v", err)
	}

	converter := document.NewLibreOfficeDocConverter(sofficeBin)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// A conversão deve ocorrer com SUCESSO (status done), desativando/ignorando a execução da macro e gerando o PDF estático
	resultStream, err := converter.Convert(ctx, bytes.NewReader(docxMacroBytes), domain.FormatDOCX, domain.FormatPDF)
	if err != nil {
		t.Fatalf("Falha ao converter documento com macro para PDF. As macros devem ser ignoradas, não falhar a conversão: %v", err)
	}

	pdfBytes, err := io.ReadAll(resultStream)
	if err != nil {
		t.Fatalf("Falha ao ler o PDF resultante de documento com macro: %v", err)
	}

	if len(pdfBytes) < 5 || !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		t.Fatalf("O arquivo gerado a partir do documento com macro não possui cabeçalho PDF válido (%%PDF-)")
	}
}

func findLibreOfficeBinary() string {
	for _, bin := range []string{"soffice", "libreoffice"} {
		if path, err := exec.LookPath(bin); err == nil {
			return path
		}
	}
	return ""
}

// createSyntheticDOCXBytes gera um buffer binário representando uma estrutura ZIP válida de arquivo .docx.
// Se includeMacro=true, embute um stream de vbaProject.bin simulando macro VBA.
func createSyntheticDOCXBytes(includeMacro bool) ([]byte, error) {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	// Add [Content_Types].xml
	contentTypesXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`
	f1, err := zipWriter.Create("[Content_Types].xml")
	if err != nil {
		return nil, err
	}
	if _, err := f1.Write([]byte(contentTypesXML)); err != nil {
		return nil, err
	}

	// Add _rels/.rels
	relsXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`
	f2, err := zipWriter.Create("_rels/.rels")
	if err != nil {
		return nil, err
	}
	if _, err := f2.Write([]byte(relsXML)); err != nil {
		return nil, err
	}

	// Add word/document.xml
	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r>
        <w:t>Teste de Garantia de Qualidade (QA) - Documento DOCX com Conteudo Estatico</w:t>
      </w:r>
    </w:p>
  </w:body>
</w:document>`
	f3, err := zipWriter.Create("word/document.xml")
	if err != nil {
		return nil, err
	}
	if _, err := f3.Write([]byte(docXML)); err != nil {
		return nil, err
	}

	if includeMacro {
		// Embutir vbaProject.bin fictício para simular documento com macros VBA (ex: .docm)
		fMacro, err := zipWriter.Create("word/vbaProject.bin")
		if err != nil {
			return nil, err
		}
		macroPayload := []byte("OLE_MACRO_PAYLOAD_VBA_PROJECT_DUMMY_SCRIPT_DONT_EXECUTE")
		if _, err := fMacro.Write(macroPayload); err != nil {
			return nil, err
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
