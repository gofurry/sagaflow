package api

import (
	"testing"

	"github.com/google/uuid"
)

func TestValidateImageTask(t *testing.T) {
	source := generationInputReferenceRequest{Source: "asset", ID: uuid.New()}
	mask := generationInputReferenceRequest{Source: "upload", ID: uuid.New()}
	target := resolvedGenerationTarget{Kind: "model", Capability: "image", Features: []string{"outpaint", "inpaint", "mask_input"}}

	t.Run("outpaint", func(t *testing.T) {
		task := &generationImageTaskRequest{
			Type: "outpaint", SourceWidth: 1024, SourceHeight: 768, TargetWidth: 1536, TargetHeight: 960,
			SourceX: 256, SourceY: 96, TopScale: 1.125, BottomScale: 1.125, LeftScale: 1.25, RightScale: 1.25,
		}
		if err := validateImageTask(target, task, []generationInputReferenceRequest{source}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("inpaint", func(t *testing.T) {
		task := &generationImageTaskRequest{Type: "inpaint", SourceWidth: 1024, SourceHeight: 768}
		if err := validateImageTask(target, task, []generationInputReferenceRequest{source, mask}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects generic image model", func(t *testing.T) {
		task := &generationImageTaskRequest{Type: "outpaint", SourceWidth: 1024, SourceHeight: 768, TargetWidth: 1536, TargetHeight: 960, TopScale: 1.125, BottomScale: 1.125, LeftScale: 1.25, RightScale: 1.25}
		if err := validateImageTask(resolvedGenerationTarget{Kind: "model", Capability: "image"}, task, []generationInputReferenceRequest{source}); err == nil {
			t.Fatal("expected unsupported outpaint error")
		}
	})

	t.Run("rejects tampered outpaint geometry", func(t *testing.T) {
		task := &generationImageTaskRequest{
			Type: "outpaint", SourceWidth: 1024, SourceHeight: 768, TargetWidth: 1537, TargetHeight: 960,
			SourceX: 256, SourceY: 96, TopScale: 1.125, BottomScale: 1.125, LeftScale: 1.25, RightScale: 1.25,
		}
		if err := validateImageTask(target, task, []generationInputReferenceRequest{source}); err == nil {
			t.Fatal("expected mismatched outpaint geometry error")
		}
	})

	t.Run("rejects non-upload mask", func(t *testing.T) {
		task := &generationImageTaskRequest{Type: "inpaint", SourceWidth: 1024, SourceHeight: 768}
		if err := validateImageTask(target, task, []generationInputReferenceRequest{source, {Source: "asset", ID: uuid.New()}}); err == nil {
			t.Fatal("expected generated mask validation error")
		}
	})
}

func TestValidateVideoReferences(t *testing.T) {
	reference := generationInputReferenceRequest{Source: "asset", ID: uuid.New()}

	t.Run("rejects image-only video model without a storyboard reference", func(t *testing.T) {
		target := resolvedGenerationTarget{Kind: "model", Capability: "video", Features: []string{"video_generation", "image_to_video"}}
		if err := validateVideoReferences(target, nil); err == nil {
			t.Fatal("expected missing storyboard reference error")
		}
	})

	t.Run("accepts image-only video model with a storyboard reference", func(t *testing.T) {
		target := resolvedGenerationTarget{Kind: "model", Capability: "video", Features: []string{"video_generation", "image_to_video"}}
		if err := validateVideoReferences(target, []generationInputReferenceRequest{reference}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("accepts text-to-video model without a storyboard reference", func(t *testing.T) {
		target := resolvedGenerationTarget{Kind: "model", Capability: "video", Features: []string{"video_generation", "text_to_video", "image_to_video"}}
		if err := validateVideoReferences(target, nil); err != nil {
			t.Fatal(err)
		}
	})
}
