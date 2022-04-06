//go:build js
// +build js

package main

import (
	"syscall/js"

	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// readInitMessage reads the bldr init message from the global.
//
// configured by bldr/runtime-wasm.ts
func readInitMessage() (*web_runtime.WebInitRuntime, error) {
	// take init data from global
	wasmInit := js.Global().Get("BLDR_INIT")
	if wasmInit.IsUndefined() {
		return nil, errors.New("init information was not defined")
	}
	bin := make([]byte, wasmInit.Length())
	js.CopyBytesToGo(bin, wasmInit)
	v := &web_runtime.WebInitRuntime{}
	if err := proto.Unmarshal(bin, v); err != nil {
		return nil, err
	}
	return v, nil
}
