//go:build !js

package entrypoint_browser_bundle

import (
	"os"
	"strings"

	"github.com/pkg/errors"
)

const tinyGoWasmDataView = `class TinyGoWasmDataView extends DataView {
	getBigUint64(offset, littleEndian) {
		return super.getBigUint64(offset >>> 0, littleEndian);
	}

	getUint32(offset, littleEndian) {
		return super.getUint32(offset >>> 0, littleEndian);
	}

	getUint8(offset) {
		return super.getUint8(offset >>> 0);
	}

	setBigUint64(offset, value, littleEndian) {
		super.setBigUint64(offset >>> 0, value, littleEndian);
	}

	setInt32(offset, value, littleEndian) {
		super.setInt32(offset >>> 0, value, littleEndian);
	}

	setUint32(offset, value, littleEndian) {
		super.setUint32(offset >>> 0, value, littleEndian);
	}

	setUint8(offset, value) {
		super.setUint8(offset >>> 0, value);
	}
}

`

// LoadTinyGoWasmExecSource reads and patches TinyGo's browser runtime.
func LoadTinyGoWasmExecSource(wasmExecFile string) (string, error) {
	source, err := os.ReadFile(wasmExecFile)
	if err != nil {
		return "", errors.Wrap(err, "read TinyGo wasm_exec.js")
	}
	return patchTinyGoWasmExecSource(string(source))
}

func patchTinyGoWasmExecSource(source string) (string, error) {
	const dataViewSource = "return new DataView(this._inst.exports.memory.buffer);"
	const dataViewPatched = "return new TinyGoWasmDataView(this._inst.exports.memory.buffer);"
	const sliceSource = "new Uint8Array(this._inst.exports.memory.buffer, array, len)"
	const slicePatched = "new Uint8Array(this._inst.exports.memory.buffer, array >>> 0, len)"
	const stringSource = "new DataView(this._inst.exports.memory.buffer, ptr, len)"
	const stringPatched = "new DataView(this._inst.exports.memory.buffer, ptr >>> 0, len)"

	if strings.Contains(source, dataViewPatched) &&
		strings.Contains(source, slicePatched) &&
		strings.Contains(source, stringPatched) {
		return source, nil
	}
	if !strings.Contains(source, dataViewSource) {
		return "", errors.New("TinyGo wasm_exec.js memory view source is unsupported")
	}
	if !strings.Contains(source, sliceSource) {
		return "", errors.New("TinyGo wasm_exec.js slice source is unsupported")
	}
	if !strings.Contains(source, stringSource) {
		return "", errors.New("TinyGo wasm_exec.js string source is unsupported")
	}

	source = strings.Replace(source, "const mem = () => {", tinyGoWasmDataView+"const mem = () => {", 1)
	source = strings.Replace(source, dataViewSource, dataViewPatched, 1)
	source = strings.Replace(source, sliceSource, slicePatched, 1)
	source = strings.Replace(source, stringSource, stringPatched, 1)
	return source, nil
}
