package templates

import (
	"html/template"
	"io"

	"github.com/labstack/echo/v4"
)

// Template is the structure that implements the template renderer interface
type Template struct {
	templates *template.Template
}

// SetTemplates sets the templates
func (t *Template) SetTemplates(templates *template.Template) {
	t.templates = templates
}

// Render renders a template document
func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}
