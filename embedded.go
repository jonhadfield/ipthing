package main

import (
	"embed"
	"html/template"
	"io"
	"io/fs"

	"github.com/labstack/echo/v4"
)

// Embed the entire public directory
//
//go:embed public/*
var publicFS embed.FS

// EmbeddedRenderer implements echo.Renderer interface for embedded templates
type EmbeddedRenderer struct {
	templates *template.Template
}

// NewEmbeddedRenderer creates a new renderer with embedded templates
func NewEmbeddedRenderer() (*EmbeddedRenderer, error) {
	// Create a sub-filesystem for just the views directory
	viewsFS, err := fs.Sub(publicFS, "public/views")
	if err != nil {
		return nil, err
	}

	// html/template auto-escapes reflected request fields (UA, Referer, XFF, etc.).
	tmpl, err := template.ParseFS(viewsFS, "*.html")
	if err != nil {
		return nil, err
	}

	return &EmbeddedRenderer{
		templates: tmpl,
	}, nil
}

// Render implements echo.Renderer interface
func (er *EmbeddedRenderer) Render(w io.Writer, name string, data any, c echo.Context) error {
	return er.templates.ExecuteTemplate(w, name, data)
}

// GetAssetFS returns the embedded filesystem for assets
func GetAssetFS() fs.FS {
	assetsFS, _ := fs.Sub(publicFS, "public/assets")
	return assetsFS
}
