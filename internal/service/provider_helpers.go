package service

import (
	"encoding/json"

	"github.com/gofurry/sagaflow/internal/store/db"
)

// jsonSummary and truncate remain in service for the voice-cloning workflow,
// which is lifecycle management rather than a generation execution adapter.
func jsonSummary(raw json.RawMessage, keys ...string) json.RawMessage {
	if len(keys) == 0 {
		return raw
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return db.JSON(map[string]any{})
	}
	out := map[string]any{}
	for _, key := range keys {
		if value, ok := root[key]; ok {
			out[key] = value
		}
	}
	return db.JSON(out)
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "…"
}
