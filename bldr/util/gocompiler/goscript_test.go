package gocompiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestExecGoScriptCompilePreservesBuildFlags(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "goscript")
	logPath := filepath.Join(dir, "argv.txt")
	script := strings.Join([]string{
		"#!/bin/sh",
		"printf '%s\\n' \"$@\" > \"$GOSCRIPT_ARGV_LOG\"",
		"exit 0",
		"",
	}, "\n")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(GoScriptCommandEnv, bin)
	t.Setenv("GOSCRIPT_ARGV_LOG", logPath)

	err := ExecGoScriptCompile(context.Background(), logrus.NewEntry(logrus.New()), GoScriptCompileOptions{
		WorkDir:                   dir,
		OutputPath:                filepath.Join(dir, "out"),
		Packages:                  []string{"."},
		BuildFlags:                []string{"-tags=build_type_debug,purego"},
		OverrideDirs:              []string{"./gs"},
		AllDependencies:           true,
		ProtobufTypeScriptBinding: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		"compile\n",
		"--build-flags\n-tags=build_type_debug,purego\n",
		"--gs-path\n./gs\n",
		"--all-dependencies\n",
		"--protobuf-ts-binding\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("argv missing %q:\n%s", want, got)
		}
	}
}

func TestExecGoScriptCompilePreservesEnv(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "goscript")
	logPath := filepath.Join(dir, "env.txt")
	script := strings.Join([]string{
		"#!/bin/sh",
		"printf 'GOOS=%s\\nGOARCH=%s\\n' \"$GOOS\" \"$GOARCH\" > \"$GOSCRIPT_ENV_LOG\"",
		"exit 0",
		"",
	}, "\n")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(GoScriptCommandEnv, bin)
	t.Setenv("GOSCRIPT_ENV_LOG", logPath)

	err := ExecGoScriptCompile(context.Background(), logrus.NewEntry(logrus.New()), GoScriptCompileOptions{
		WorkDir:    dir,
		OutputPath: filepath.Join(dir, "out"),
		Packages:   []string{"."},
		Env:        []string{"GOOS=js", "GOARCH=wasm"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "GOOS=js\nGOARCH=wasm\n"; got != want {
		t.Fatalf("env log = %q, want %q", got, want)
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

func TestNewGoScriptCmdDefaultsToPinnedModule(t *testing.T) {
	t.Setenv(GoScriptCommandEnv, "")

	cmd := newGoScriptCmd(context.Background(), "compile", "--dir", ".")
	gotArgs := strings.Join(cmd.Args, "\n")
	for _, want := range []string{
		"go\n",
		"run\n" + goScriptModule + "\n",
		"compile\n--dir\n.\n",
	} {
		if !strings.Contains(gotArgs+"\n", want) {
			t.Fatalf("argv missing %q:\n%s", want, gotArgs)
		}
	}
	if got := envValue(cmd.Env, "GONOSUMDB"); got != goScriptNoSumDB {
		t.Fatalf("GONOSUMDB = %q, want %q", got, goScriptNoSumDB)
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	var out string
	for _, val := range env {
		if after, ok := strings.CutPrefix(val, prefix); ok {
			out = after
		}
	}
	return out
}
