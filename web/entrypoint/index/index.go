package web_entrypoint_index

import (
	"bytes"
	"encoding/json"
	"text/template"

	// _ enables embed
	_ "embed"
)

//go:embed index.html
var indexHTML string

// ImportMap is the <importmap> tag contents.
type ImportMap struct {
	Imports map[string]string `json:"imports"`
}

// IndexData contains the params for the index.html template
type IndexData struct {
	ImportMap      ImportMap
	EntrypointPath string
}

// String returns the JSON string representation of ImportMap
func (m ImportMap) String() string {
	b, _ := json.Marshal(m)
	return string(b)
}

// RenderIndexHTML processes the index.html template with the provided data.
func RenderIndexHTML(data IndexData) (string, error) {
	tmpl, err := template.New("index").Parse(indexHTML)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
