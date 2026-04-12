package web_entrypoint_index

import (
	"bytes"
	"slices"
	"text/template"

	"github.com/aperturerobotics/fastjson"

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
	var a fastjson.Arena
	obj := a.NewObject()
	imports := a.NewObject()
	keys := make([]string, 0, len(m.Imports))
	for key := range m.Imports {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		imports.Set(key, a.NewString(m.Imports[key]))
	}
	obj.Set("imports", imports)
	return string(obj.MarshalTo(nil))
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
