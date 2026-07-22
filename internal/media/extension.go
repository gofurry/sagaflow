package media

import "strings"

// ExtensionForMIME returns a deterministic file extension for media types
// SagaFlow stores or exchanges. It deliberately avoids mime.ExtensionsByType:
// that function consults the host MIME registry and can return different first
// choices on Windows, Linux and macOS (for example .mp2 instead of .mp3).
func ExtensionForMIME(value string) string {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
	if extension, ok := extensions[mediaType]; ok {
		return extension
	}
	return ".bin"
}

var extensions = map[string]string{
	"application/json":         ".json",
	"application/octet-stream": ".bin",
	"application/pdf":          ".pdf",
	"application/zip":          ".zip",
	"audio/flac":               ".flac",
	"audio/mp4":                ".m4a",
	"audio/mpeg":               ".mp3",
	"audio/ogg":                ".ogg",
	"audio/wav":                ".wav",
	"audio/x-wav":              ".wav",
	"image/gif":                ".gif",
	"image/jpeg":               ".jpg",
	"image/png":                ".png",
	"image/svg+xml":            ".svg",
	"image/webp":               ".webp",
	"text/html":                ".html",
	"text/markdown":            ".md",
	"text/plain":               ".txt",
	"video/mp4":                ".mp4",
	"video/webm":               ".webm",
}
