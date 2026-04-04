//go:build !js

package web_pkg_vite

import (
	web_entrypoint_index "github.com/aperturerobotics/bldr/web/entrypoint/index"
)

// BuildImportMapFromEntries assembles an ImportMap from the collected import map
// entries produced by BuildWebPkgsVite. Each entry maps a logical specifier
// (e.g. "react") to a hashed output path (e.g. "/b/pkg/react/index-a1b2c3.mjs").
func BuildImportMapFromEntries(entries []ImportMapEntry) web_entrypoint_index.ImportMap {
	imports := make(map[string]string, len(entries))
	for _, entry := range entries {
		imports[entry.Specifier] = entry.OutputPath
	}
	return web_entrypoint_index.ImportMap{Imports: imports}
}
