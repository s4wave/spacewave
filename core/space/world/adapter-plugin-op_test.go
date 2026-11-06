package space_world_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
)

// testOpPlugin is the plugin the World declares handles the mock World
// operation.
const testOpPlugin = "test-plugin"

// runPluginOpThrough writes an operation whose type only a plugin resolves,
// then remounts without the plugin: replay stops at the operation, asks for
// the plugin the World declares with LoadOpPlugins, and continues once the
// plugin's handler registers.
func runPluginOpThrough(t *testing.T) {
	// Resolve the mock World operation only through the plugin, which the
	// World declares.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	f := newAdapterFixtureOn(t, fixtureOpts{opTypes: &space_world.OpTypes{
		Declared: func(_ context.Context, _ world.WorldState, opTypeID string) ([]string, error) {
			if opTypeID == world_mock.MockWorldOpTypeID {
				return []string{testOpPlugin}, nil
			}
			return nil, nil
		},
	}})

	// Write the operation while the plugin runs.
	releasePlugin := startTestOpPlugin(ctx, t, f.tb.Bus)
	eng, engRef := f.start(t)
	err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, "plugin-object", nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		_, _, err = ws.ApplyWorldOp(ctx, world_mock.NewMockWorldOp("plugin-object", "hello"), "")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Remount without the plugin, with a loader that starts it on request.
	releasePlugin()
	loads := make(chan []string, 1)
	releaseLoader, err := f.tb.Bus.AddController(ctx, &testOpPluginLoader{b: f.tb.Bus, engineID: f.engineID, loads: loads}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLoader()
	type mount struct {
		eng *world.BusEngine
		ref directive.Reference
	}
	remounted := make(chan mount, 1)
	go func() {
		eng, ref := f.remount(ctx, t, eng, engRef)
		remounted <- mount{eng: eng, ref: ref}
	}()

	// Replay asks for the plugin and reaches the World after the operation.
	select {
	case ids := <-loads:
		if !slices.Equal(ids, []string{testOpPlugin}) {
			t.Fatalf("replay asked for plugins %q, want %s", ids, testOpPlugin)
		}
	case <-ctx.Done():
		t.Fatal("replay stopped at the plugin's operation without asking for the plugin")
	}
	m := <-remounted
	defer m.ref.Release()
	if err := world.AccessEngine(ctx, m.eng, func(ctx context.Context, eng world.Engine) error {
		return hasObjects(ctx, eng, "plugin-object")
	}); err != nil {
		t.Fatal(err)
	}
}

// startTestOpPlugin registers the plugin's handler for the mock operations
// until the returned release.
func startTestOpPlugin(ctx context.Context, t *testing.T, b bus.Bus) func() {
	t.Helper()
	release, err := b.AddController(ctx, world.NewLookupOpController(testOpPlugin, "", world_mock.LookupMockOp), nil)
	if err != nil {
		t.Fatal(err)
	}
	return release
}

// testOpPluginLoader stands in for a Space resource's operation plugin
// loader: it reports the plugins the first LoadOpPlugins for its engine asks
// for, and starts the test plugin.
type testOpPluginLoader struct {
	// b is the bus the plugin registers on.
	b bus.Bus
	// engineID is the Space World engine.
	engineID string
	// loads receives the plugins of the first request.
	loads chan []string
}

// GetControllerInfo returns information about the controller.
func (l *testOpPluginLoader) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/op-plugin-loader", controller.MustParseVersion("0.0.1"), "test operation plugin loader")
}

// Execute returns immediately.
func (l *testOpPluginLoader) Execute(context.Context) error {
	return nil
}

// HandleDirective starts the test plugin for the engine's first request.
func (l *testOpPluginLoader) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(space_world.LoadOpPlugins)
	if !ok || dir.LoadOpPluginsEngineID() != l.engineID {
		return nil, nil
	}
	select {
	case l.loads <- dir.LoadOpPluginsPluginIDs():
		go func() {
			_, _ = l.b.AddController(context.WithoutCancel(ctx), world.NewLookupOpController(testOpPlugin, "", world_mock.LookupMockOp), nil)
		}()
	default:
	}
	return nil, nil
}

// Close releases nothing.
func (l *testOpPluginLoader) Close() error {
	return nil
}
