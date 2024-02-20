package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bytecodealliance/wasmtime-go/v17"
)

func main() {
	// Set up a new engine and store.
	engine := wasmtime.NewEngine()
	store := wasmtime.NewStore(engine)

	// Load the WebAssembly module.
	module, err := wasmtime.NewModuleFromFile(engine, "../demo.wasm")
	check(err)

	linker := wasmtime.NewLinker(engine)
	err = linker.DefineWasi()
	check(err)

	config := wasmtime.NewWasiConfig()
	config.SetArgv([]string{"demo.wasm"})
	config.PreopenDir(".", "/")
	config.PreopenDir("/tmp", "/tmp")
	config.SetStdoutFile("./out.log")
	config.SetStderrFile("./out.log")
	store.SetWasi(config)

	// Instantiate the module with WASI imports.
	instance, err := linker.Instantiate(store, module)
	check(err)

	// exportFuncs := instance.Exports(store)
	// start := exportFuncs[1].Func()
	start := instance.GetFunc(store, "_start")
	if start == nil {
		check(errors.New("function '_start' not found"))
	}

	// Call the "_start" function.
	_, err = start.Call(store)
	if strings.HasSuffix(err.Error(), "with i32 exit status 0") {
		err = nil
	}
	check(err)

	fmt.Println("WebAssembly module executed successfully.")

	f, err := os.Open("out.log")
	check(err)
	_, err = io.Copy(os.Stdout, f)
	check(err)
	_ = f.Close()
}

func check(err error) {
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
