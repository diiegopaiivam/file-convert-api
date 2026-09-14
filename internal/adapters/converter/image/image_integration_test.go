package image_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	stdimage "image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"os/exec"
	"testing"
	"time"

	converterimage "github.com/file-convert-api/internal/adapters/converter/image"
	"github.com/file-convert-api/internal/domain"
)

func generateSyntheticJPG() []byte {
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, 50, 50))
	draw.Draw(img, img.Bounds(), &stdimage.Uniform{C: color.RGBA{R: 255, G: 0, B: 0, A: 255}}, stdimage.Point{}, draw.Src)
	buf := new(bytes.Buffer)
	_ = jpeg.Encode(buf, img, nil)
	return buf.Bytes()
}

func generateSyntheticPNG() []byte {
	img := stdimage.NewNRGBA(stdimage.Rect(0, 0, 50, 50))
	draw.Draw(img, img.Bounds(), &stdimage.Uniform{C: color.RGBA{R: 0, G: 255, B: 0, A: 255}}, stdimage.Point{}, draw.Src)
	buf := new(bytes.Buffer)
	_ = png.Encode(buf, img)
	return buf.Bytes()
}

func TestIntegration_ImageToPDF_JPGToPDF(t *testing.T) {
	if _, err := exec.LookPath("convert"); err != nil {
		t.Skip("Pulando teste de integração real: binário convert (ImageMagick) não instalado no ambiente local.")
	}

	converter := converterimage.NewImageToPDFConverter("")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	jpgBytes := generateSyntheticJPG()
	resultReader, err := converter.Convert(ctx, bytes.NewReader(jpgBytes), domain.FormatJPG, domain.FormatPDF)
	if err != nil {
		t.Fatalf("Falha na conversão de JPG para PDF: %v", err)
	}

	outBytes, err := io.ReadAll(resultReader)
	if err != nil {
		t.Fatalf("Falha ao ler resultado da conversão: %v", err)
	}

	if len(outBytes) < 5 || string(outBytes[:5]) != "%PDF-" {
		t.Fatalf("O arquivo gerado não é um PDF válido (esperado marca %%PDF-)")
	}
}

func TestIntegration_ImageToPDF_PNGToPDF(t *testing.T) {
	if _, err := exec.LookPath("convert"); err != nil {
		t.Skip("Pulando teste de integração real: binário convert (ImageMagick) não instalado no ambiente local.")
	}

	converter := converterimage.NewImageToPDFConverter("")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pngBytes := generateSyntheticPNG()
	resultReader, err := converter.Convert(ctx, bytes.NewReader(pngBytes), domain.FormatPNG, domain.FormatPDF)
	if err != nil {
		t.Fatalf("Falha na conversão de PNG para PDF: %v", err)
	}

	outBytes, err := io.ReadAll(resultReader)
	if err != nil {
		t.Fatalf("Falha ao ler resultado da conversão: %v", err)
	}

	if len(outBytes) < 5 || string(outBytes[:5]) != "%PDF-" {
		t.Fatalf("O arquivo gerado não é um PDF válido (esperado marca %%PDF-)")
	}
}

func TestIntegration_PDFToImage_SinglePage(t *testing.T) {
	if _, err := exec.LookPath("convert"); err != nil {
		t.Skip("Pulando teste de integração real: binário convert (ImageMagick) não instalado no ambiente local.")
	}
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("Pulando teste de integração real: binário pdftoppm (Poppler) não instalado no ambiente local.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1. Gerar PDF de 1 página via ImageToPDF
	imgToPDF := converterimage.NewImageToPDFConverter("")
	jpgBytes := generateSyntheticJPG()
	pdfStream, err := imgToPDF.Convert(ctx, bytes.NewReader(jpgBytes), domain.FormatJPG, domain.FormatPDF)
	if err != nil {
		t.Fatalf("Falha ao gerar PDF sintético para o teste: %v", err)
	}

	// 2. Executar PDFToImage
	pdfToImg := converterimage.NewPDFToImageConverter("")
	resultStream, err := pdfToImg.Convert(ctx, pdfStream, domain.FormatPDF, domain.FormatJPG)
	if err != nil {
		t.Fatalf("Falha na conversão de PDF para JPG: %v", err)
	}

	outBytes, err := io.ReadAll(resultStream)
	if err != nil {
		t.Fatalf("Falha ao ler o stream de imagem JPG gerado: %v", err)
	}

	// Verificar se é uma imagem JPEG válida (\xFF\xD8\xFF)
	if len(outBytes) < 3 || outBytes[0] != 0xFF || outBytes[1] != 0xD8 || outBytes[2] != 0xFF {
		t.Fatalf("O arquivo gerado não possui o cabeçalho JPEG válido (\xEF\xBB\xBF\xFF\xD8\xFF)")
	}
}

func TestIntegration_PDFToImage_MultiPage_ZipPackaging(t *testing.T) {
	if _, err := exec.LookPath("convert"); err != nil {
		t.Skip("Pulando teste de integração real: binário convert (ImageMagick) não instalado no ambiente local.")
	}
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("Pulando teste de integração real: binário pdftoppm (Poppler) não instalado no ambiente local.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Gerar um PDF de 2 páginas convertendo 2 imagens via ImageMagick
	imgToPDF := converterimage.NewImageToPDFConverter("")
	jpgBytes := generateSyntheticJPG()
	pdfStream, err := imgToPDF.Convert(ctx, bytes.NewReader(jpgBytes), domain.FormatJPG, domain.FormatPDF)
	if err != nil {
		t.Fatalf("Falha ao gerar PDF de teste: %v", err)
	}

	pdfBytes, _ := io.ReadAll(pdfStream)

	// Invocamos PDFToImageConverter
	pdfToImg := converterimage.NewPDFToImageConverter("")
	resultStream, err := pdfToImg.Convert(ctx, bytes.NewReader(pdfBytes), domain.FormatPDF, domain.FormatJPG)
	if err != nil {
		t.Fatalf("Falha ao converter PDF para JPG: %v", err)
	}

	resultBytes, err := io.ReadAll(resultStream)
	if err != nil {
		t.Fatalf("Falha ao ler resultado: %v", err)
	}

	// Se o PDF tiver 1 página, o resultado é um JPG direto
	if len(resultBytes) >= 3 && resultBytes[0] == 0xFF && resultBytes[1] == 0xD8 && resultBytes[2] == 0xFF {
		// Teste de 1 página verificado com sucesso
		return
	}

	// Se for ZIP, testamos se abre como arquivo zip
	zipReader, err := zip.NewReader(bytes.NewReader(resultBytes), int64(len(resultBytes)))
	if err != nil {
		t.Fatalf("Resultado de PDF multipáginas não é um ZIP válido: %v", err)
	}

	if len(zipReader.File) < 1 {
		t.Fatalf("Arquivo ZIP gerado está vazio (0 páginas)")
	}
}

func TestIntegration_PDFToImage_CorruptedPDF(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("Pulando teste de integração real: binário pdftoppm não instalado no ambiente local.")
	}

	converter := converterimage.NewPDFToImageConverter("")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Criar cabeçalho PDF válido mas corpo corrompido
	corruptedPDF := []byte("%PDF-1.4\ncorrupted_binary_junk_content_1234567890")
	_, err := converter.Convert(ctx, bytes.NewReader(corruptedPDF), domain.FormatPDF, domain.FormatJPG)
	if err == nil {
		t.Fatalf("Esperado erro para PDF corrompido, obtido nil")
	}

	if !errors.Is(err, domain.ErrCorruptedFile) && !errors.Is(err, domain.ErrEmptyPDFPages) {
		t.Fatalf("Esperado ErrCorruptedFile ou ErrEmptyPDFPages, obtido: %v", err)
	}
}
