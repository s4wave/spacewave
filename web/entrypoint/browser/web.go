//go:build js
// +build js

package web_entrypoint_browser

import (
	"errors"
	"syscall/js"

	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
)

// ReadInitMessage reads the bldr init message from the global.
//
// configured by runtime-wasm.ts
func ReadInitMessage() (*web_runtime.WebRuntimeHostInit, error) {
	wasmInit := js.Global().Get("BLDR_INIT")
	if wasmInit.IsUndefined() {
		return nil, errors.New("init information was not defined")
	}
	bin := make([]byte, wasmInit.Length())
	js.CopyBytesToGo(bin, wasmInit)
	v := &web_runtime.WebRuntimeHostInit{}
	if err := v.UnmarshalVT(bin); err != nil {
		return nil, err
	}
	return v, nil
}
