package media

import "testing"

func TestExtensionForMIMEIsDeterministic(t *testing.T) {
	tests := map[string]string{
		"text/plain; charset=utf-8": ".txt",
		"text/markdown":             ".md",
		"audio/mpeg":                ".mp3",
		"image/jpeg":                ".jpg",
		"IMAGE/PNG":                 ".png",
		"application/x-unknown":     ".bin",
		"":                          ".bin",
	}
	for mediaType, want := range tests {
		if got := ExtensionForMIME(mediaType); got != want {
			t.Errorf("ExtensionForMIME(%q) = %q, want %q", mediaType, got, want)
		}
	}
}
