package views

import (
	"html/template"
	"io"
	"io/fs"
	"path/filepath"
)

type Renderer struct {
	tpls *template.Template
}

func NewRenderer(templatesFS fs.FS) (*Renderer, error) {
	t := template.New("")
	var files []string
	if err := fs.WalkDir(templatesFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".html" {
			return nil
		}
		files = append(files, path)
		return nil
	}); err != nil {
		return nil, err
	}

	parsed, err := t.ParseFS(templatesFS, files...)
	if err != nil {
		return nil, err
	}
	return &Renderer{tpls: parsed}, nil
}

func (r *Renderer) Render(w io.Writer, name string, data any) error {
	return r.tpls.ExecuteTemplate(w, name, data)
}
