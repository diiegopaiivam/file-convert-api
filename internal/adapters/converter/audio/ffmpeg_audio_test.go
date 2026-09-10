package audio_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/file-convert-api/internal/adapters/converter/audio"
	"github.com/file-convert-api/internal/domain"
)

func TestFFmpegAudioConverter_CanConvert(t *testing.T) {
	converter := audio.NewFFmpegAudioConverter("")

	tests := []struct {
		name         string
		src          domain.Format
		target       domain.Format
		expectedBool bool
	}{
		{
			name:         "WAV para MP3 é suportado",
			src:          domain.FormatWAV,
			target:       domain.FormatMP3,
			expectedBool: true,
		},
		{
			name:         "MP3 para WAV é suportado",
			src:          domain.FormatMP3,
			target:       domain.FormatWAV,
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

func TestFFmpegAudioConverter_Convert_UnsupportedFormat(t *testing.T) {
	converter := audio.NewFFmpegAudioConverter("")
	ctx := context.Background()
	src := bytes.NewReader([]byte("conteudo"))

	_, err := converter.Convert(ctx, src, domain.FormatJPG, domain.FormatPDF)
	if !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Fatalf("Esperado erro ErrUnsupportedFormat, obtido: %v", err)
	}
}

func TestFFmpegAudioConverter_Convert_EmptyFile(t *testing.T) {
	converter := audio.NewFFmpegAudioConverter("")
	ctx := context.Background()
	src := bytes.NewReader([]byte{})

	_, err := converter.Convert(ctx, src, domain.FormatWAV, domain.FormatMP3)
	if !errors.Is(err, domain.ErrEmptyFile) {
		t.Fatalf("Esperado erro ErrEmptyFile, obtido: %v", err)
	}
}

func TestFFmpegAudioConverter_Convert_ContextTimeout(t *testing.T) {
	converter := audio.NewFFmpegAudioConverter("sleep") // simula comando demorado caso executado
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Microsecond)
	defer cancel()

	time.Sleep(2 * time.Millisecond)

	src := bytes.NewReader([]byte("dummy content for timeout test"))
	_, err := converter.Convert(ctx, src, domain.FormatWAV, domain.FormatMP3)
	if err == nil {
		t.Fatalf("Esperado erro de timeout ou contexto cancelado, obtido nil")
	}
}
