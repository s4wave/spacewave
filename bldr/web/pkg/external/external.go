package web_pkg_external

import (
	"path/filepath"

	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
)

// BldrExternal are package prefixes bundled externally for all bldr components.
// Bundler call sites externalize both the package root and subpath imports such
// as @aptre/protobuf-es-lite/service-type.
var BldrExternal = []string{
	"react",
	"react-dom",
	"@aptre/bldr",
	"@aptre/bldr-react",
	"@aptre/protobuf-es-lite",
}

var protobufEsLiteDistImports = []string{
	"index.js",
	"message.js",
	"field.js",
	"scalar.js",
	"enum.js",
	"binary.js",
	"json.js",
	"partial.js",
	"proto-int64.js",
	"proto-double.js",
	"type-registry.js",
	"service-type.js",
	"google/protobuf/any.pb.js",
	"google/protobuf/api.pb.js",
	"google/protobuf/duration.pb.js",
	"google/protobuf/empty.pb.js",
	"google/protobuf/source_context.pb.js",
	"google/protobuf/struct.pb.js",
	"google/protobuf/timestamp.pb.js",
	"google/protobuf/type.pb.js",
	"google/protobuf/wrappers.pb.js",
}

// BldrDistWebPkgImports maps each BldrExternal package id to its entry import
// sub-paths relative to that package's web pkg root. These paths define the
// served web pkg entry names: buildWebPkg strips known extensions (including
// ".pb") from each import to derive the served "[name].mjs", so a consumer that
// remaps imports to /b/pkg/ URLs must derive the same names from this list,
// not from the package's on-disk layout (whose dist/ subdir and .pb.js
// filenames differ from the served names).
var BldrDistWebPkgImports = map[string][]string{
	"react":                   {"index.js", "jsx-runtime.js", "jsx-dev-runtime.js"},
	"react-dom":               {"index.js", "client.js"},
	"@aptre/bldr":             {"index.ts"},
	"@aptre/bldr-react":       {"index.ts"},
	"@aptre/protobuf-es-lite": protobufEsLiteDistImports,
}

// GetBldrDistWebPkgRefs returns the web pkg refs for BldrExternal.
func GetBldrDistWebPkgRefs(buildPkgsDir, bldrDistRoot string) []*web_pkg.WebPkgRef {
	return []*web_pkg.WebPkgRef{{
		WebPkgId:   "react",
		WebPkgRoot: filepath.Join(buildPkgsDir, "node_modules/react"),
		Imports:    BldrDistWebPkgImports["react"],
	}, {
		WebPkgId:   "react-dom",
		WebPkgRoot: filepath.Join(buildPkgsDir, "node_modules/react-dom"),
		Imports:    BldrDistWebPkgImports["react-dom"],
	}, {
		WebPkgId:   "@aptre/bldr",
		WebPkgRoot: filepath.Join(bldrDistRoot, "web", "bldr"),
		Imports:    BldrDistWebPkgImports["@aptre/bldr"],
	}, {
		WebPkgId:   "@aptre/bldr-react",
		WebPkgRoot: filepath.Join(bldrDistRoot, "web", "bldr-react"),
		Imports:    BldrDistWebPkgImports["@aptre/bldr-react"],
	}, {
		WebPkgId:   "@aptre/protobuf-es-lite",
		WebPkgRoot: filepath.Join(buildPkgsDir, "node_modules/@aptre/protobuf-es-lite/dist"),
		Imports:    BldrDistWebPkgImports["@aptre/protobuf-es-lite"],
	}}
}
