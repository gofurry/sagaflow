package api

import "testing"

func TestDownloadFilenameRestoresOriginalExtension(t *testing.T) {
	tests := []struct {
		name, original, mimeType, want string
	}{
		{name: "最终分镜", original: "seedance-output.mp4", mimeType: "video/mp4", want: "最终分镜.mp4"},
		{name: "狼兽人.png", original: "upload.png", mimeType: "image/png", want: "狼兽人.png"},
		{name: "旁白", original: "voice", mimeType: "audio/mpeg", want: "旁白.mp3"},
	}
	for _, test := range tests {
		if got := downloadFilename(test.name, test.original, test.mimeType); got != test.want {
			t.Errorf("downloadFilename(%q, %q, %q) = %q, want %q", test.name, test.original, test.mimeType, got, test.want)
		}
	}
}
