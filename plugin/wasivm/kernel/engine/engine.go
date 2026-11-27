// Package engine defines the WebAssembly engine the kernel runs guests on.
//
// The kernel compiles and instantiates guest programs only through these
// interfaces, so one kernel runs on wazero natively and on the browser's
// WebAssembly API.
package engine

import "context"

// Runtime compiles guest modules and captures their execution state.
type Runtime interface {
	// Define provides fns to guests as the imports of module. Define each
	// module once, before instantiating a guest that imports it.
	Define(ctx context.Context, module string, fns map[string]HostFunction) error
	// Compile compiles a WebAssembly binary into a reusable module.
	Compile(ctx context.Context, wasm []byte) (Module, error)
	// Snapshotter returns the snapshotter of the host call running under
	// ctx, or nil when the guest cannot capture its state.
	Snapshotter(ctx context.Context) Snapshotter
	// Close releases the runtime and every module it compiled.
	Close(ctx context.Context) error
}

// Module is a compiled guest module.
type Module interface {
	// Instantiate creates an instance of the module named name, unique
	// within the runtime.
	Instantiate(ctx context.Context, name string) (Instance, error)
	// Close releases the compiled module.
	Close(ctx context.Context) error
}

// ValueType is a WebAssembly value type in its binary encoding.
type ValueType = byte

// I32 and I64 are the value types host functions use.
const (
	// I32 is a 32-bit integer, the type of a C int or pointer.
	I32 ValueType = 0x7f
	// I64 is a 64-bit integer.
	I64 ValueType = 0x7e
)

// HostFunction is a host function a guest imports.
type HostFunction struct {
	// Params are the parameter types.
	Params []ValueType
	// Results are the result types.
	Results []ValueType
	// Call runs the function. The stack holds the arguments on entry and
	// the results on return.
	Call func(ctx context.Context, inst Instance, stack []uint64) error
}

// Instance is a running guest module instance.
type Instance interface {
	// Memory returns the instance's linear memory.
	Memory() Memory
	// ExportedFunction returns an exported function, or nil when the guest
	// does not export it.
	ExportedFunction(name string) Function
	// ExportedGlobal returns an exported global, or nil when the guest does
	// not export it.
	ExportedGlobal(name string) Global
	// Close stops the instance and releases it.
	Close(ctx context.Context) error
}

// Function is an exported guest function.
type Function interface {
	// Call invokes the function. Once ctx ends, the guest stops and Call
	// returns an error.
	Call(ctx context.Context, args ...uint64) ([]uint64, error)
}

// Global is an exported guest global.
type Global interface {
	// Get returns the current value.
	Get() uint64
	// Set changes the value. Set has no effect on an immutable global.
	Set(val uint64)
}

// Memory is a guest's linear memory. Each access reports false when it falls
// outside the memory.
type Memory interface {
	// Read returns length bytes at offset. The slice aliases guest memory.
	Read(offset, length uint32) ([]byte, bool)
	// ReadByteAt returns the byte at offset.
	ReadByteAt(offset uint32) (byte, bool)
	// ReadUint32Le returns the little-endian uint32 at offset.
	ReadUint32Le(offset uint32) (uint32, bool)
	// ReadUint64Le returns the little-endian uint64 at offset.
	ReadUint64Le(offset uint32) (uint64, bool)
	// Write copies data to offset.
	Write(offset uint32, data []byte) bool
	// WriteByteAt writes one byte at offset.
	WriteByteAt(offset uint32, val byte) bool
	// WriteUint32Le writes a little-endian uint32 at offset.
	WriteUint32Le(offset uint32, val uint32) bool
	// WriteUint64Le writes a little-endian uint64 at offset.
	WriteUint64Le(offset uint32, val uint64) bool
	// Size returns the memory size in bytes.
	Size() uint32
	// Grow adds pages of 64 KiB and returns the previous size in pages.
	Grow(pages uint32) (uint32, bool)
}

// Snapshotter captures the call stack of a running host call, so the guest
// can later resume as if that call returned again. The kernel builds fork,
// setjmp and longjmp on it.
type Snapshotter interface {
	// Snapshot captures the call stack of the running host call. The stack is
	// complete once the host call returns.
	Snapshot() Snapshot
}

// Snapshot is a captured guest call stack. It holds no memory or globals:
// the caller restores those it needs. A fork of an instance is another
// instance of the same module into which the caller copied the instance's
// memory and globals.
type Snapshot interface {
	// Restore resumes the guest call running under ctx at the capture,
	// returning returnValues from the host function that took it. It does
	// not return. The guest call must be the one the capture was taken in,
	// or the call at the same depth in a fork of its instance.
	Restore(ctx context.Context, returnValues []uint64)
	// Resume runs the captured guest call in inst, a fork of the instance
	// the capture was taken in, as if the host function that took it
	// returned returnValues. It waits for the capture to complete and
	// returns when the guest call does.
	Resume(ctx context.Context, inst Instance, returnValues []uint64) ([]uint64, error)
}
