package models

import "time"

type VerificationStatus string

const (
	VerificationPending VerificationStatus = "pending"
	VerificationRunning VerificationStatus = "running"
	VerificationPassed  VerificationStatus = "passed"
	VerificationFailed  VerificationStatus = "failed"
)

type DatabaseComparison struct {
	Database        string  `json:"database"`
	SourceSizeBytes int64   `json:"source_size_bytes"`
	TargetSizeBytes int64   `json:"target_size_bytes"`
	TargetExists    bool    `json:"target_exists"`
	SizeMatch       bool    `json:"size_match"`
	SizeDiffPercent float64 `json:"size_diff_percent"`
	Status          string  `json:"status"` // ok, missing, size_mismatch, source_only
}

type VerificationSummary struct {
	TotalDatabases int  `json:"total_databases"`
	Matched        int  `json:"matched"`
	Missing        int  `json:"missing"`
	SizeMismatch   int  `json:"size_mismatch"`
	Passed         bool `json:"passed"`
}

type Verification struct {
	Status      VerificationStatus   `json:"status"`
	JobName     string               `json:"job_name,omitempty"`
	Databases   []DatabaseComparison `json:"databases,omitempty"`
	Summary     VerificationSummary  `json:"summary"`
	Error       string               `json:"error,omitempty"`
	StartedAt   *time.Time           `json:"started_at,omitempty"`
	CompletedAt *time.Time           `json:"completed_at,omitempty"`
}
