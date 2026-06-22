package gocompiler

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestExecGoScriptCompileIgnoresExternalCommandEnv(t *testing.T) {
	dir := t.TempDir()
	writeGoScriptModule(t, dir, "example.com/goscriptcmd", map[string]string{
		"main.go": "package goscriptcmd\nconst Value = 1\n",
	})
	t.Setenv("BLDR_GOSCRIPT", filepath.Join(dir, "missing-goscript"))
	err := ExecGoScriptCompile(context.Background(), logrus.NewEntry(logrus.New()), GoScriptCompileOptions{
		WorkDir:    dir,
		OutputPath: filepath.Join(dir, "out"),
		Packages:   []string{"."},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertGoScriptOutputContains(t, filepath.Join(dir, "out"), "example.com/goscriptcmd", "Value")
}

func TestExecGoScriptCompilePreservesBuildFlags(t *testing.T) {
	dir := t.TempDir()
	writeGoScriptModule(t, dir, "example.com/goscripttags", map[string]string{
		"default.go": "package goscripttags\nconst DefaultValue = 1\n",
		"tagged.go":  "//go:build customtag\n\npackage goscripttags\nconst TaggedValue = 2\n",
	})
	err := ExecGoScriptCompile(context.Background(), logrus.NewEntry(logrus.New()), GoScriptCompileOptions{
		WorkDir:    dir,
		OutputPath: filepath.Join(dir, "out"),
		Packages:   []string{"."},
		BuildFlags: []string{"-tags=customtag"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertGoScriptOutputContains(t, filepath.Join(dir, "out"), "example.com/goscripttags", "TaggedValue")
}

func TestExecGoScriptCompileUsesJsWasmTarget(t *testing.T) {
	dir := t.TempDir()
	writeGoScriptModule(t, dir, "example.com/goscripttarget", map[string]string{
		"generic.go": "package goscripttarget\nconst GenericValue = 1\n",
		"js.go":      "//go:build js && wasm\n\npackage goscripttarget\nconst JsWasmValue = 2\n",
		"linux.go":   "//go:build linux\n\npackage goscripttarget\nconst LinuxValue = 3\n",
	})
	err := ExecGoScriptCompile(context.Background(), logrus.NewEntry(logrus.New()), GoScriptCompileOptions{
		WorkDir:    dir,
		OutputPath: filepath.Join(dir, "out"),
		Packages:   []string{"."},
	})
	if err != nil {
		t.Fatal(err)
	}
	output := readGoScriptOutput(t, filepath.Join(dir, "out"), "example.com/goscripttarget")
	for _, want := range []string{"GenericValue", "JsWasmValue"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "LinuxValue") {
		t.Fatalf("output should not include linux-only file:\n%s", output)
	}
}

func TestExecGoScriptCompileUsesCompilerCacheRoot(t *testing.T) {
	dir := t.TempDir()
	cacheRoot := filepath.Join(dir, ".bldr", "cache", "gs")
	writeGoScriptModule(t, dir, "example.com/goscriptcache", map[string]string{
		"main.go": "package goscriptcache\nconst Value = 1\n",
	})
	err := ExecGoScriptCompile(context.Background(), logrus.NewEntry(logrus.New()), GoScriptCompileOptions{
		WorkDir:    dir,
		OutputPath: filepath.Join(dir, "out"),
		CacheRoot:  cacheRoot,
		Packages:   []string{"."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := countGoScriptCacheManifests(t, cacheRoot); got == 0 {
		t.Fatal("compiler cache root did not receive cache manifests")
	}
}

func TestGoScriptCompilerCacheRootFromEnvDefaultsDisabled(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), ".bldr")
	buildPath := filepath.Join(stateRoot, "build", "web", "spacewave-core")
	t.Setenv(GoScriptCompilerCacheRootEnv, "")
	got, err := GoScriptCompilerCacheRootFromEnv(buildPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("cache root = %q, want empty", got)
	}
}

func TestGoScriptCompilerCacheRootFromEnvIgnoresGoScriptCliEnv(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), ".bldr")
	buildPath := filepath.Join(stateRoot, "build", "web", "spacewave-core")
	t.Setenv("GOSCRIPT_COMPILER_CACHE_ROOT", filepath.Join("cache", "gs"))
	got, err := GoScriptCompilerCacheRootFromEnv(buildPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("cache root = %q, want empty", got)
	}
}

func TestGoScriptCompilerCacheRootFromEnvResolvesRelativeUnderBldrStateRoot(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), ".bldr")
	buildPath := filepath.Join(stateRoot, "build", "web", "spacewave-core")
	t.Setenv(GoScriptCompilerCacheRootEnv, filepath.Join("cache", "gs"))
	got, err := GoScriptCompilerCacheRootFromEnv(buildPath)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(stateRoot, "cache", "gs")
	if got != want {
		t.Fatalf("cache root = %q, want %q", got, want)
	}
}

func TestGoScriptCompilerCacheRootFromEnvResolvesRelativeUnderReleaseStateRoot(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), ".bldr-dist")
	buildPath := filepath.Join(stateRoot, "build", "web", "js", "wasm", "spacewave-core")
	t.Setenv(GoScriptCompilerCacheRootEnv, filepath.Join("cache", "gs"))
	got, err := GoScriptCompilerCacheRootFromEnv(buildPath)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(stateRoot, "cache", "gs")
	if got != want {
		t.Fatalf("cache root = %q, want %q", got, want)
	}
}

func TestResolveGoScriptCompilerCacheRootAcceptsAbsoluteRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache", "gs")
	got, err := ResolveGoScriptCompilerCacheRoot(filepath.Join(t.TempDir(), "work"), root)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("cache root = %q, want %q", got, root)
	}
}

func TestResolveGoScriptCompilerCacheRootRejectsEscapingRelativeRoot(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), ".bldr")
	buildPath := filepath.Join(stateRoot, "build", "web", "spacewave-core")
	_, err := ResolveGoScriptCompilerCacheRoot(buildPath, filepath.Join("..", "cache"))
	if err == nil || !strings.Contains(err.Error(), "escapes .bldr state root") {
		t.Fatalf("resolve escaping cache root error = %v, want escape error", err)
	}
}

func TestGoListImportPathPreservesEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/goscriptenv\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := strings.Join([]string{
		"//go:build js && wasm",
		"",
		"package goscriptenv",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "browser.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	importPath, err := GoListImportPath(context.Background(), dir, nil, "GOOS=js", "GOARCH=wasm")
	if err != nil {
		t.Fatal(err)
	}
	if importPath != "example.com/goscriptenv" {
		t.Fatalf("import path = %q, want example.com/goscriptenv", importPath)
	}
}

func writeGoScriptModule(t *testing.T, dir, modulePath string, files map[string]string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+modulePath+"\n\ngo 1.25.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(files[name]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func assertGoScriptOutputContains(t *testing.T, outputRoot, importPath, want string) {
	t.Helper()
	output := readGoScriptOutput(t, outputRoot, importPath)
	if !strings.Contains(output, want) {
		t.Fatalf("output missing %q:\n%s", want, output)
	}
}

func readGoScriptOutput(t *testing.T, outputRoot, importPath string) string {
	t.Helper()
	path := filepath.Join(outputRoot, "@goscript", filepath.FromSlash(importPath), "index.ts")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join(outputRoot, "@goscript", filepath.FromSlash(importPath), "main.gs.ts")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func countGoScriptCacheManifests(t *testing.T, cacheRoot string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(cacheRoot, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == "manifest.json" {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}
