package inference

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
)

// RequestSnapshot is the provider-neutral, persistence-safe representation of
// an inference request. It deliberately excludes credentials, content streams,
// and provider-reachable input URLs.
type RequestSnapshot struct {
	RequestID    string          `json:"request_id"`
	ProviderCode string          `json:"provider_code"`
	AdapterCode  string          `json:"adapter_code"`
	Endpoint     string          `json:"endpoint"`
	TargetKind   TargetKind      `json:"target_kind"`
	TargetID     string          `json:"target_id"`
	Capability   Capability      `json:"capability"`
	Prompt       string          `json:"prompt"`
	Parameters   map[string]any  `json:"parameters"`
	Inputs       []InputSnapshot `json:"inputs"`
}

type InputSnapshot struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	MediaType     string `json:"media_type"`
	MIMEType      string `json:"mime_type"`
	ProviderURL   bool   `json:"provider_url"`
	ContentStream bool   `json:"content_stream"`
}

type ResultSnapshot struct {
	Artifacts []ArtifactSnapshot `json:"artifacts"`
	Usage     map[string]any     `json:"usage"`
}

type ArtifactSnapshot struct {
	MediaType string         `json:"media_type"`
	MIMEType  string         `json:"mime_type"`
	SourceURL string         `json:"source_url,omitempty"`
	Metadata  map[string]any `json:"metadata"`
}

type FailureSnapshot struct {
	Kind       ErrorKind `json:"kind"`
	Provider   string    `json:"provider"`
	StatusCode int       `json:"status_code"`
	Retryable  bool      `json:"retryable"`
	Message    string    `json:"message"`
}

// SnapshotRequest returns a detached snapshot that is safe to persist and
// expose in the internal debugging UI. Runtime.APIKey is never copied.
func SnapshotRequest(request Request) RequestSnapshot {
	inputs := make([]InputSnapshot, 0, len(request.Inputs))
	for _, input := range request.Inputs {
		inputs = append(inputs, InputSnapshot{
			ID: input.ID, Name: input.Name, MediaType: input.MediaType, MIMEType: input.MIMEType,
			ProviderURL: strings.TrimSpace(input.URL) != "", ContentStream: input.Content != nil,
		})
	}
	return RequestSnapshot{
		RequestID: request.ID, ProviderCode: request.Runtime.ProviderCode, AdapterCode: request.Runtime.AdapterCode,
		Endpoint: sanitizeURL(request.Runtime.Endpoint), TargetKind: request.Target.Kind, TargetID: request.Target.ID,
		Capability: request.Target.Capability, Prompt: request.Prompt,
		Parameters: sanitizeMap(request.Parameters), Inputs: inputs,
	}
}

func SnapshotResult(result Result) ResultSnapshot {
	artifacts := make([]ArtifactSnapshot, 0, len(result.Artifacts))
	for _, artifact := range result.Artifacts {
		metadata := map[string]any{}
		if len(artifact.Metadata) > 0 {
			var decoded map[string]any
			if json.Unmarshal(artifact.Metadata, &decoded) == nil {
				metadata = sanitizeMap(decoded)
			}
		}
		artifacts = append(artifacts, ArtifactSnapshot{
			MediaType: artifact.MediaType, MIMEType: artifact.MIMEType,
			SourceURL: sanitizeURL(artifact.SourceURL), Metadata: metadata,
		})
	}
	return ResultSnapshot{Artifacts: artifacts, Usage: sanitizeMap(result.Usage)}
}

func SnapshotError(err error) FailureSnapshot {
	if err == nil {
		return FailureSnapshot{}
	}
	var providerErr *Error
	if errors.As(err, &providerErr) {
		return FailureSnapshot{
			Kind: providerErr.Kind, Provider: providerErr.Provider, StatusCode: providerErr.StatusCode,
			Retryable: providerErr.Retryable, Message: providerErr.Message,
		}
	}
	kind := ErrorProvider
	if errors.Is(err, context.DeadlineExceeded) {
		kind = ErrorTimeout
	}
	return FailureSnapshot{Kind: kind, Message: err.Error()}
}

// SnapshotDetails detaches and redacts provider event details before they are
// persisted. Adapters may use event details to expose their actual request
// payload without exposing credentials or signed URL query parameters.
func SnapshotDetails(details map[string]any) map[string]any {
	return sanitizeMap(details)
}

func sanitizeMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	clean := make(map[string]any, len(values))
	for key, value := range values {
		if sensitiveKey(key) {
			clean[key] = "[REDACTED]"
			continue
		}
		clean[key] = sanitizeValue(value)
	}
	return clean
}

func sanitizeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeMap(typed)
	case []any:
		clean := make([]any, len(typed))
		for index, item := range typed {
			clean[index] = sanitizeValue(item)
		}
		return clean
	case string:
		if strings.HasPrefix(typed, "http://") || strings.HasPrefix(typed, "https://") {
			return sanitizeURL(typed)
		}
	default:
		valueType := reflect.TypeOf(value)
		if valueType != nil && (valueType.Kind() == reflect.Map || valueType.Kind() == reflect.Slice || valueType.Kind() == reflect.Array) {
			encoded, err := json.Marshal(value)
			if err == nil {
				var normalized any
				if json.Unmarshal(encoded, &normalized) == nil {
					return sanitizeValue(normalized)
				}
			}
		}
	}
	return value
}

func sensitiveKey(key string) bool {
	normalized := strings.NewReplacer("-", "", "_", "", ".", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	for _, marker := range []string{"apikey", "authorization", "accesstoken", "refreshtoken", "password", "secret", "credential", "cookie", "signature", "xamzcredential"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return normalized == "token"
}

func sanitizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
