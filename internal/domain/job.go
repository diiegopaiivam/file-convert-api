package domain

import (
	"time"
)

// JobStatus indica o estado atual do processamento de um job assíncrono.
type JobStatus string

const (
	StatusPending    JobStatus = "pending"
	StatusProcessing JobStatus = "processing"
	StatusDone       JobStatus = "done"
	StatusFailed     JobStatus = "failed"
)

// Job representa a entidade principal de conversão assíncrona.
type Job struct {
	ID              string     `json:"job_id"`
	SourceFormat    Format     `json:"source_format"`
	TargetFormat    Format     `json:"target_format"`
	SourceObjectKey string     `json:"source_object_key"`
	TargetObjectKey string     `json:"target_object_key,omitempty"`
	Status          JobStatus  `json:"status"`
	Error           *JobError  `json:"error,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// JobError detalha a causa de falha quando o job transiciona para o status "failed".
type JobError struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Details []ErrorDetail `json:"details,omitempty"`
}

// APIErrorResponse representa o formato JSON padronizado retornado em falhas síncronas.
type APIErrorResponse struct {
	Code      string        `json:"code"`
	Message   string        `json:"message"`
	Details   []ErrorDetail `json:"details,omitempty"`
	Timestamp string        `json:"timestamp"`
}

// ErrorDetail fornece informações contextuais específicas de um erro.
type ErrorDetail struct {
	Field string `json:"field,omitempty"`
	Issue string `json:"issue"`
}
