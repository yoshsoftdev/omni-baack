package importjob

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Source string

const (
	SourceCSV    Source = "csv"
	SourceExcel  Source = "excel"
	SourceManual Source = "manual"
)

type JobStatus string

const (
	StatusPending    JobStatus = "pending"
	StatusProcessing JobStatus = "processing"
	StatusCompleted  JobStatus = "completed"
	StatusFailed     JobStatus = "failed"
)

type RowStatus string

const (
	RowImported RowStatus = "imported"
	RowUpdated  RowStatus = "updated"
	RowSkipped  RowStatus = "skipped"
	RowFailed   RowStatus = "failed"
)

type Job struct {
	ID            uuid.UUID
	TeamID        uuid.UUID
	Source        Source
	Status        JobStatus
	Filename      string
	TotalRows     int
	ImportedCount int
	UpdatedCount  int
	SkippedCount  int
	FailedCount   int
	ErrorSummary  *string
	CreatedBy     *uuid.UUID
	StartedAt     *time.Time
	CompletedAt   *time.Time
	CreatedAt     time.Time
}

type File struct {
	ID         uuid.UUID
	JobID      uuid.UUID
	StorageKey string
	MimeType   string
	SizeBytes  int64
	Checksum   string
	CreatedAt  time.Time
}

type Row struct {
	ID           uuid.UUID
	JobID        uuid.UUID
	RowNumber    int
	Status       RowStatus
	ContactID    *uuid.UUID
	RawData      json.RawMessage
	ErrorMessage *string
	CreatedAt    time.Time
}

type ListFilter struct {
	Status *JobStatus
}
