// Package s4wave_function is the contract of PureFunctions: TypeScript modules
// that turn protobuf input into protobuf output under a memory and instruction
// budget, and run the same on a Go daemon and in a browser.
package s4wave_function

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// ModuleName is the name a runner gives the function's module.
const ModuleName = "/function.js"

// DefaultMemoryLimitBytes is the heap limit of a budget that sets none.
const DefaultMemoryLimitBytes = 32 << 20

// DefaultInterruptChecks is the interrupt check limit of a budget that sets
// none, about ten million operations.
const DefaultInterruptChecks = 1000

// ErrModuleRequired is returned when a PureFunction has no compiled module.
var ErrModuleRequired = errors.New("the function has no compiled module")

// ErrTypeRequired is returned when a PureFunction names no input or output
// type.
var ErrTypeRequired = errors.New("the function needs an input and an output type")

// GetMemoryLimit returns the heap limit in bytes, applying the default.
func (b *Budget) GetMemoryLimit() uint32 {
	if limit := b.GetMemoryLimitBytes(); limit != 0 {
		return limit
	}
	return DefaultMemoryLimitBytes
}

// GetInterruptLimit returns the interrupt check limit, applying the default.
func (b *Budget) GetInterruptLimit() uint32 {
	if checks := b.GetInterruptChecks(); checks != 0 {
		return checks
	}
	return DefaultInterruptChecks
}

// Validate checks that the function has a module and names its types.
func (f *PureFunction) Validate() error {
	if f.GetModule() == "" {
		return ErrModuleRequired
	}
	if f.GetInputType() == "" || f.GetOutputType() == "" {
		return ErrTypeRequired
	}
	return nil
}

// CreatePureFunction creates the World object key as a function/pure holding
// fn.
func CreatePureFunction(ctx context.Context, ws world.WorldState, key string, fn *PureFunction) error {
	// Check the function before it is stored.
	if err := fn.Validate(); err != nil {
		return err
	}

	// Create the object holding the function and set its type.
	obj, _, err := world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
		bcs.SetBlock(fn, true)
		return nil
	})
	world.ReleaseObjectState(obj)
	if err != nil {
		return errors.Wrapf(err, "create function %q", key)
	}
	return world_types.SetObjectType(ctx, ws, key, PureFunctionTypeID)
}

// SetPureFunction stores fn at key, creating the function/pure object when it
// is missing. An existing function is changed in place, so the edges that
// reach it remain.
func SetPureFunction(ctx context.Context, ws world.WorldState, key string, fn *PureFunction) error {
	// Check the function before it is stored.
	if err := fn.Validate(); err != nil {
		return err
	}

	// Create the object when it is missing.
	obj, found, err := ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return errors.Wrapf(err, "look up function %q", key)
	}
	if !found {
		return CreatePureFunction(ctx, ws, key, fn)
	}

	// Refuse an object of another type.
	typeID, err := world_types.GetObjectType(ctx, ws, key)
	if err != nil {
		return errors.Wrapf(err, "read the type of %q", key)
	}
	if typeID != PureFunctionTypeID {
		return errors.Errorf("%q is a %q object, not a %s", key, typeID, PureFunctionTypeID)
	}

	// Replace the function in the object's root block.
	_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(fn, true)
		return nil
	})
	return errors.Wrapf(err, "set function %q", key)
}

// ReadPureFunction reads the PureFunction key.
func ReadPureFunction(ctx context.Context, ws world.WorldState, key string) (*PureFunction, error) {
	// Look up the function's object.
	obj, err := world.MustGetObject(ctx, ws, key)
	if err != nil {
		return nil, err
	}
	defer world.ReleaseObjectState(obj)

	// Decode the function from the object's root block.
	var fn *PureFunction
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		fn, err = block.UnmarshalBlock[PureFunction](ctx, bcs)
		return err
	})
	if err != nil {
		return nil, errors.Wrapf(err, "read function %q", key)
	}
	if fn == nil {
		return nil, errors.Wrapf(ErrModuleRequired, "read function %q", key)
	}
	return fn, nil
}
