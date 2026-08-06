//go:build !js

package entrypoint_browser_bundle

import (
	"strings"
	"testing"
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
