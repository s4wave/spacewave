package web_entrypoint_index

import (
	"errors"
	"html"
	"slices"
	"strings"

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

// RenderIndexHTML renders the embedded browser index shell with the provided data.
func RenderIndexHTML(data IndexData) (string, error) {
	const (
		// #nosec G101 -- These are Go template markers, not credentials.
		importMapToken = "{{ .ImportMap.String }}"
		// #nosec G101 -- This is a Go template marker, not a credential.
		entrypointToken = "{{ .EntrypointPath }}"
	)
	if !strings.Contains(indexHTML, importMapToken) || !strings.Contains(indexHTML, entrypointToken) {
		return "", errors.New("index HTML template missing render tokens")
	}
	rendered := strings.ReplaceAll(indexHTML, importMapToken, escapeScriptJSON(data.ImportMap.String()))
	rendered = strings.ReplaceAll(rendered, entrypointToken, html.EscapeString(data.EntrypointPath))
	return rendered, nil
}

func escapeScriptJSON(value string) string {
	value = strings.ReplaceAll(value, "</", "<\\/")
	value = strings.ReplaceAll(value, "<!--", "\\u003c!--")
	return value
}
