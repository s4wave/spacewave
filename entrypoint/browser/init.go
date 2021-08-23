//go:build js
// +build js

package main

import (
	"runtime"
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
	wasmInit := js.Global().Get("BLDR_WASM_INIT")
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

const apertureText = `
       d8888                           888
      d88888                           888                             
     d88P888                           888                             
    d88P 888 88888b.   .d88b.  888d888 888888 888  888 888d888 .d88b.  
   d88P  888 888 "88b d8P  Y8b 888P"   888    888  888 888P"  d8P  Y8b 
  d88P   888 888  888 88888888 888     888    888  888 888    88888888 
 d8888888888 888 d88P Y8b.     888     Y88b.  Y88b 888 888    Y8b.     
d88P     888 88888P"   "Y8888  888      "Y888  "Y88888 888     "Y8888  
             888                                                       
             888                                                       
             888                                                       
`

const welcomeBanner = `Welcome to Bldr by Aperture Robotics`

// writeBanner writes the banner to the browser console.
func writeBanner() {
	defer func() {
		_ = recover()
	}()

	// write aperture banner
	js.Global().Get("console").Call(
		"log",
		"%c"+apertureText,
		// "color:red;font-family:system-ui;font-size:4rem;-webkit-text-stroke:1px black;font-weight:bold",
		"color:#ff3838;font-size:0.8em",
	)

	// write text
	js.Global().Get("console").Call(
		"log",
		"%c"+welcomeBanner+", "+runtime.Version()+" "+runtime.GOOS+"/"+runtime.GOARCH,
		"color:#70ffff;font-family:system-ui;font-size:1.82rem;font-weight:bold",
	)
}
