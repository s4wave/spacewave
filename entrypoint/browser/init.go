//go:build js
// +build js

package main

import (
	"syscall/js"

	"github.com/aperturerobotics/bldr/runtime/web"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// readInitMessage reads the bldr init message from the global.
//
// configured by bldr/runtime-wasm.ts
func readInitMessage() (*web.WebInitRuntime, error) {
	// take init data from global
	wasmInit := js.Global().Get("BLDR_INIT")
	if wasmInit.IsUndefined() {
		return nil, errors.New("init information was not defined")
	}
	bin := make([]byte, wasmInit.Length())
	js.CopyBytesToGo(bin, wasmInit)
	v := &web.WebInitRuntime{}
	if err := proto.Unmarshal(bin, v); err != nil {
		return nil, err
	}
	return v, nil
}
