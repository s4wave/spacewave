package web_pkg_external

import (
	"path/filepath"

	"github.com/s4wave/spacewave/bldr"
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
	"@aptre/bldr-sdk",
	"@aptre/protobuf-es-lite",
}

// bldrSdkImports are the browser modules of @aptre/bldr-sdk. Consumers import
// any of them by subpath, and the runtime and every plugin must share one
// instance of each so SDK React contexts reach plugin components. The Node-only
// resource/unix-client.ts and the backend-only plugin/host are omitted.
var bldrSdkImports = []string{
	"index.ts",
	"defer.ts",
	"dispose-symbol.ts",
	"plugin.ts",
	"hooks/createResourceContext.tsx",
	"hooks/index.ts",
	"hooks/ResourceDevToolsContext.tsx",
	"hooks/ResourcesContext.tsx",
	"hooks/resourceTransitionState.ts",
	"hooks/useMappedResource.ts",
	"hooks/useResource.tsx",
	"hooks/useResourcesClient.tsx",
	"hooks/useStreamingResource.ts",
	"impl/backend-api.ts",
	"resource/client.ts",
	"resource/index.ts",
	"resource/resource.pb.ts",
	"resource/resource.ts",
	"resource/resource_srpc.pb.ts",
	"resource/server/attached-resource.ts",
	"resource/server/context.ts",
	"resource/server/index.ts",
	"resource/server/mux.ts",
	"resource/server/server.ts",
	"resource/server/tracked-client.ts",
	"resource/server/tracked-resource.ts",
	"state/index.ts",
	"state/state.pb.ts",
	"state/state.ts",
	"state/state_srpc.pb.ts",
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
// served web pkg entry names (servedEntryName in
// bldr/web/bundler/vite/web-pkg-naming.ts): a TypeScript source drops only its
// own extension, and a compiled JavaScript entry drops every known extension,
// including ".pb", to match its export subpath. A consumer that remaps imports
// to /b/pkg/ URLs must derive the same names from this list, not from the
// package's on-disk layout.
var BldrDistWebPkgImports = map[string][]string{
	"react":                   {"index.js", "jsx-runtime.js", "jsx-dev-runtime.js"},
	"react-dom":               {"index.js", "client.js"},
	"@aptre/bldr":             {"index.ts"},
	"@aptre/bldr-react":       {"index.ts"},
	"@aptre/bldr-sdk":         bldrSdkImports,
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
		WebPkgRoot: bldr.ResolveDistSourcePath(bldrDistRoot, "web", "bldr"),
		Imports:    BldrDistWebPkgImports["@aptre/bldr"],
	}, {
		WebPkgId:   "@aptre/bldr-react",
		WebPkgRoot: bldr.ResolveDistSourcePath(bldrDistRoot, "web", "bldr-react"),
		Imports:    BldrDistWebPkgImports["@aptre/bldr-react"],
	}, {
		WebPkgId:   "@aptre/bldr-sdk",
		WebPkgRoot: bldr.ResolveDistSourcePath(bldrDistRoot, "sdk"),
		Imports:    BldrDistWebPkgImports["@aptre/bldr-sdk"],
	}, {
		WebPkgId:   "@aptre/protobuf-es-lite",
		WebPkgRoot: filepath.Join(buildPkgsDir, "node_modules/@aptre/protobuf-es-lite/dist"),
		Imports:    BldrDistWebPkgImports["@aptre/protobuf-es-lite"],
	}}
}
