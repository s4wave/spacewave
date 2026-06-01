package web_pkg_external

import (
	"path/filepath"

	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
)

// BldrExternal are packages that are bundled externally for all bldr components.
var BldrExternal = []string{
	"react",
	"react-dom",
	"@aptre/bldr",
	"@aptre/bldr-react",
	"@aptre/protobuf-es-lite",
	"quickjs-wasi-reactor",
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
		Imports:    protobufEsLiteDistImports,
	}, {
		WebPkgId:   "quickjs-wasi-reactor",
		WebPkgRoot: filepath.Join(buildPkgsDir, "node_modules/quickjs-wasi-reactor/dist"),
		Imports:    []string{"index.js"},
	}}
}
