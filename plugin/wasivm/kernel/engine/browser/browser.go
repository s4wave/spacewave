//go:build js

// Package browser runs kernel guests on the browser's WebAssembly API.
//
// A guest must be transformed with Binaryen's Asyncify pass (wasm-opt
// --asyncify). A WebAssembly import must return before the guest continues,
// but a host call compiled to TypeScript may need to await. The engine
// unwinds the guest's call stack into a buffer, awaits the call, and rewinds
// the guest to the same import. A copy of that buffer is a snapshot: rewinding
// from it again returns from the import a second time, as fork, setjmp and
// longjmp need.
//
// GoScript replaces this package with its TypeScript implementation in the
// repository's gs directory. These declarations give the kernel the package's
// API to compile against. Compiled by Go, the engine reports errUnsupported.
package browser

import (
	"context"

	"github.com/pkg/errors"

	"github.com/s4wave/spacewave/plugin/wasivm/kernel/engine"
)

// errUnsupported is the error of every operation outside GoScript.
var errUnsupported = errors.New("the browser engine runs only when compiled with GoScript")

// Runtime compiles guests with WebAssembly.compile.
type Runtime struct{}

// New creates a Runtime.
func New() *Runtime {
	return &Runtime{}
}

// Define provides fns to guests as the imports of module.
func (r *Runtime) Define(ctx context.Context, module string, fns map[string]engine.HostFunction) error {
	return errUnsupported
}

// Compile compiles a WebAssembly binary into a reusable module.
func (r *Runtime) Compile(ctx context.Context, wasm []byte) (engine.Module, error) {
	return nil, errUnsupported
}

// Snapshotter returns the snapshotter of the host call running under ctx, or
// nil when its guest is not transformed with Asyncify.
func (r *Runtime) Snapshotter(ctx context.Context) engine.Snapshotter {
	return nil
}

// Close releases the runtime.
func (r *Runtime) Close(ctx context.Context) error {
	return nil
}

// Module is a compiled WebAssembly.Module.
type Module struct{}

// Instantiate creates an instance of the module named name.
func (m *Module) Instantiate(ctx context.Context, name string) (engine.Instance, error) {
	return nil, errUnsupported
}

// Close releases the compiled module.
func (m *Module) Close(ctx context.Context) error {
	return nil
}

// Instance is a WebAssembly.Instance.
type Instance struct{}

// Memory returns the instance's linear memory.
func (i *Instance) Memory() engine.Memory {
	return nil
}

// ExportedFunction returns an exported function, or nil.
func (i *Instance) ExportedFunction(name string) engine.Function {
	return nil
}

// ExportedGlobal returns an exported global, or nil.
func (i *Instance) ExportedGlobal(name string) engine.Global {
	return nil
}

// Close stops the instance.
func (i *Instance) Close(ctx context.Context) error {
	return nil
}

// Function is an exported guest function. Its calls suspend across
// asynchronous host calls.
type Function struct{}

// Call invokes the function.
func (f *Function) Call(ctx context.Context, args ...uint64) ([]uint64, error) {
	return nil, errUnsupported
}

// _ is a type assertion
var (
	_ engine.Runtime  = (*Runtime)(nil)
	_ engine.Module   = (*Module)(nil)
	_ engine.Instance = (*Instance)(nil)
	_ engine.Function = (*Function)(nil)
)
