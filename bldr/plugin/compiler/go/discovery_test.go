//go:build !js

package bldr_plugin_compiler_go

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

// TestDiscoveryConstructors exercises syntax arity and selective type resolution together.
func TestDiscoveryConstructors(t *testing.T) {
	// Keep all constructor forms in one module load to bound test cost.
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module github.com/s4wave/spacewave\n\ngo 1.26.2\n")
	writeFile(t, root, "bldr/web/bundler/output.go", "package bundler\ntype WebBundlerOutput struct{}\n")
	tests := []struct {
		// name identifies a separate package-scope declaration.
		name string
		// source defines the constructor under test.
		source string
		// parameters includes a variadic tail.
		parameters int
		// variadic identifies optional arguments.
		variadic bool
		// absent indicates a method that must not become a factory.
		absent bool
		// invalid indicates a non-signature object or unsupported required arity.
		invalid bool
	}{
		{name: "empty", source: "func NewFactory() {}"},
		{name: "unnamed", source: "func NewFactory(int) {}", parameters: 1},
		{name: "grouped", source: "func NewFactory(a, b int) {}", parameters: 2, invalid: true},
		{name: "variadic", source: "func NewFactory(...int) {}", parameters: 1, variadic: true},
		{name: "options", source: "func NewFactory(int, ...int) {}", parameters: 2, variadic: true},
		{name: "groupedoptions", source: "func NewFactory(a, b int, rest ...int) {}", parameters: 3, variadic: true, invalid: true},
		{name: "method", source: "type T struct{}\nfunc (T) NewFactory() {}", absent: true},
		{name: "literal", source: "var NewFactory = func(int, ...int) {}", parameters: 2, variadic: true},
		{name: "inferred", source: "func constructor(int) {}\nvar NewFactory = constructor", parameters: 1},
		{name: "alias", source: "type Constructor = func(int)\nvar NewFactory Constructor", invalid: true},
		{name: "named", source: "type Constructor func(int)\nvar NewFactory Constructor", invalid: true},
		{name: "constant", source: "const NewFactory = 1", invalid: true},
		{name: "object", source: "var NewFactory = struct{}{}", invalid: true},
		{name: "typeobject", source: "type NewFactory struct{}", invalid: true},
	}
	var roots []string
	for _, tc := range tests {
		writeFile(t, root, tc.name+"/factory.go", "package "+tc.name+"\n"+tc.source+"\n")
		roots = append(roots, "./"+tc.name)
	}

	// Validate call shapes using the same record consumed by plugin and CLI generation.
	an, err := AnalyzePackages(t.Context(), logrus.NewEntry(logrus.New()), root, roots, nil, "linux", "amd64", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkg := an.GetPackages()["github.com/s4wave/spacewave/"+tc.name]
			if tc.absent {
				if pkg.Factory != nil {
					t.Fatal("method discovered as a package constructor")
				}
				return
			}
			if pkg.Factory == nil {
				t.Fatal("missing constructor")
			}
			_, err := pkg.Factory.NeedsBus(pkg.Path)
			if (err != nil) != tc.invalid {
				t.Fatalf("arity validation: got %v, invalid=%v", err, tc.invalid)
			}
			if pkg.Factory.Parameters != tc.parameters {
				t.Fatalf("parameters: got %d, want %d", pkg.Factory.Parameters, tc.parameters)
			}
			if pkg.Factory.Variadic != tc.variadic {
				t.Fatalf("variadic: got %v, want %v", pkg.Factory.Variadic, tc.variadic)
			}
		})
	}
}

// TestDiscoveryAnnotationTypes preserves aliases, inference, and exact output identity.
func TestDiscoveryAnnotationTypes(t *testing.T) {
	// Place each invalid type in its own package so both scanners can report it.
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module github.com/s4wave/spacewave\n\ngo 1.26.2\n")
	writeFile(t, root, "bldr/web/bundler/output.go", "package bundler\ntype WebBundlerOutput struct{ Path string }\n")
	tests := []struct {
		// name identifies the fixture package.
		name string
		// declaration contains type aliases and the annotated variable.
		declaration string
		// output selects the reference struct instead of a string-underlying value.
		output bool
		// invalid requires ErrUnexpectedVarType from both scanners.
		invalid bool
	}{
		{name: "stringalias", declaration: "type Text = string\nTAG\nvar Value Text"},
		{name: "namedstring", declaration: "type Text string\nTAG\nvar Value Text"},
		{name: "inferredstring", declaration: "TAG\nvar Value = text()\nfunc text() string { return \"\" }"},
		{name: "outputalias", declaration: "type Output = bundler.WebBundlerOutput\nTAG\nvar Value Output", output: true},
		{name: "inferredoutput", declaration: "TAG\nvar Value = output()\nfunc output() bundler.WebBundlerOutput { return bundler.WebBundlerOutput{} }", output: true},
		{name: "namedoutput", declaration: "type Output bundler.WebBundlerOutput\nTAG\nvar Value Output", invalid: true},
		{name: "samespelling", declaration: "type WebBundlerOutput struct{ Path string }\nTAG\nvar Value WebBundlerOutput", invalid: true},
		{name: "pointer", declaration: "TAG\nvar Value = &bundler.WebBundlerOutput{}", invalid: true},
		{name: "number", declaration: "TAG\nvar Value = 1", invalid: true},
	}
	var roots []string
	for _, tc := range tests {
		source := "package " + tc.name + "\nimport \"github.com/s4wave/spacewave/bldr/web/bundler\"\nvar _ bundler.WebBundlerOutput\n"
		source += strings.ReplaceAll(tc.declaration, "TAG", "// bldr:esbuild entry.ts\n// bldr:vite entry.ts")
		writeFile(t, root, tc.name+"/binding.go", source+"\n")
		roots = append(roots, "./"+tc.name)
	}

	// All annotated roots and the reference output package share one typed load.
	an, err := AnalyzePackages(t.Context(), logrus.NewEntry(logrus.New()), root, roots, nil, "js", "wasm", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkgPath := "github.com/s4wave/spacewave/" + tc.name
			an.packagePaths = []string{pkgPath}
			code := an.GetGoCodeFiles()
			esbuild, esbuildErr := an.FindEsbuildVariables(code)
			vite, viteErr := an.FindViteVariables(code)
			if tc.invalid {
				if !errors.Is(esbuildErr, ErrUnexpectedVarType) {
					t.Fatalf("esbuild accepted invalid type: %v", esbuildErr)
				}
				if !errors.Is(viteErr, ErrUnexpectedVarType) {
					t.Fatalf("vite accepted invalid type: %v", viteErr)
				}
				return
			}
			if esbuildErr != nil {
				t.Fatal(esbuildErr)
			}
			if viteErr != nil {
				t.Fatal(viteErr)
			}
			if len(esbuild[pkgPath]) != 1 {
				t.Fatalf("missing esbuild binding: %v", esbuild)
			}
			if len(vite[pkgPath]) != 1 {
				t.Fatalf("missing vite binding: %v", vite)
			}
			if (esbuild[pkgPath]["Value"].EsbuildVarType == EsbuildVarType_EsbuildVarType_WEB_BUNDLER_OUTPUT) != tc.output {
				t.Fatal("esbuild output identity changed")
			}
			if (vite[pkgPath]["Value"].ViteVarType == ViteVarType_ViteVarType_WEB_BUNDLER_OUTPUT) != tc.output {
				t.Fatal("vite output identity changed")
			}
		})
	}
}

// TestDiscoveryTargetFiles excludes host factories and includes target-tagged dependencies.
func TestDiscoveryTargetFiles(t *testing.T) {
	// Distinct filenames make target selection observable in the watch manifest.
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module github.com/s4wave/spacewave\n\ngo 1.26.2\n")
	writeFile(t, root, "bldr/web/bundler/output.go", "package bundler\ntype WebBundlerOutput struct{}\n")
	writeFile(t, root, "root/common.go", "package root\n")
	writeFile(t, root, "root/native.go", "//go:build !js\n\npackage root\nfunc NewFactory(a, b int) {}\n")
	writeFile(t, root, "root/browser.go", "//go:build js && goscript && bldr_analyze\n\npackage root\nimport _ \"github.com/s4wave/spacewave/child\"\nfunc NewFactory() {}\n")
	writeFile(t, root, "child/child.go", "package child\nvar NewFactory = func(int, ...int) {}\n")

	// Imported discovery must use the same target files as explicit roots.
	an, err := AnalyzePackages(t.Context(), logrus.NewEntry(logrus.New()), root, []string{"./root"}, []string{"goscript"}, "js", "wasm", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(an.controllerFactories) != 2 {
		t.Fatalf("factories: got %d, want 2", len(an.controllerFactories))
	}
	files := an.GetProgramSourceFiles()["github.com/s4wave/spacewave/root"]
	if slices.ContainsFunc(files, func(file string) bool { return strings.HasSuffix(file, "/native.go") }) {
		t.Fatal("source manifest included a host-only file")
	}
	if !slices.ContainsFunc(files, func(file string) bool { return strings.HasSuffix(file, "/browser.go") }) {
		t.Fatal("source manifest omitted the selected browser file")
	}
	if len(an.GetGoCodeFiles()) != 1 {
		t.Fatal("imported discovery expanded annotation roots")
	}
}
