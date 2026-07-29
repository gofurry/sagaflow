package service

import (
	"strings"
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

func TestValidateScreenshotParameters(t *testing.T) {
	tests := []struct {
		name    string
		params  mediaParameters
		wantErr bool
	}{
		{name: "original size", params: mediaParameters{ImageFormat: "png"}},
		{name: "resize width only", params: mediaParameters{ImageFormat: "jpg", OutputWidth: 1280}},
		{name: "crop and resize", params: mediaParameters{ImageFormat: "png", CropX: 10, CropY: 20, CropWidth: 640, CropHeight: 360, OutputWidth: 1920, OutputHeight: 1080}},
		{name: "reject incomplete crop", params: mediaParameters{ImageFormat: "png", CropX: 10}, wantErr: true},
		{name: "reject tiny output", params: mediaParameters{ImageFormat: "png", OutputWidth: 8}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMediaParameters("screenshot", tt.params)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestBuildScreenshotCommandWithCropAndResize(t *testing.T) {
	service := &MediaToolsService{}
	output, args, cleanup, err := service.buildCommand(db.MediaJob{Tool: "screenshot"}, mediaParameters{
		TimeSeconds:  1.25,
		ImageFormat:  "jpg",
		CropX:        20,
		CropY:        30,
		CropWidth:    640,
		CropHeight:   360,
		OutputWidth:  1280,
		OutputHeight: 720,
	}, []string{"source.mp4"})
	defer cleanup()
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if output.extension != ".jpg" || output.mimeType != "image/jpeg" || output.mediaType != "image" {
		t.Fatalf("unexpected output: %#v", output)
	}
	command := strings.Join(args, " ")
	for _, expected := range []string{"-ss 1.250", "-frames:v 1", "-vf crop=640:360:20:30,scale=1280:720", "-q:v 2"} {
		if !strings.Contains(command, expected) {
			t.Fatalf("command %q does not contain %q", command, expected)
		}
	}
}
