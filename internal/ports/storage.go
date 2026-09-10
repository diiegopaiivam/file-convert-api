package ports

import (
	"context"
	"io"
)

// ObjectStorage abstrai o armazenamento de arquivos binários.
type ObjectStorage interface {
	// Upload armazena um arquivo e retorna a chave única do objeto armazenado.
	Upload(ctx context.Context, key string, content io.Reader, size int64, contentType string) error

	// Download recupera o stream de leitura de um arquivo armazenado.
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete remove um objeto do storage.
	Delete(ctx context.Context, key string) error
}
