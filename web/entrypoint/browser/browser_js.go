//go:build js
// +build js

package web_entrypoint_browser

import (
	"errors"
	"syscall/js"

	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/blang/semver"
)

// ControllerID is the browser runtime controller ID.
const ControllerID = "bldr/web/entrypoint/browser"

// Version is the version of the runtime implementation.
var Version = semver.MustParse("0.0.1")

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
