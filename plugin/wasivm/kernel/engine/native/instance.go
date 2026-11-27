package native

import (
	"context"

	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero/api"

	"github.com/s4wave/spacewave/plugin/wasivm/kernel/engine"
)

// Instance is a running wazero guest and its active guest calls. Only the
// goroutine running the guest uses it.
type Instance struct {
	// mod is the wazero module instance.
	mod api.Module
	// table records the instances of the runtime that instantiated it.
	table *instanceTable
	// calls are the guest calls in progress.
	calls *callStack
}

// frame returns the innermost guest call, or nil.
func (i *Instance) frame() *frame {
	return i.calls.top()
}

// Memory returns the instance's linear memory, or nil when the guest has
// none.
func (i *Instance) Memory() engine.Memory {
	mem := i.mod.Memory()
	if mem == nil {
		return nil
	}
	return &Memory{mem: mem}
}

// ExportedFunction returns an exported function, or nil when the guest does
// not export it.
func (i *Instance) ExportedFunction(name string) engine.Function {
	fn := i.mod.ExportedFunction(name)
	if fn == nil {
		return nil
	}
	return &Function{inst: i, calls: i.calls, name: name, fn: fn}
}

// ExportedGlobal returns an exported global, or nil when the guest does not
// export it.
func (i *Instance) ExportedGlobal(name string) engine.Global {
	g := i.mod.ExportedGlobal(name)
	if g == nil {
		return nil
	}
	return &Global{g: g}
}

// resume rewinds a new outermost guest call of the instance to the unwound
// stack of c, whose host call then returns results. The instance is a fork of
// the one c was taken in.
func (i *Instance) resume(ctx context.Context, c *capture, results []uint64) ([]uint64, error) {
	if i.calls.unwinder == nil || c.depth != len(i.calls.frames) {
		return nil, errors.New("resume outside a fork of the snapshot's instance")
	}
	fn := i.mod.ExportedFunction(c.name)
	if fn == nil {
		return nil, errors.Errorf("resume of missing export %s", c.name)
	}
	return i.calls.call(ctx, i, c.name, fn, c.args, c.stack, results)
}

// Close stops the instance and releases it.
func (i *Instance) Close(ctx context.Context) error {
	i.table.remove(i.mod)
	return i.mod.Close(ctx)
}

// _ is a type assertion
var _ engine.Instance = (*Instance)(nil)
