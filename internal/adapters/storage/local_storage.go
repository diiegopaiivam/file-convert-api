package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/file-convert-api/internal/ports"
)

var _ ports.ObjectStorage = (*LocalStorage)(nil)

type LocalStorage struct {
	basePath string
}

func NewLocalStorage(basePath string) (*LocalStorage, error) {
	if basePath == "" {
		basePath = filepath.Join(os.TempDir(), "file-convert-storage")
	}
	if err := os.MkdirAll(basePath, 0777); err != nil {
		fallbackPath := filepath.Join(os.TempDir(), "file-convert-storage")
		if errFallback := os.MkdirAll(fallbackPath, 0777); errFallback == nil {
			return &LocalStorage{basePath: fallbackPath}, nil
		}
		return nil, fmt.Errorf("falha ao criar pasta de storage local (%s): %w", basePath, err)
	}
	return &LocalStorage{basePath: basePath}, nil
}

func (s *LocalStorage) Upload(ctx context.Context, key string, content io.Reader, size int64, contentType string) error {
	fullPath := filepath.Join(s.basePath, key)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0777); err != nil {
		fallbackPath := filepath.Join(os.TempDir(), "file-convert-storage", key)
		if errFallback := os.MkdirAll(filepath.Dir(fallbackPath), 0777); errFallback == nil {
			fullPath = fallbackPath
		} else {
			return fmt.Errorf("falha ao criar subdiretório do storage (%s): %w", filepath.Dir(fullPath), err)
		}
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("falha ao criar arquivo no storage (%s): %w", fullPath, err)
	}
	defer f.Close()

	_, err = io.Copy(f, content)
	return err
}

func (s *LocalStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	fullPath := filepath.Join(s.basePath, key)
	f, err := os.Open(fullPath)
	if err != nil {
		fallbackPath := filepath.Join(os.TempDir(), "file-convert-storage", key)
		return os.Open(fallbackPath)
	}
	return f, nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	fullPath := filepath.Join(s.basePath, key)
	err := os.Remove(fullPath)
	if err != nil {
		fallbackPath := filepath.Join(os.TempDir(), "file-convert-storage", key)
		_ = os.Remove(fallbackPath)
	}
	return nil
}
