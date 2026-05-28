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
		WorkDir:         dir,
		OutputPath:      filepath.Join(dir, "out"),
		Packages:        []string{"."},
		BuildFlags:      []string{"-tags=build_type_debug,purego"},
		OverrideDirs:    []string{"./gs"},
		AllDependencies: true,
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
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("argv missing %q:\n%s", want, got)
		}
	}
}
