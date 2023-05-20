package world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
)

// BusEngine uses a directive lookup to access the Engine.
type BusEngine = RefCountEngine

// NewBusEngine constructs a new BusEngine instance.
//
// ctx can be nil to prevent the lookup from occurring until SetContext is called.
func NewBusEngine(ctx context.Context, b bus.Bus, engineID string) *BusEngine {
	return NewRefCountEngine(ctx, true, NewBusEngineResolver(b, engineID))
}

// NewBusEngineResolver constructs a resolver function for a bus engine.
func NewBusEngineResolver(b bus.Bus, engineID string) EngineResolver {
	return func(ctx context.Context, released func()) (*Engine, func(), error) {
		lookupVal, _, lookupRef, err := ExLookupWorldEngine(ctx, b, false, engineID, released)
		if err != nil {
			return nil, nil, err
		}
		return &lookupVal, lookupRef.Release, nil
	}
}

// _ is a type assertion
var _ Engine = ((*BusEngine)(nil))
