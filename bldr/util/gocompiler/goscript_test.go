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
