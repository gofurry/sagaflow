package runtimecontract

import (
	"os"
	"time"

	"github.com/gofurry/sagaflow/internal/config"
)

const (
	APIVersion         = 1
	DataSchemaVersion  = 1
	DesktopTokenHeader = "X-SagaFlow-Desktop-Token"
)

// RuntimeInfo is the desktop launcher's machine-readable handshake. It is
// ephemeral and must never be treated as persistent application state.
type RuntimeInfo struct {
	PID               int       `json:"pid"`
	URL               string    `json:"url"`
	Version           string    `json:"version"`
	APIVersion        int       `json:"api_version"`
	DataSchemaVersion int       `json:"data_schema_version"`
	StartedAt         time.Time `json:"started_at"`
}

func NewInfo(cfg config.Config) RuntimeInfo {
	return RuntimeInfo{
		PID:               os.Getpid(),
		URL:               "http://" + cfg.Address(),
		Version:           cfg.App.Version,
		APIVersion:        APIVersion,
		DataSchemaVersion: DataSchemaVersion,
		StartedAt:         time.Now().UTC(),
	}
}
