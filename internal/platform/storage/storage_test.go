package storage

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSignedURLJSONContract(t *testing.T) {
	payload, err := json.Marshal(SignedURL{URL: "https://assets.example/file.mp4", ExpiresAt: time.Unix(0, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	value := string(payload)
	if !strings.Contains(value, `"url":"https://assets.example/file.mp4"`) || !strings.Contains(value, `"expires_at":`) {
		t.Fatalf("unexpected signed URL JSON: %s", value)
	}
	if strings.Contains(value, `"URL"`) || strings.Contains(value, `"ExpiresAt"`) {
		t.Fatalf("signed URL leaked Go field names: %s", value)
	}
}
