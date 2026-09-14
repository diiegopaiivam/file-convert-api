package domain

import "errors"

const (
	ErrCodeEmptyFile               = "EMPTY_FILE"
	ErrCodeFileTooLarge            = "FILE_TOO_LARGE"
	ErrCodeUnsupportedFormat        = "UNSUPPORTED_FORMAT"
	ErrCodeCorruptedFile           = "CORRUPTED_FILE"
	ErrCodeJobTimeout              = "JOB_TIMEOUT"
	ErrCodeInternalError           = "INTERNAL_ERROR"
	ErrCodeUnauthorized            = "UNAUTHORIZED"
	ErrCodeForbidden               = "FORBIDDEN"
	ErrCodeRateLimitExceeded       = "RATE_LIMIT_EXCEEDED"
	ErrCodeConcurrentJobsExceeded  = "CONCURRENT_JOBS_EXCEEDED"
	ErrCodePasswordProtectedPDF      = "PASSWORD_PROTECTED_PDF"
	ErrCodePasswordProtectedDocument = "PASSWORD_PROTECTED_DOCUMENT"
	ErrCodeEmptyPDFPages             = "EMPTY_PDF_PAGES"
	ErrCodeEmptyDocumentPages        = "EMPTY_DOCUMENT_PAGES"
	ErrCodePageLimitExceeded         = "PAGE_LIMIT_EXCEEDED"
	ErrCodeImageResolutionExceeded   = "IMAGE_RESOLUTION_EXCEEDED"
)

var (
	ErrEmptyFile                 = errors.New("file is empty")
	ErrFileTooLarge              = errors.New("file exceeds max size limit")
	ErrUnsupportedFormat         = errors.New("unsupported file format")
	ErrCorruptedFile             = errors.New("file is corrupted or invalid")
	ErrJobTimeout                = errors.New("job execution timed out")
	ErrJobNotFound               = errors.New("job not found")
	ErrInternalError             = errors.New("internal error")
	ErrUnauthorized              = errors.New("unauthorized: invalid or missing API key")
	ErrRateLimitExceeded         = errors.New("rate limit exceeded")
	ErrConcurrentJobsExceeded    = errors.New("concurrent jobs quota exceeded")
	ErrPasswordProtectedPDF      = errors.New("pdf is password protected")
	ErrPasswordProtectedDocument = errors.New("document is password protected")
	ErrEmptyPDFPages             = errors.New("pdf has no readable pages")
	ErrEmptyDocumentPages        = errors.New("document has no readable pages")
	ErrPageLimitExceeded         = errors.New("page count exceeds maximum allowed limit")
	ErrImageResolutionExceeded   = errors.New("image exceeds max resolution limit")
)
