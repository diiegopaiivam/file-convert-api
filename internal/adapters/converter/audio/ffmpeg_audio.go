package audio

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

var _ ports.Converter = (*FFmpegAudioConverter)(nil)

// FFmpegAudioConverter realiza a conversão de áudio (.wav ↔ .mp3) utilizando o binário ffmpeg.
type FFmpegAudioConverter struct {
	ffmpegPath string
}

// NewFFmpegAudioConverter cria uma nova instância do adaptador FFmpegAudioConverter.
func NewFFmpegAudioConverter(ffmpegPath string) *FFmpegAudioConverter {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	return &FFmpegAudioConverter{
		ffmpegPath: ffmpegPath,
	}
}

// CanConvert verifica se o par (srcFormat, targetFormat) é suportado por este adaptador.
func (c *FFmpegAudioConverter) CanConvert(srcFormat, targetFormat domain.Format) bool {
	validWavToMp3 := (srcFormat == domain.FormatWAV && targetFormat == domain.FormatMP3)
	validMp3ToWav := (srcFormat == domain.FormatMP3 && targetFormat == domain.FormatWAV)
	return validWavToMp3 || validMp3ToWav
}

// Convert realiza a conversão assíncrona/stream executando o subprocesso ffmpeg CLI.
func (c *FFmpegAudioConverter) Convert(ctx context.Context, src io.Reader, srcFormat, targetFormat domain.Format) (io.Reader, error) {
	if !c.CanConvert(srcFormat, targetFormat) {
		return nil, domain.ErrUnsupportedFormat
	}

	// Criar diretório temporário para isolar a conversão
	tmpDir, err := os.MkdirTemp("", "audio-convert-*")
	if err != nil {
		return nil, fmt.Errorf("%w: falha ao criar diretório temporário: %v", domain.ErrInternalError, err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, fmt.Sprintf("input.%s", srcFormat))
	outputPath := filepath.Join(tmpDir, fmt.Sprintf("output.%s", targetFormat))

	// Escrever arquivo de entrada
	inputFile, err := os.Create(inputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: falha ao criar arquivo temporário de entrada: %v", domain.ErrInternalError, err)
	}

	n, err := io.Copy(inputFile, src)
	_ = inputFile.Close()
	if err != nil {
		return nil, fmt.Errorf("%w: falha ao copiar stream de entrada: %v", domain.ErrInternalError, err)
	}

	if n == 0 {
		return nil, domain.ErrEmptyFile
	}

	// Definir argumentos do FFmpeg com base no formato de destino
	var args []string
	if srcFormat == domain.FormatWAV && targetFormat == domain.FormatMP3 {
		args = []string{"-y", "-i", inputPath, "-codec:a", "libmp3lame", "-qscale:a", "2", outputPath}
	} else if srcFormat == domain.FormatMP3 && targetFormat == domain.FormatWAV {
		args = []string{"-y", "-i", inputPath, "-codec:a", "pcm_s16le", outputPath}
	}

	cmd := exec.CommandContext(ctx, c.ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err = cmd.Run()
	if ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled {
			return nil, domain.ErrJobTimeout
		}
	}

	if err != nil {
		return nil, fmt.Errorf("%w: ffmpeg falhou com erro: %s (%v)", domain.ErrCorruptedFile, stderr.String(), err)
	}

	outputBytes, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: falha ao ler arquivo de saída convertido: %v", domain.ErrInternalError, err)
	}

	return bytes.NewReader(outputBytes), nil
}
