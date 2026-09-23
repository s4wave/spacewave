//go:build !js

package bldr_plugin_compiler_js

import (
	"os"
	"path/filepath"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/fsutil"
	frontend "github.com/s4wave/spacewave/bldr/frontend"
	bldr_plugin_compiler "github.com/s4wave/spacewave/bldr/plugin/compiler"
)

// writeFrontendBindings records stable source attachments for asset consumers.
// Explicit bundles retain their entries; obsolete automatic outputs are removed.
func writeFrontendBindings(assetsDir string, bindings map[string]*frontend.Binding, preserve bool) error {
	frontendDir := filepath.Join(assetsDir, bldr_plugin_compiler.ViteAssetSubdir, "b", "fe")
	manifestPath := filepath.Join(frontendDir, ".vite", "manifest.json")
	var arena fastjson.Arena
	var parser fastjson.Parser
	entries := arena.NewObject()
	if preserve {
		data, err := os.ReadFile(manifestPath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if len(data) != 0 {
			entries, err = parser.ParseBytes(data)
			if err != nil {
				return err
			}
			if _, err := entries.Object(); err != nil {
				return err
			}
		}
	} else if err := fsutil.CleanCreateDir(frontendDir); err != nil {
		return err
	}
	for source, binding := range bindings {
		entry := arena.NewObject()
		attachment := arena.NewObject()
		attachment.Set("entrypoint", arena.NewString(binding.GetEntrypoint()))
		entry.Set("frontendBinding", attachment)
		entries.Set(source, entry)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(manifestPath, entries.MarshalTo(nil), 0o644) //nolint:gosec // Fixed path within the compiler-owned asset directory.
}
