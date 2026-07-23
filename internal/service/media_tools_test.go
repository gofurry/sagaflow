package service

import (
	"testing"

	"github.com/gofurry/sagaflow/internal/store/db"
)

func TestValidateMediaSources(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		params  mediaParameters
		types   []string
		wantErr bool
	}{
		{name: "inspect image", tool: "inspect", types: []string{"image"}},
		{name: "audio transcode", tool: "transcode", params: mediaParameters{Format: "mp3"}, types: []string{"audio"}},
		{name: "reject audio as video", tool: "transcode", params: mediaParameters{Format: "mp4"}, types: []string{"audio"}, wantErr: true},
		{name: "video screenshot", tool: "screenshot", types: []string{"video"}},
		{name: "reject image screenshot", tool: "screenshot", types: []string{"image"}, wantErr: true},
		{name: "merge videos", tool: "merge", types: []string{"video", "video"}},
		{name: "reject mixed merge", tool: "merge", types: []string{"video", "audio"}, wantErr: true},
		{name: "mute video", tool: "audio", params: mediaParameters{AudioMode: "mute"}, types: []string{"video"}},
		{name: "reject mute audio", tool: "audio", params: mediaParameters{AudioMode: "mute"}, types: []string{"audio"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assets := make([]db.Asset, 0, len(tt.types))
			for _, mediaType := range tt.types {
				assets = append(assets, db.Asset{MediaType: mediaType})
			}
			err := validateMediaSources(tt.tool, tt.params, assets)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}
