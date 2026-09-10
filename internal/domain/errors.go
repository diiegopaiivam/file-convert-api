package domain

import "errors"

const (
	ErrCodeEmptyFile        = "EMPTY_FILE"
	ErrCodeFileTooLarge     = "FILE_TOO_LARGE"
	ErrCodeUnsupportedFormat = "UNSUPPORTED_FORMAT"
	ErrCodeCorruptedFile    = "CORRUPTED_FILE"
	ErrCodeJobTimeout       = "JOB_TIMEOUT"
	ErrCodeInternalError    = "INTERNAL_ERROR"
)

var (
	ErrEmptyFile         = errors.New("file is empty")
	ErrFileTooLarge      = errors.New("file exceeds max size limit")
	ErrUnsupportedFormat = errors.New("unsupported file format")
	ErrCorruptedFile     = errors.New("file is corrupted or invalid")
	ErrJobTimeout        = errors.New("job execution timed out")
	ErrJobNotFound       = errors.New("job not found")
	ErrInternalError     = errors.New("internal error")
)
