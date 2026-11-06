package world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
)

// EngineConfig configures a controller that serves a World engine on the bus
// under its engine ID.
type EngineConfig interface {
	config.Config
	// GetEngineId returns the ID the controller serves its engine under.
	GetEngineId() string
}

// StartEngine loads the controller conf configures and returns an Engine that
// resolves conf's engine ID on the bus. The Engine follows each engine the
// controller serves, across the loader's restarts of a failed run and
// replacements within one run, so a caller never holds a retired engine.
//
// StartEngine waits for the first engine and returns the startup error the
// controller reports for it. factory constructs the controller when set;
// otherwise the bus resolves the factory by config ID. ctx bounds the Engine's
// lookups. Release the reference to stop the controller.
//
// The returned Engine implements only Engine. Assert an optional interface of
// the served engine, such as the Space engine's SetRetainedRoot, inside
// AccessEngine.
func StartEngine(
	ctx context.Context,
	b bus.Bus,
	conf EngineConfig,
	factory controller.Factory,
) (*BusEngine, directive.Reference, error) {
	// Require a valid config that names its engine.
	if err := conf.Validate(); err != nil {
		return nil, nil, err
	}
	engineID := conf.GetEngineId()
	if engineID == "" {
		return nil, nil, ErrEmptyEngineID
	}

	// Load the controller, then wait for its first engine.
	var load directive.Directive = resolver.NewLoadControllerWithConfig(conf)
	if factory != nil {
		load = loader.NewExecController(factory, conf)
	}
	_, ref, err := b.AddDirective(ctx, load, nil)
	if err != nil {
		return nil, nil, err
	}
	eng := NewBusEngine(ctx, b, engineID)
	if _, err := eng.GetSeqno(ctx); err != nil {
		ref.Release()
		return nil, nil, err
	}
	return eng, ref, nil
}

// AccessEngine calls cb with the engine that serves eng. When eng resolves its
// engine on demand, as a BusEngine does, cb runs with the current engine under
// a context canceled when that engine is replaced, and runs again with its
// successor, also when cb returns world.ErrEngineRetired from a retired one.
func AccessEngine(ctx context.Context, eng Engine, cb func(ctx context.Context, eng Engine) error) error {
	rc, ok := eng.(*RefCountEngine)
	if !ok {
		return cb(ctx, eng)
	}
	return rc.access(ctx, func(ctx context.Context, val *Engine) error {
		return AccessEngine(ctx, *val, cb)
	})
}
