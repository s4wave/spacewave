package web_pkg_external

import (
	"path/filepath"

	web_entrypoint_index "github.com/aperturerobotics/bldr/web/entrypoint/index"
	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
)

// BldrExternal are packages that are bundled externally for all bldr components.
var BldrExternal = []string{
	"react",
	"react-dom",
	"@aptre/bldr",
	"@aptre/bldr-react",
	"@aptre/protobuf-es-lite",
}

// GetBldrExternalWebPkgRefs returns the web pkg refs for BldrExternal.
func GetBldrDistWebPkgRefs(buildPkgsDir, bldrDistRoot string) []*web_pkg.WebPkgRef {
	return []*web_pkg.WebPkgRef{{
		WebPkgId:   "react",
		WebPkgRoot: filepath.Join(buildPkgsDir, "node_modules/react"),
		Imports:    []string{"index.js", "jsx-runtime.js", "jsx-dev-runtime.js"},
	}, {
		WebPkgId:   "react-dom",
		WebPkgRoot: filepath.Join(buildPkgsDir, "node_modules/react-dom"),
		Imports:    []string{"index.js", "client.js"},
	}, {
		WebPkgId:   "@aptre/bldr",
		WebPkgRoot: filepath.Join(bldrDistRoot, "web", "bldr"),
		Imports:    []string{"index.ts"},
	}, {
		WebPkgId:   "@aptre/bldr-react",
		WebPkgRoot: filepath.Join(bldrDistRoot, "web", "bldr-react"),
		Imports:    []string{"index.ts"},
	}, {
		WebPkgId:   "@aptre/protobuf-es-lite",
		WebPkgRoot: filepath.Join(buildPkgsDir, "node_modules/@aptre/protobuf-es-lite/dist"),
		Imports:    []string{"index.js", "google/protobuf/empty.pb.js", "google/protobuf/timestamp.pb.js"},
	}}
}

// GetBldrDistImportMap returns the import map for BldrExternal.
func GetBldrDistImportMap(pkgsPathPrefix string) web_entrypoint_index.ImportMap {
	return web_entrypoint_index.ImportMap{
		// NOTE: be sure to update the WebPkgs list as well
		Imports: map[string]string{
			"react":                pkgsPathPrefix + "react/index.mjs",
			"react/jsx-runtime":     pkgsPathPrefix + "react/jsx-runtime.mjs",
			"react/jsx-dev-runtime": pkgsPathPrefix + "react/jsx-dev-runtime.mjs",
			"react-dom":            pkgsPathPrefix + "react-dom/index.mjs",
			"react-dom/client":     pkgsPathPrefix + "react-dom/client.mjs",
			"react-dom/test-utils": pkgsPathPrefix + "react-dom/test-utils.mjs",
			"@aptre/bldr":          pkgsPathPrefix + "@aptre/bldr/index.mjs",
			"@aptre/bldr-react":    pkgsPathPrefix + "@aptre/bldr-react/index.mjs",

			"@aptre/protobuf-es-lite":                       pkgsPathPrefix + "@aptre/protobuf-es-lite/index.mjs",
			"@aptre/protobuf-es-lite/google/protobuf/empty":     pkgsPathPrefix + "@aptre/protobuf-es-lite/google/protobuf/empty.pb.mjs",
			"@aptre/protobuf-es-lite/google/protobuf/timestamp": pkgsPathPrefix + "@aptre/protobuf-es-lite/google/protobuf/timestamp.pb.mjs",
		},
	}
}
