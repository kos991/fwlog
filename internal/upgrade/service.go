package upgrade

import "time"

type State string

const (
	StateIdle      State = "idle"
	StateRunning   State = "running"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"
)

type Status struct {
	State          State     `json:"state"`
	CurrentVersion string    `json:"current_version"`
	TargetVersion  string    `json:"target_version,omitempty"`
	Message        string    `json:"message,omitempty"`
	Error          string    `json:"error,omitempty"`
	BackupPath     string    `json:"backup_path,omitempty"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	FinishedAt     time.Time `json:"finished_at,omitempty"`
}

type Service interface {
	Status() Status
}
