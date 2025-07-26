package main

import (
	"github.com/labstack/echo/v4"
	"io"
	"text/template"
)

// Template provides template rendering functionality
type Template struct {
	templates *template.Template
}

// NewTemplate creates a new template renderer
func NewTemplate(pattern string) (*Template, error) {
	templates, err := template.ParseGlob(pattern)
	if err != nil {
		return nil, err
	}
	return &Template{templates: templates}, nil
}

// Render renders a template with the given data
func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}
