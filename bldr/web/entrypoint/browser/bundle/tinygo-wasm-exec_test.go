//go:build !js

package entrypoint_browser_bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
)

func TestPatchTinyGoWasmExecSourceNormalizesWasmPointers(t *testing.T) {
	source := `
	const mem = () => {
		return new DataView(this._inst.exports.memory.buffer);
	}
	const loadSlice = (array, len, cap) => {
		return new Uint8Array(this._inst.exports.memory.buffer, array, len);
	}
	const loadString = (ptr, len) => {
		return decoder.decode(new DataView(this._inst.exports.memory.buffer, ptr, len));
	}
	`

	patched, err := patchTinyGoWasmExecSource(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"new TinyGoWasmDataView(this._inst.exports.memory.buffer)",
		"new Uint8Array(this._inst.exports.memory.buffer, array >>> 0, len)",
		"new DataView(this._inst.exports.memory.buffer, ptr >>> 0, len)",
	} {
		if !strings.Contains(patched, want) {
			t.Fatalf("patched TinyGo wasm_exec.js is missing %q:\n%s", want, patched)
		}
	}
}

func TestApplyTinyGoWasmExecPatchesInjectedSource(t *testing.T) {
	dir := t.TempDir()
	wasmExecPath := filepath.Join(dir, "wasm_exec.js")
	entrypointPath := filepath.Join(dir, "entrypoint.js")
	if err := os.WriteFile(wasmExecPath, []byte(`
	const mem = () => {
		return new DataView(this._inst.exports.memory.buffer);
	}
	const loadSlice = (array, len, cap) => {
		return new Uint8Array(this._inst.exports.memory.buffer, array, len);
	}
	const loadString = (ptr, len) => {
		return decoder.decode(new DataView(this._inst.exports.memory.buffer, ptr, len));
	}
	globalThis.tinyGoMemory = mem
	globalThis.tinyGoLoadSlice = loadSlice
	globalThis.tinyGoLoadString = loadString
	globalThis.Go = class {}
	`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entrypointPath, []byte("console.log(Go)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := esbuild.BuildOptions{
		AbsWorkingDir: dir,
		Bundle:        true,
		EntryPoints:   []string{entrypointPath},
		Inject:        []string{wasmExecPath},
		Write:         false,
	}
	ApplyTinyGoWasmExecPatches(&opts, wasmExecPath)
	result := esbuild.Build(opts)
	if len(result.Errors) != 0 {
		t.Fatalf("esbuild errors: %v", result.Errors)
	}
	if len(result.OutputFiles) != 1 {
		t.Fatalf("output files = %d, want 1", len(result.OutputFiles))
	}
	output := string(result.OutputFiles[0].Contents)
	if !strings.Contains(output, "array >>> 0") {
		t.Fatalf("injected TinyGo runtime was not patched:\n%s", output)
	}
}
