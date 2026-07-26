package webui

import (
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestEmbeddedCachePolicy(t *testing.T) {
	app := fiber.New()
	Mount(app)

	for _, route := range []string{"/", "/projects/example"} {
		response, err := app.Test(httptest.NewRequest("GET", route, nil))
		if err != nil {
			t.Fatalf("request %s failed: %v", route, err)
		}
		if got := response.Header.Get(fiber.HeaderCacheControl); got != "no-cache" {
			t.Fatalf("expected index response %s to bypass cache, got %q", route, got)
		}
	}

	dist, err := fs.Sub(embedded, "dist")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(dist, "assets")
	if err != nil || len(entries) == 0 {
		t.Fatalf("cannot find embedded asset: %v", err)
	}
	response, err := app.Test(httptest.NewRequest("GET", "/assets/"+entries[0].Name(), nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := response.Header.Get(fiber.HeaderCacheControl); !strings.Contains(got, "immutable") {
		t.Fatalf("expected fingerprinted asset to be immutable, got %q", got)
	}
}
