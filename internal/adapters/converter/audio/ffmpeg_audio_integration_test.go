package audio_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/file-convert-api/internal/adapters/converter/audio"
	"github.com/file-convert-api/internal/domain"
)

// generateValidWAVHeader gera um arquivo WAV PCM de 1 segundo de silêncio sintético válido para testes de integração.
func generateValidWAVHeader() []byte {
	sampleRate := uint32(44100)
	numChannels := uint16(1)
	bitsPerSample := uint16(16)
	numSamples := sampleRate // 1 segundo
	dataSize := uint32(numSamples * uint32(numChannels) * uint32(bitsPerSample/8))
	fileSize := 36 + dataSize

	buf := new(bytes.Buffer)
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, fileSize)
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16)) // Subchunk1Size
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))  // AudioFormat (1 = PCM)
	_ = binary.Write(buf, binary.LittleEndian, numChannels)
	_ = binary.Write(buf, binary.LittleEndian, sampleRate)
	byteRate := sampleRate * uint32(numChannels) * uint32(bitsPerSample/8)
	_ = binary.Write(buf, binary.LittleEndian, byteRate)
	blockAlign := numChannels * (bitsPerSample / 8)
	_ = binary.Write(buf, binary.LittleEndian, blockAlign)
	_ = binary.Write(buf, binary.LittleEndian, bitsPerSample)
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, dataSize)

	// Escrever samples de silêncio (zeros)
	buf.Write(make([]byte, dataSize))

	return buf.Bytes()
}

// isValidMP3Header verifica se o buffer contém cabeçalho de MP3 válido (ID3 tag ou sync word 0xFFE0 / 0xFFFB).
func isValidMP3Header(data []byte) bool {
	if len(data) < 3 {
		return false
	}
	// Tag ID3v2
	if data[0] == 'I' && data[1] == 'D' && data[2] == '3' {
		return true
	}
	// Frame Sync Word (0xFF seguido de 0xFB, 0xF3, 0xF2, etc)
	if len(data) >= 2 && data[0] == 0xFF && (data[1]&0xE0) == 0xE0 {
		return true
	}
	return false
}

// isValidWAVHeader verifica se o buffer contém o cabeçalho 'RIFF' + 'WAVE'.
func isValidWAVHeader(data []byte) bool {
	if len(data) < 12 {
		return false
	}
	return string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE"
}

func TestIntegration_FFmpegAudioConverter_WAVToMP3_ValidHeader(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("Pulando teste de integração real: binário ffmpeg não instalado no host de execução do teste.")
	}

	converter := audio.NewFFmpegAudioConverter("")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wavBytes := generateValidWAVHeader()
	resultReader, err := converter.Convert(ctx, bytes.NewReader(wavBytes), domain.FormatWAV, domain.FormatMP3)
	if err != nil {
		t.Fatalf("Falha na conversão de WAV válido para MP3: %v", err)
	}

	outBytes, err := io.ReadAll(resultReader)
	if err != nil {
		t.Fatalf("Falha ao ler o stream de resultado: %v", err)
	}

	if !isValidMP3Header(outBytes) {
		t.Errorf("O arquivo de saída gerado pelo FFmpeg NÃO possui um cabeçalho MP3 (ID3/Sync) válido. Bytes recebidos (primeiros 10): %x", outBytes[:min(10, len(outBytes))])
	}
}

func TestIntegration_FFmpegAudioConverter_CorruptedFile(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("Pulando teste de integração real: binário ffmpeg não instalado no host.")
	}

	converter := audio.NewFFmpegAudioConverter("")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	corruptedData := []byte("ESTE_NAO_E_UM_ARQUIVO_DE_AUDIO_VALIDO_HEADER_CORROMPIDO")
	_, err := converter.Convert(ctx, bytes.NewReader(corruptedData), domain.FormatWAV, domain.FormatMP3)

	if err == nil {
		t.Fatalf("Esperada falha na conversão de arquivo corrompido, mas obteve sucesso.")
	}

	if !errors.Is(err, domain.ErrCorruptedFile) {
		t.Errorf("Esperado erro domain.ErrCorruptedFile, obtido: %v", err)
	}
}

func TestIntegration_FFmpegAudioConverter_EmptyFile(t *testing.T) {
	converter := audio.NewFFmpegAudioConverter("")
	ctx := context.Background()

	emptyData := []byte{}
	_, err := converter.Convert(ctx, bytes.NewReader(emptyData), domain.FormatWAV, domain.FormatMP3)

	if err == nil || !errors.Is(err, domain.ErrEmptyFile) {
		t.Fatalf("Esperado erro domain.ErrEmptyFile, obtido: %v", err)
	}
}

func TestIntegration_FFmpegAudioConverter_UnsupportedFormat(t *testing.T) {
	converter := audio.NewFFmpegAudioConverter("")
	ctx := context.Background()

	_, err := converter.Convert(ctx, bytes.NewReader([]byte("data")), domain.FormatJPG, domain.FormatPDF)

	if err == nil || !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Fatalf("Esperado erro domain.ErrUnsupportedFormat, obtido: %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
