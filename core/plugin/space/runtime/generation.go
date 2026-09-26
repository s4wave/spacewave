package plugin_space_runtime

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	bus_bridge "github.com/aperturerobotics/controllerbus/bus/bridge"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/pkg/errors"
	bldr_core "github.com/s4wave/spacewave/bldr/core"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_default "github.com/s4wave/spacewave/bldr/plugin/host/default"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	plugin_host_static "github.com/s4wave/spacewave/bldr/plugin/host/static"
	"github.com/s4wave/spacewave/bldr/storage"
	storage_controller "github.com/s4wave/spacewave/bldr/storage/controller"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	"github.com/sirupsen/logrus"
)

// Generation is one running instance of the Space plugin runtime: an isolated
// child bus with the Space plugin scheduler and plugin/space controller.
//
// A generation ends when the daemon plugin host set changes, the host watch
// fails, or the runtime stops.
type Generation struct {
	// bus is the isolated child bus.
	bus bus.Bus
	// scheduler schedules the Space plugins on bus.
	scheduler *plugin_host_scheduler.Controller
	// space runs the Space plugins and process bindings on bus.
	space *plugin_space.Controller
	// terminal receives the first error that ends the generation.
	terminal chan error
	// done is closed after the generation is released.
	done chan struct{}
	// releases undoes the startup steps. release runs them in reverse order.
	releases []func()
	// cancel cancels the generation context.
	cancel context.CancelFunc
}

// startGeneration starts one generation of the Space plugin runtime on a child
// bus of parent.
func startGeneration(
	ctx context.Context,
	parent bus.Bus,
	le *logrus.Entry,
	conf *plugin_space.Config,
) (*Generation, error) {
	ctx, cancel := context.WithCancel(ctx)
	g := &Generation{
		terminal: make(chan error, 1),
		done:     make(chan struct{}),
		cancel:   cancel,
	}
	if err := g.start(ctx, parent, le, conf); err != nil {
		g.release()
		return nil, err
	}
	return g, nil
}

// GetBus returns the isolated child bus of the generation.
func (g *Generation) GetBus() bus.Bus {
	return g.bus
}

// GetScheduler returns the Space plugin scheduler.
func (g *Generation) GetScheduler() *plugin_host_scheduler.Controller {
	return g.scheduler
}

// GetSpaceController returns the plugin/space controller.
func (g *Generation) GetSpaceController() *plugin_space.Controller {
	return g.space
}

// Done returns a channel closed after the generation is released.
func (g *Generation) Done() <-chan struct{} {
	return g.done
}

// start builds the child bus, bridges the parent infrastructure into it, and
// starts the scheduler and plugin/space controller.
func (g *Generation) start(
	ctx context.Context,
	parent bus.Bus,
	le *logrus.Entry,
	conf *plugin_space.Config,
) error {
	child, resolver, err := bldr_core.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}
	g.bus = child

	// plugin/space fetches manifests from the parent and, when the Space names a
	// host plugin, loads its plugins through the parent.
	resolver.AddFactory(plugin_host_scheduler.NewFactory(child))
	resolver.AddFactory(volume_rpc_server.NewFactory(child))
	factoryOpts := []plugin_space.FactoryOption{plugin_space.WithManifestSource(parent)}
	if conf.GetHostPluginId() != "" {
		factoryOpts = append(factoryOpts, plugin_space.WithLoadTarget(parent))
	}
	resolver.AddFactory(plugin_space.NewFactory(child, factoryOpts...))
	if err := g.addController(ctx, child, bus_bridge.NewBusBridge(parent, bridgeFilter)); err != nil {
		return err
	}

	// Store plugin state in the daemon host storage.
	if storageID := conf.GetHostStorageId(); storageID != "" {
		if err := g.addStorage(ctx, parent, child, resolver, storageID); err != nil {
			return err
		}
	}

	// Schedule plugins against the daemon plugin hosts present at startup.
	hosts, err := g.watchHosts(ctx, parent)
	if err != nil {
		return err
	}
	if err := g.addController(ctx, child, plugin_host_static.NewController(hosts)); err != nil {
		return err
	}

	// Start the scheduler and expose it to LookupPluginScheduler on the parent so
	// session status sees it.
	schedulerConf := plugin_host_default.NewSchedulerConfig(
		conf.GetSpaceId(),
		conf.GetEngineId(),
		bldr_plugin.PluginVolumeID,
		bldr_plugin.PluginVolumeID,
		conf.GetSessionPeerId(),
		true,
		true,
		true,
	)
	schedulerConf.HostStorageId = conf.GetHostStorageId()
	scheduler, schedulerRelease, err := plugin_host_default.StartPluginSchedulerWithConfig(ctx, child, schedulerConf)
	if err != nil {
		return err
	}
	g.releases = append(g.releases, schedulerRelease)
	g.scheduler = scheduler
	if err := g.addController(ctx, parent, bus_bridge.NewBusBridge(child, schedulerLookupFilter)); err != nil {
		return err
	}

	// Start the Space plugins and process bindings.
	space, _, spaceRef, err := plugin_space.StartControllerWithConfig(ctx, child, conf, nil)
	if err != nil {
		return err
	}
	g.releases = append(g.releases, spaceRef.Release)
	g.space = space
	return nil
}

// addController adds ctrl to b for the generation lifetime.
func (g *Generation) addController(ctx context.Context, b bus.Bus, ctrl controller.Controller) error {
	release, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		return err
	}
	g.releases = append(g.releases, release)
	return nil
}

// addStorage runs the daemon storage storageID on the child bus.
func (g *Generation) addStorage(
	ctx context.Context,
	parent bus.Bus,
	child bus.Bus,
	resolver *static.Resolver,
	storageID string,
) error {
	selected, _, selectedRef, err := bus.ExecWaitValue[storage.LookupStorageValue](
		ctx,
		parent,
		storage.NewLookupStorage(storageID),
		bus.ReturnIfIdle(true),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	if selectedRef == nil {
		return errors.Errorf("host storage %q not found", storageID)
	}
	g.releases = append(g.releases, selectedRef.Release)

	selected.AddFactories(child, resolver)
	return g.addController(ctx, child, storage_controller.BuildStorageController(
		storageID,
		[]storage.Storage{selected},
		controller.NewInfo("space/storage", controller.MustParseVersion("0.0.1"), "Space plugin storage"),
	))
}

// watchHosts watches the daemon plugin host set for the generation lifetime
// and returns its first snapshot. A later change or watch failure ends the
// generation.
func (g *Generation) watchHosts(ctx context.Context, parent bus.Bus) ([]plugin_host.PluginHost, error) {
	hostReady := make(chan error, 1)
	watch := &hostWatch{hostReady: hostReady, reportTerminal: g.reportTerminal}
	_, release, err := bus.ExecCollectValuesWatch(
		ctx,
		parent,
		plugin_host.NewLookupPluginHost(nil),
		true,
		watch.deliver,
		watch.fail,
	)
	if err != nil {
		return nil, err
	}
	g.releases = append(g.releases, release)

	select {
	case err = <-hostReady:
	case <-ctx.Done():
		return nil, context.Canceled
	}
	if err != nil {
		return nil, errors.Wrap(err, "watch daemon plugin hosts")
	}
	return watch.hosts, nil
}

// reportTerminal ends the generation with err. Only the first error is kept.
func (g *Generation) reportTerminal(err error) {
	select {
	case g.terminal <- err:
	default:
	}
}

// wait blocks until the generation ends and returns the reason.
func (g *Generation) wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-g.terminal:
		return err
	}
}

// release undoes the startup steps in reverse order and closes done.
func (g *Generation) release() {
	for _, release := range slices.Backward(g.releases) {
		release()
	}
	g.cancel()
	close(g.done)
}
