package world

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/sirupsen/logrus"
)

// namedEngine is an Engine stand-in told apart by name.
type namedEngine struct {
	Engine
	name string
}

// replacingTestController serves whichever engine its container holds under
// one engine ID.
type replacingTestController struct {
	id      string
	engines *ccontainer.CContainer[*Engine]
}

// GetControllerInfo identifies the test controller.
func (c *replacingTestController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("world/test/replacing", controller.MustParseVersion("0.0.1"), "replacing test engine")
}

// GetWorldEngine waits for an engine to be served.
func (c *replacingTestController) GetWorldEngine(ctx context.Context) (Engine, error) {
	val, err := c.engines.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return *val, nil
}

// AcquireWorldEngine waits for an engine to be served. The stand-in engines
// never close, so release does nothing.
func (c *replacingTestController) AcquireWorldEngine(ctx context.Context) (Engine, func(), error) {
	eng, err := c.GetWorldEngine(ctx)
	if err != nil {
		return nil, nil, err
	}
	return eng, func() {}, nil
}

// WaitWorldEngineReplaced waits until eng is no longer served.
func (c *replacingTestController) WaitWorldEngineReplaced(ctx context.Context, eng Engine) error {
	_, err := c.engines.WaitValueWithValidator(ctx, func(current *Engine) (bool, error) {
		return current == nil || *current != eng, nil
	}, nil)
	return err
}

// HandleDirective resolves lookups of the controller's engine ID.
func (c *replacingTestController) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	if lookup, ok := di.GetDirective().(LookupWorldEngine); ok && lookup.LookupWorldEngineID() == c.id {
		return directive.R(NewWorldEngineResolver(c))
	}
	return nil, nil
}

// Execute waits for the controller to be removed.
func (c *replacingTestController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// Close releases nothing.
func (c *replacingTestController) Close() error { return nil }

// TestBusEngineFollowsReplacedEngine checks that a BusEngine reaches the engine
// a controller serves after replacing its engine within one run.
func TestBusEngineFollowsReplacedEngine(t *testing.T) {
	// Start a fresh bus.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	b, _, err := core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}

	// Serve the first engine from a controller on the bus.
	var first, second Engine = &namedEngine{name: "first"}, &namedEngine{name: "second"}
	ctrl := &replacingTestController{id: "test-engine", engines: ccontainer.NewCContainer(&first)}
	rel, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	// Replace the engine while a caller holds the first one.
	eng := NewBusEngine(ctx, b, "test-engine")
	var seen []string
	err = AccessEngine(ctx, eng, func(ctx context.Context, eng Engine) error {
		// Record the engine. The successor ends the access.
		name := eng.(*namedEngine).name
		seen = append(seen, name)
		if name != "first" {
			return nil
		}

		// Replace the first engine and wait for its access to end.
		ctrl.engines.SetValue(&second)
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[1] != "second" {
		t.Fatalf("accessed engines %v, want [first second]", seen)
	}
}
