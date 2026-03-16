package model

import "time"

type TransferJobStatus string

const (
	JobStatusQueued     TransferJobStatus = "queued"
	JobStatusValidating TransferJobStatus = "validating"
	JobStatusRunning    TransferJobStatus = "running"
	JobStatusCancelling TransferJobStatus = "cancelling"
	JobStatusCompleted  TransferJobStatus = "completed"
	JobStatusFailed     TransferJobStatus = "failed"
	JobStatusCancelled  TransferJobStatus = "cancelled"
)

type TransferJob struct {
	ID              string             `json:"id"`
	Operation       TransferOperation  `json:"operation"`
	SourceHost      string             `json:"source_host"`
	DestinationHost string             `json:"destination_host"`
	Status          TransferJobStatus  `json:"status"`
	AllowLive       bool               `json:"allow_live"`
	QuiesceSource   bool               `json:"quiesce_source"`
	RequestedBy     string             `json:"requested_by,omitempty"`
	Error           string             `json:"error,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
	StartedAt       *time.Time         `json:"started_at,omitempty"`
	FinishedAt      *time.Time         `json:"finished_at,omitempty"`
	Items           []TransferJobItem  `json:"items"`
	Events          []TransferJobEvent `json:"events,omitempty"`
	Summary         TransferJobSummary `json:"summary"`
}

type TransferJobSummary struct {
	TotalItems     int   `json:"total_items"`
	CompletedItems int   `json:"completed_items"`
	FailedItems    int   `json:"failed_items"`
	CancelledItems int   `json:"cancelled_items"`
	RunningItems   int   `json:"running_items"`
	QueuedItems    int   `json:"queued_items"`
	BytesEstimated int64 `json:"bytes_estimated"`
	BytesCopied    int64 `json:"bytes_copied"`
}

type TransferJobItem struct {
	Index             int               `json:"index"`
	SourceVolume      string            `json:"source_volume"`
	DestinationVolume string            `json:"destination_volume"`
	Status            TransferJobStatus `json:"status"`
	BytesEstimated    int64             `json:"bytes_estimated"`
	BytesCopied       int64             `json:"bytes_copied"`
	Warnings          []string          `json:"warnings,omitempty"`
	Error             string            `json:"error,omitempty"`
	SourceCleanup     string            `json:"source_cleanup,omitempty"`
}

type TransferJobEvent struct {
	ID        int64     `json:"id"`
	JobID     string    `json:"job_id"`
	ItemIndex int       `json:"item_index"`
	Level     string    `json:"level"`
	Step      string    `json:"step"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type TransferJobItemRequest struct {
	SourceVolume      string `json:"sourceVolume"`
	DestinationVolume string `json:"destinationVolume"`
}

type CreateTransferJobRequest struct {
	Operation       TransferOperation        `json:"operation"`
	SourceHost      string                   `json:"sourceHost"`
	DestinationHost string                   `json:"destinationHost"`
	AllowLive       bool                     `json:"allowLive"`
	QuiesceSource   bool                     `json:"quiesceSource"`
	Items           []TransferJobItemRequest `json:"items"`
}

type CreateTransferJobResponse struct {
	JobID string `json:"jobId"`
}
