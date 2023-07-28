package determine_cjs_exports

import (
	"bytes"
	"embed"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/exp/slices"
)

// DetermineCjsExportsFS contains the contents of the determine-cjs-exports.mjs file.
//
//go:embed determine-cjs-exports.mjs
var DetermineCjsExportsFS embed.FS

// GetDetermineCjsExportsScript returns the contents of the determine-cjs-exports.mjs file.
func GetDetermineCjsExportsScript() string {
	data, _ := DetermineCjsExportsFS.ReadFile("determine-cjs-exports.mjs")
	return string(data)
}

// GetSupportedExtensions returns the list of file extensions determine-cjs-exports supports.
func GetSupportedExtensions() []string {
	return []string{"", ".js", ".ts", ".cjs", ".es", ".json"}
}

// SupportsExtension checks if the extension is supported.
func SupportsExtension(filename string) bool {
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = filename
	}
	ext = strings.TrimPrefix(ext, ".")
	ext = strings.TrimSpace(ext)
	if ext == "" {
		return true
	}
	return slices.Contains(GetSupportedExtensions(), "."+ext)
}

// CjsExportsResult is the result of calling determine-cjs-exports.
type CjsExportsResult struct {
	Reexport      string   `json:"reexport,omitempty"`
	ExportDefault bool     `json:"exportDefault"`
	Exports       []string `json:"exports"`
	Error         string   `json:"error"`
	Stack         string   `json:"stack"`
}

// GenerateRemapExports generates a javascript file which imports and re-exports
// the exports from the commonjs module as an esm module.
func GenerateRemapExports(importPath string, result *CjsExportsResult) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "export * from %q;\n", importPath)
	exports := result.Exports
	// TODO: https://github.com/esm-dev/esm.sh/issues/713
	// if result.ExportDefault && (len(exports) == 0 || slices.Contains(exports, "default")) {
	{
		fmt.Fprintf(&buf, "export { default } from %q;\n", importPath)
	}
	if len(exports) > 0 {
		fmt.Fprintf(&buf, "import __cjs_exports$ from %q;\n", importPath)
		fmt.Fprintf(&buf, "export const { %s } = __cjs_exports$;\n", strings.Join(exports, ", "))
	}
	return buf.String()
}
