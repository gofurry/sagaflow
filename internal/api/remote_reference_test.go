package api

import (
	"context"
	"net"
	"testing"
)

func TestValidateGenerationReferenceURL(t *testing.T) {
	publicLookup := func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	parsed, err := validateGenerationReferenceURL(context.Background(), " https://example.com/reference.png ", publicLookup)
	if err != nil {
		t.Fatalf("expected public URL to be accepted: %v", err)
	}
	if parsed.String() != "https://example.com/reference.png" {
		t.Fatalf("unexpected normalized URL: %q", parsed.String())
	}

	privateLookup := func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	if _, err := validateGenerationReferenceURL(context.Background(), "http://localhost/reference.png", privateLookup); err == nil {
		t.Fatal("expected local URL to be rejected")
	}
	if _, err := validateGenerationReferenceURL(context.Background(), "file:///tmp/reference.png", publicLookup); err == nil {
		t.Fatal("expected non-HTTP URL to be rejected")
	}
	if _, err := validateGenerationReferenceURL(context.Background(), "https://user:password@example.com/reference.png", publicLookup); err == nil {
		t.Fatal("expected URL credentials to be rejected")
	}
}

func TestGenerationReferenceSourceURL(t *testing.T) {
	if got := generationReferenceSourceURL([]byte(`{"source_kind":"url","source_url":"https://example.com/reference.png"}`)); got != "https://example.com/reference.png" {
		t.Fatalf("unexpected source URL: %q", got)
	}
	for _, raw := range [][]byte{
		[]byte(`{"source_kind":"upload","source_url":"https://example.com/reference.png"}`),
		[]byte(`{"source_kind":"url","source_url":"file:///tmp/reference.png"}`),
		[]byte(`not-json`),
	} {
		if got := generationReferenceSourceURL(raw); got != "" {
			t.Fatalf("expected unusable metadata to be ignored, got %q", got)
		}
	}
}
