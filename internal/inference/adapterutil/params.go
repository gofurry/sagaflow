package adapterutil

import (
	"encoding/json"
	"path"
	"strings"
)

func JoinURL(baseURL, urlPath string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return baseURL + "/" + strings.TrimLeft(path.Clean("/"+urlPath), "/")
}

func CopyParam(dst, src map[string]any, key string) {
	if value, ok := src[key]; ok {
		dst[key] = value
	}
}

func StringParam(values map[string]any, key, fallback string) string {
	if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func NumberParam(values map[string]any, key string, fallback float64) float64 {
	if value, ok := values[key].(float64); ok {
		return value
	}
	if value, ok := values[key].(int); ok {
		return float64(value)
	}
	return fallback
}

func BoolParam(values map[string]any, key string, fallback bool) bool {
	if value, ok := values[key].(bool); ok {
		return value
	}
	return fallback
}

func FindString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if found, ok := value[key].(string); ok && found != "" {
			return found
		}
	}
	for _, child := range value {
		if nested, ok := child.(map[string]any); ok {
			if found := FindString(nested, keys...); found != "" {
				return found
			}
		}
	}
	return ""
}

func FindMediaURL(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"video_url", "url"} {
			if raw, ok := typed[key].(string); ok && (strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")) {
				return raw
			}
		}
		for _, child := range typed {
			if found := FindMediaURL(child); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := FindMediaURL(child); found != "" {
				return found
			}
		}
	}
	return ""
}

func JSONSummary(raw json.RawMessage, keys ...string) json.RawMessage {
	if len(keys) == 0 {
		return append(json.RawMessage(nil), raw...)
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return json.RawMessage(`{}`)
	}
	out := map[string]any{}
	for _, key := range keys {
		if value, ok := root[key]; ok {
			out[key] = value
		}
	}
	encoded, _ := json.Marshal(out)
	return encoded
}

func JSON(value any) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}

func Truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "…"
}
