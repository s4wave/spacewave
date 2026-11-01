package web_pkg_external

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aperturerobotics/fastjson"
)

// TestBldrDistWebPkgRefsIncludeProtobufEsLiteRuntimeExports checks that the
// served protobuf-es-lite modules are the runtime exports of the installed
// package, so a release that adds or drops a module fails here and not in the
// dev server's web pkg build.
func TestBldrDistWebPkgRefsIncludeProtobufEsLiteRuntimeExports(t *testing.T) {
	// Resolve distribution references from a representative build-pkgs directory.
	const buildPkgsDir = "/build-pkgs"
	refs := GetBldrDistWebPkgRefs(buildPkgsDir, "/bldr")

	// Find the protobuf runtime reference and capture its exported imports.
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

	// Read the installed package's exports.
	data, err := os.ReadFile("../../../../node_modules/@aptre/protobuf-es-lite/package.json")
	if errors.Is(err, fs.ErrNotExist) {
		t.Skip("@aptre/protobuf-es-lite not installed; run bun install")
	}
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := fastjson.ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}

	// Collect each export's module under dist, less the code generator.
	var want []string
	pkg.GetObject("exports").Visit(func(key []byte, v *fastjson.Value) {
		if strings.HasPrefix(string(key), "./protoplugin") {
			return
		}
		target := string(v.GetStringBytes("import"))
		want = append(want, strings.TrimPrefix(target, "./dist/"))
	})

	// Compare the sorted lists.
	got := slices.Sorted(slices.Values(refImports))
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("@aptre/protobuf-es-lite imports=%v, want the installed exports %v", got, want)
	}

	// Require BldrExternal to load the protobuf runtime as a shared package.
	if !slices.Contains(BldrExternal, "@aptre/protobuf-es-lite") {
		t.Fatal("@aptre/protobuf-es-lite missing from BldrExternal")
	}
}

// TestBldrSdkImportsCoverBrowserModules checks that every browser module of
// the SDK is served, so no plugin import falls outside the import map.
func TestBldrSdkImportsCoverBrowserModules(t *testing.T) {
	// Define the SDK source root used to enumerate browser modules.
	const sdkRoot = "../../../sdk"
	var modules []string

	// Collect browser modules while excluding tests and the unsupported client.
	err := filepath.WalkDir(sdkRoot, func(path string, d fs.DirEntry, err error) error {
		// Propagate walk errors and skip directory entries.
		if err != nil || d.IsDir() {
			return err
		}

		// Ignore non-TypeScript files and SDK test sources.
		ext := filepath.Ext(path)
		if ext != ".ts" && ext != ".tsx" || strings.HasSuffix(path, ".test.ts") {
			return nil
		}

		// Convert each supported browser module to an SDK-relative path.
		rel, err := filepath.Rel(sdkRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel != "resource/unix-client.ts" {
			modules = append(modules, rel)
		}
		return nil
	})

	// Fail if the SDK source walk could not enumerate its modules.
	if err != nil {
		t.Fatal(err)
	}

	// Compare sorted SDK files with the import-map inventory.
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

// TestStylexRuntimeExport checks the served runtime against its installed ESM export.
func TestStylexRuntimeExport(t *testing.T) {
	// Read the package contract used by the native compiler and consumers.
	const buildPkgsDir = "../../../.."
	data, err := os.ReadFile(filepath.Join(buildPkgsDir, "node_modules/@stylexjs/stylex/package.json"))
	if errors.Is(err, fs.ErrNotExist) {
		t.Skip("@stylexjs/stylex not installed; run bun install")
	}
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := fastjson.ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	target := strings.TrimPrefix(string(pkg.GetStringBytes("exports", ".", "import")), "./")
	if target == "" {
		t.Fatal("StyleX has no ESM root export")
	}

	// Build the production references and locate their real runtime module.
	for _, ref := range GetBldrDistWebPkgRefs(buildPkgsDir, "/bldr") {
		if ref.GetWebPkgId() != "@stylexjs/stylex" {
			continue
		}
		if !slices.Equal(ref.GetImports(), []string{target}) {
			t.Fatalf("StyleX imports %v, want its ESM root %q", ref.GetImports(), target)
		}
		if _, err := os.Stat(filepath.Join(ref.GetWebPkgRoot(), target)); err != nil {
			t.Fatal(err)
		}
		name, _ := servedEntryName(target)
		if got, want := BldrDistImportMap("/b/pkg/")["@stylexjs/stylex"], "/b/pkg/@stylexjs/stylex/"+name+".mjs"; got != want {
			t.Fatalf("StyleX import map %q, want %q", got, want)
		}
		return
	}
	t.Fatal("StyleX runtime reference is missing")
}
