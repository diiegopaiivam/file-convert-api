package domain

// Format representa o formato/extensão de um arquivo suportado na conversão.
type Format string

const (
	FormatWAV  Format = "wav"
	FormatMP3  Format = "mp3"
	FormatJPG  Format = "jpg"
	FormatPNG  Format = "png"
	FormatPDF  Format = "pdf"
	FormatDOC  Format = "doc"
	FormatDOCX Format = "docx"
)
