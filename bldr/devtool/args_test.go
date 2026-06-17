//go:build !js

package devtool

import "testing"

func TestResolveWebStartModeDefaultsToGoScript(t *testing.T) {
	args := &DevtoolArgs{}

	mode, err := args.resolveWebStartMode()
	if err != nil {
		t.Fatal(err)
	}
	if mode != webStartModeGoScript {
		t.Fatalf("web start mode = %s, want %s", mode, webStartModeGoScript)
	}
}

func TestResolveWebStartModeWasmOverride(t *testing.T) {
	args := &DevtoolArgs{WebUseWasm: true}

	mode, err := args.resolveWebStartMode()
	if err != nil {
		t.Fatal(err)
	}
	if mode != webStartModeWasm {
		t.Fatalf("web start mode = %s, want %s", mode, webStartModeWasm)
	}
}

func TestResolveWebStartModeRejectsConflictingCompilerFlags(t *testing.T) {
	args := &DevtoolArgs{WebUseWasm: true, WebUseGoScript: true}

	if _, err := args.resolveWebStartMode(); err == nil {
		t.Fatal("resolveWebStartMode() error = nil, want conflict error")
	}
}
