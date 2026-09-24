package web_pkg_external

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestBldrDistWebPkgRefsIncludeProtobufEsLiteRuntimeExports(t *testing.T) {
	const buildPkgsDir = "/build-pkgs"
	refs := GetBldrDistWebPkgRefs(buildPkgsDir, "/bldr")

	var refImports []string
	for _, ref := range refs {
		if ref.GetWebPkgId() != "@aptre/protobuf-es-lite" {
			continue
		}
		if got, want := ref.GetWebPkgRoot(), filepath.Join(buildPkgsDir, "node_modules/@aptre/protobuf-es-lite/dist"); got != want {
			t.Fatalf("@aptre/protobuf-es-lite root=%q want %q", got, want)
		}
		refImports = ref.GetImports()
		break
	}
	if len(refImports) == 0 {
		t.Fatal("@aptre/protobuf-es-lite missing from Bldr dist web package refs")
	}

	for _, imp := range []string{
		"index.js",
		"message.js",
		"field.js",
		"scalar.js",
		"enum.js",
		"binary.js",
		"json.js",
		"partial.js",
		"proto-double.js",
		"proto-int64.js",
		"service-type.js",
		"type-registry.js",
		"google/protobuf/any.pb.js",
		"google/protobuf/api.pb.js",
		"google/protobuf/duration.pb.js",
		"google/protobuf/empty.pb.js",
		"google/protobuf/source_context.pb.js",
		"google/protobuf/struct.pb.js",
		"google/protobuf/timestamp.pb.js",
		"google/protobuf/type.pb.js",
		"google/protobuf/wrappers.pb.js",
	} {
		if !slices.Contains(refImports, imp) {
			t.Fatalf("@aptre/protobuf-es-lite imports=%v, want %s", refImports, imp)
		}
	}

	if !slices.Contains(BldrExternal, "@aptre/protobuf-es-lite") {
		t.Fatal("@aptre/protobuf-es-lite missing from BldrExternal")
	}
}

// TestBldrSdkImportsCoverBrowserModules checks that every browser module of
// the SDK is served, so no plugin import falls outside the import map.
func TestBldrSdkImportsCoverBrowserModules(t *testing.T) {
	const sdkRoot = "../../../sdk"
	var modules []string
	err := filepath.WalkDir(sdkRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := filepath.Ext(path)
		if ext != ".ts" && ext != ".tsx" || strings.HasSuffix(path, ".test.ts") {
			return nil
		}
		rel, err := filepath.Rel(sdkRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel != "resource/unix-client.ts" && !strings.HasPrefix(rel, "plugin/host/") {
			modules = append(modules, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	got := slices.Sorted(slices.Values(BldrDistWebPkgImports["@aptre/bldr-sdk"]))
	slices.Sort(modules)
	if !slices.Equal(got, modules) {
		t.Fatalf("@aptre/bldr-sdk imports=%v, want %v", got, modules)
	}
}

func TestBldrExternalProtobufEsLiteCoversServiceTypeSubpath(t *testing.T) {
	if !bldrExternalMatches("@aptre/protobuf-es-lite/service-type") {
		t.Fatal("@aptre/protobuf-es-lite/service-type is not covered by BldrExternal")
	}
}

func bldrExternalMatches(id string) bool {
	for _, pkg := range BldrExternal {
		if id == pkg || strings.HasPrefix(id, pkg+"/") {
			return true
		}
	}
	return false
}
