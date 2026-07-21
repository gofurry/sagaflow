package webui

import (
	"embed"
	"io/fs"
	"mime"
	"path"
	"strings"

	"github.com/gofiber/fiber/v3"
)

//go:embed dist/*
var embedded embed.FS

func Mount(app *fiber.App) {
	dist, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err)
	}
	app.Get("/*", func(c fiber.Ctx) error {
		name := strings.TrimPrefix(path.Clean("/"+c.Path()), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		data, readErr := fs.ReadFile(dist, name)
		if readErr != nil {
			if strings.HasPrefix(name, "api/") || strings.Contains(path.Base(name), ".") {
				return fiber.ErrNotFound
			}
			data, readErr = fs.ReadFile(dist, "index.html")
			name = "index.html"
		}
		if readErr != nil {
			return fiber.ErrNotFound
		}
		if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
			c.Set(fiber.HeaderContentType, contentType)
		}
		return c.Send(data)
	})
}
