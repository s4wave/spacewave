package native

import (
	"context"

	"github.com/tetratelabs/wazero/api"

	"github.com/s4wave/spacewave/plugin/wasivm/kernel/engine"
)

// Function is an exported wazero guest function.
type Function struct {
	// inst is the instance that exports it.
	inst engine.Instance
	// calls are the guest calls in progress on inst.
	calls *callStack
	// name is its export name.
	name string
	// fn is the wazero function.
	fn api.Function
}

// Call invokes the function as a new guest call of its instance.
func (f *Function) Call(ctx context.Context, args ...uint64) ([]uint64, error) {
	return f.calls.call(ctx, f.inst, f.name, f.fn, args, nil, nil)
}

// _ is a type assertion
var _ engine.Function = (*Function)(nil)
