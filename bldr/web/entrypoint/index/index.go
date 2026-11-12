package web_entrypoint_index

import (
	"errors"
	"html"
	"maps"
	"slices"
	"strings"

	"github.com/aperturerobotics/fastjson"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"

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
	// ReactVersion is the version of the React build the import map binds.
	// The shell switches to Development React only when the cached
	// development generation has this version; empty keeps production.
	ReactVersion string
}

// String returns the JSON string representation of ImportMap
func (m ImportMap) String() string {
	// Build a deterministic import map by sorting its keys before encoding.
	var a fastjson.Arena
	obj := a.NewObject()
	imports := a.NewObject()
	keys := make([]string, 0, len(m.Imports))
	for key := range m.Imports {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	// Populate the import object in sorted order and marshal it as JSON.
	for _, key := range keys {
		imports.Set(key, a.NewString(m.Imports[key]))
	}
	obj.Set("imports", imports)
	return string(obj.MarshalTo(nil))
}

// WebPkgFallbackPath serves shared web packages through the service worker,
// which redirects each package to the files its release pins.
const WebPkgFallbackPath = "/b/pkg/"

// WithWebPkgFallback returns a copy of the map that binds the shared web
// package prefix to WebPkgFallbackPath unless the map already binds it.
//
// Firefox ignores every import map after the document's first, so the
// document's own map must resolve the bare package specifiers bundles emit.
// Web views add more specific per-package prefixes where the browser accepts
// additional maps; the longest matching prefix wins.
func (m ImportMap) WithWebPkgFallback() ImportMap {
	imports := maps.Clone(m.Imports)
	if imports == nil {
		imports = make(map[string]string, 1)
	}
	if _, ok := imports[web_pkg.ImportSpecifierPrefix]; !ok {
		imports[web_pkg.ImportSpecifierPrefix] = WebPkgFallbackPath
	}
	return ImportMap{Imports: imports}
}

// RenderIndexHTML renders the embedded browser index shell with the provided data.
func RenderIndexHTML(data IndexData) (string, error) {
	// Define the template markers that receive caller-provided index data.
	const (
		// #nosec G101 -- These are Go template markers, not credentials.
		importMapToken = "{{ .ImportMap.String }}"
		// #nosec G101 -- This is a Go template marker, not a credential.
		entrypointToken = "{{ .EntrypointPath }}"
		// #nosec G101 -- This is a Go template marker, not a credential.
		reactVersionToken = "{{ .ReactVersion }}"
	)

	// Require both template markers before rendering the embedded index.
	if !strings.Contains(indexHTML, importMapToken) || !strings.Contains(indexHTML, entrypointToken) || !strings.Contains(indexHTML, reactVersionToken) {
		return "", errors.New("index HTML template missing render tokens")
	}

	// Escape embedded JSON for script content and the entrypoint for HTML.
	rendered := strings.ReplaceAll(indexHTML, importMapToken, escapeScriptJSON(data.ImportMap.WithWebPkgFallback().String()))
	rendered = strings.ReplaceAll(rendered, entrypointToken, html.EscapeString(data.EntrypointPath))
	rendered = strings.ReplaceAll(rendered, reactVersionToken, html.EscapeString(data.ReactVersion))
	return rendered, nil
}

func escapeScriptJSON(value string) string {
	value = strings.ReplaceAll(value, "</", "<\\/")
	value = strings.ReplaceAll(value, "<!--", "\\u003c!--")
	return value
}
