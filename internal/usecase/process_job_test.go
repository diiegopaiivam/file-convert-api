package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
	"github.com/file-convert-api/internal/usecase"
)

// --- FAKES / MOCKS DOS PORTS ---

type FakeJobRepository struct {
	mu   sync.Mutex
	jobs map[string]*domain.Job
}

func NewFakeJobRepository() *FakeJobRepository {
	return &FakeJobRepository{
		jobs: make(map[string]*domain.Job),
	}
}

func (r *FakeJobRepository) Create(ctx context.Context, job *domain.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs[job.ID] = job
	return nil
}

func (r *FakeJobRepository) GetByID(ctx context.Context, id string) (*domain.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return nil, nil
	}
	return job, nil
}

func (r *FakeJobRepository) UpdateStatus(ctx context.Context, id string, status domain.JobStatus, targetKey string, jobErr *domain.JobError) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return domain.ErrJobNotFound
	}
	job.Status = status
	if targetKey != "" {
		job.TargetObjectKey = targetKey
	}
	if jobErr != nil {
		job.Error = jobErr
	}
	job.UpdatedAt = time.Now()
	return nil
}

type FakeObjectStorage struct {
	mu    sync.Mutex
	files map[string][]byte
}

func NewFakeObjectStorage() *FakeObjectStorage {
	return &FakeObjectStorage{
		files: make(map[string][]byte),
	}
}

func (s *FakeObjectStorage) Upload(ctx context.Context, key string, content io.Reader, size int64, contentType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	buf, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	s.files[key] = buf
	return nil
}

func (s *FakeObjectStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.files[key]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", key)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *FakeObjectStorage) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.files, key)
	return nil
}

type FakeConverter struct {
	canConvertFunc func(src, target domain.Format) bool
	convertFunc    func(ctx context.Context, src io.Reader, srcFormat, targetFormat domain.Format) (io.Reader, error)
}

func (c *FakeConverter) CanConvert(src, target domain.Format) bool {
	if c.canConvertFunc != nil {
		return c.canConvertFunc(src, target)
	}
	return true
}

func (c *FakeConverter) Convert(ctx context.Context, src io.Reader, srcFormat, targetFormat domain.Format) (io.Reader, error) {
	if c.convertFunc != nil {
		return c.convertFunc(ctx, src, srcFormat, targetFormat)
	}
	return bytes.NewReader([]byte("fake-converted-audio-content")), nil
}

// --- TESTES UNITÁRIOS ---

func TestProcessJobUseCase_Execute_Success(t *testing.T) {
	repo := NewFakeJobRepository()
	storage := NewFakeObjectStorage()
	registry := usecase.NewDefaultConverterRegistry()

	converter := &FakeConverter{
		canConvertFunc: func(src, target domain.Format) bool {
			return src == domain.FormatWAV && target == domain.FormatMP3
		},
		convertFunc: func(ctx context.Context, src io.Reader, srcFormat, targetFormat domain.Format) (io.Reader, error) {
			return bytes.NewReader([]byte("converted-mp3-stream")), nil
		},
	}
	_ = registry.Register(converter)

	// Setup initial job and source file in storage
	job := &domain.Job{
		ID:              "job-123",
		SourceFormat:    domain.FormatWAV,
		TargetFormat:    domain.FormatMP3,
		SourceObjectKey: "sources/input.wav",
		Status:          domain.StatusPending,
		CreatedAt:       time.Now(),
	}
	_ = repo.Create(context.Background(), job)
	_ = storage.Upload(context.Background(), "sources/input.wav", bytes.NewReader([]byte("dummy-wav-content")), 17, "audio/wav")

	uc := usecase.NewProcessJobUseCase(repo, storage, registry)
	err := uc.Execute(context.Background(), "job-123")

	if err != nil {
		t.Fatalf("Esperado sucesso no processamento do job, obtido erro: %v", err)
	}

	updatedJob, _ := repo.GetByID(context.Background(), "job-123")
	if updatedJob.Status != domain.StatusDone {
		t.Errorf("Status esperado %s, obtido %s", domain.StatusDone, updatedJob.Status)
	}
	if updatedJob.TargetObjectKey != "converted/job-123.mp3" {
		t.Errorf("TargetObjectKey esperado 'converted/job-123.mp3', obtido '%s'", updatedJob.TargetObjectKey)
	}

	// Verify target file exists in storage
	stream, err := storage.Download(context.Background(), "converted/job-123.mp3")
	if err != nil {
		t.Fatalf("Arquivo convertido não foi salvo no storage: %v", err)
	}
	defer stream.Close()
	content, _ := io.ReadAll(stream)
	if string(content) != "converted-mp3-stream" {
		t.Errorf("Conteúdo convertido retornado inconsistente: %s", string(content))
	}
}

func TestProcessJobUseCase_Execute_CorruptedFileError(t *testing.T) {
	repo := NewFakeJobRepository()
	storage := NewFakeObjectStorage()
	registry := usecase.NewDefaultConverterRegistry()

	converter := &FakeConverter{
		convertFunc: func(ctx context.Context, src io.Reader, srcFormat, targetFormat domain.Format) (io.Reader, error) {
			return nil, domain.ErrCorruptedFile
		},
	}
	_ = registry.Register(converter)

	job := &domain.Job{
		ID:              "job-corrupted",
		SourceFormat:    domain.FormatWAV,
		TargetFormat:    domain.FormatMP3,
		SourceObjectKey: "sources/corrupted.wav",
		Status:          domain.StatusPending,
	}
	_ = repo.Create(context.Background(), job)
	_ = storage.Upload(context.Background(), "sources/corrupted.wav", bytes.NewReader([]byte("bad-data")), 8, "audio/wav")

	uc := usecase.NewProcessJobUseCase(repo, storage, registry)
	err := uc.Execute(context.Background(), "job-corrupted")

	if err == nil || !errors.Is(err, domain.ErrCorruptedFile) {
		t.Fatalf("Esperado erro ErrCorruptedFile, obtido: %v", err)
	}

	updatedJob, _ := repo.GetByID(context.Background(), "job-corrupted")
	if updatedJob.Status != domain.StatusFailed {
		t.Errorf("Status esperado %s, obtido %s", domain.StatusFailed, updatedJob.Status)
	}
	if updatedJob.Error == nil || updatedJob.Error.Code != domain.ErrCodeCorruptedFile {
		t.Errorf("Codigo de erro no job esperado %s, obtido %v", domain.ErrCodeCorruptedFile, updatedJob.Error)
	}
}

func TestProcessJobUseCase_Execute_TimeoutError(t *testing.T) {
	repo := NewFakeJobRepository()
	storage := NewFakeObjectStorage()
	registry := usecase.NewDefaultConverterRegistry()

	converter := &FakeConverter{
		convertFunc: func(ctx context.Context, src io.Reader, srcFormat, targetFormat domain.Format) (io.Reader, error) {
			return nil, domain.ErrJobTimeout
		},
	}
	_ = registry.Register(converter)

	job := &domain.Job{
		ID:              "job-timeout",
		SourceFormat:    domain.FormatWAV,
		TargetFormat:    domain.FormatMP3,
		SourceObjectKey: "sources/slow.wav",
		Status:          domain.StatusPending,
	}
	_ = repo.Create(context.Background(), job)
	_ = storage.Upload(context.Background(), "sources/slow.wav", bytes.NewReader([]byte("heavy-data")), 10, "audio/wav")

	uc := usecase.NewProcessJobUseCase(repo, storage, registry)
	err := uc.Execute(context.Background(), "job-timeout")

	if err == nil || !errors.Is(err, domain.ErrJobTimeout) {
		t.Fatalf("Esperado erro ErrJobTimeout, obtido: %v", err)
	}

	updatedJob, _ := repo.GetByID(context.Background(), "job-timeout")
	if updatedJob.Status != domain.StatusFailed {
		t.Errorf("Status esperado %s, obtido %s", domain.StatusFailed, updatedJob.Status)
	}
	if updatedJob.Error == nil || updatedJob.Error.Code != domain.ErrCodeJobTimeout {
		t.Errorf("Codigo de erro no job esperado %s, obtido %v", domain.ErrCodeJobTimeout, updatedJob.Error)
	}
}

func TestProcessJobUseCase_Execute_UnsupportedFormatError(t *testing.T) {
	repo := NewFakeJobRepository()
	storage := NewFakeObjectStorage()
	registry := usecase.NewDefaultConverterRegistry() // registry vazio

	job := &domain.Job{
		ID:              "job-unsupported",
		SourceFormat:    domain.FormatDOCX,
		TargetFormat:    domain.FormatMP3,
		SourceObjectKey: "sources/doc.docx",
		Status:          domain.StatusPending,
	}
	_ = repo.Create(context.Background(), job)

	uc := usecase.NewProcessJobUseCase(repo, storage, registry)
	err := uc.Execute(context.Background(), "job-unsupported")

	if err == nil || !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Fatalf("Esperado erro ErrUnsupportedFormat, obtido: %v", err)
	}

	updatedJob, _ := repo.GetByID(context.Background(), "job-unsupported")
	if updatedJob.Status != domain.StatusFailed {
		t.Errorf("Status esperado %s, obtido %s", domain.StatusFailed, updatedJob.Status)
	}
	if updatedJob.Error == nil || updatedJob.Error.Code != domain.ErrCodeUnsupportedFormat {
		t.Errorf("Codigo de erro no job esperado %s, obtido %v", domain.ErrCodeUnsupportedFormat, updatedJob.Error)
	}
}

func TestProcessJobUseCase_Execute_JobNotFound(t *testing.T) {
	repo := NewFakeJobRepository()
	storage := NewFakeObjectStorage()
	registry := usecase.NewDefaultConverterRegistry()

	uc := usecase.NewProcessJobUseCase(repo, storage, registry)
	err := uc.Execute(context.Background(), "inexistent-job-id")

	if !errors.Is(err, domain.ErrJobNotFound) {
		t.Fatalf("Esperado erro ErrJobNotFound, obtido: %v", err)
	}
}
