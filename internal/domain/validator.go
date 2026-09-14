package domain

import (
	"bytes"
	"io"
)

// IsValidConversionPair verifica se o par (srcFormat, targetFormat) pertence à whitelist fechada da v1.
func IsValidConversionPair(srcFormat, targetFormat Format) bool {
	switch srcFormat {
	case FormatWAV:
		return targetFormat == FormatMP3
	case FormatMP3:
		return targetFormat == FormatWAV
	case FormatJPG, FormatPNG:
		return targetFormat == FormatPDF
	case FormatPDF:
		return targetFormat == FormatJPG
	case FormatDOC, FormatDOCX:
		return targetFormat == FormatPDF
	default:
		return false
	}
}

// ValidateMagicBytes inspeciona os primeiros bytes do arquivo para verificar a assinatura binária real.
// Retorna um novo io.Reader contendo o stream completo reconstituído ou erro ErrCorruptedFile se os magic bytes forem inválidos.
func ValidateMagicBytes(reader io.Reader, expectedFormat Format) (io.Reader, error) {
	header := make([]byte, 512)
	n, err := io.ReadFull(reader, header)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, ErrCorruptedFile
	}

	if n == 0 {
		return nil, ErrEmptyFile
	}

	headerBytes := header[:n]
	if !matchesMagicBytes(headerBytes, expectedFormat) {
		return nil, ErrCorruptedFile
	}

	// Reconstroi o reader combinando os bytes já lidos do cabeçalho com o restante do stream
	multiReader := io.MultiReader(bytes.NewReader(headerBytes), reader)
	return multiReader, nil
}

func matchesMagicBytes(b []byte, format Format) bool {
	switch format {
	case FormatWAV:
		// WAV deve começar com "RIFF" (bytes 0-3) e conter "WAVE" (bytes 8-11)
		if len(b) < 12 {
			return false
		}
		isRiff := bytes.Equal(b[0:4], []byte("RIFF"))
		isWave := bytes.Equal(b[8:12], []byte("WAVE"))
		return isRiff && isWave

	case FormatMP3:
		if len(b) < 3 {
			return false
		}
		// ID3 header tag
		if bytes.Equal(b[0:3], []byte("ID3")) {
			return true
		}
		// Frame sync bitmask 0xFFE0
		if len(b) >= 2 && b[0] == 0xFF && (b[1]&0xE0) == 0xE0 {
			return true
		}
		return false

	case FormatJPG:
		// JPEG SOI marker \xFF\xD8\xFF
		if len(b) < 3 {
			return false
		}
		return b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF

	case FormatPNG:
		// PNG Magic bytes \x89PNG\r\n\x1a\n
		if len(b) < 8 {
			return false
		}
		pngMagic := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		return bytes.Equal(b[0:8], pngMagic)

	case FormatPDF:
		// PDF Magic bytes %PDF-
		if len(b) < 5 {
			return false
		}
		return bytes.Equal(b[0:5], []byte("%PDF-"))

	case FormatDOC:
		// OLE2 Compound File \xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1
		if len(b) < 8 {
			return false
		}
		oleMagic := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
		return bytes.Equal(b[0:8], oleMagic)

	case FormatDOCX:
		// Zip PK Header \x50\x4B\x03\x04
		if len(b) < 4 {
			return false
		}
		pkMagic := []byte{0x50, 0x4B, 0x03, 0x04}
		return bytes.Equal(b[0:4], pkMagic)

	default:
		return false
	}
}
