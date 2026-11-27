package plugin_space_runtime

import (
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/directive"
	backoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	process_binding "github.com/s4wave/spacewave/core/plugin/process"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// ControllerID is the controller ID.
const ControllerID = "plugin/space/runtime"

// Version is the version of this controller.
var Version = controller.MustParseVersion("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "runs the shared plugin runtime of one Space"

// Controller runs the plugin runtime of one Space on a child bus of the daemon
// bus. Contents mounts acquire it with an ExecController directive, so mounts
// with equal configs share one runtime, and it stops after the last mount
// releases its reference.
//
// The generation keeps loaded only the plugins some holder demands. Host
// changes reconcile within the running scheduler. A composition failure is
// published to every mount and retried with backoff.
type Controller struct {
	*bus.BusController[*Config]

	// bcast guards gen, err, prefixes, demands, and accountPluginIDs.
	bcast broadcast.Broadcast
	// gen is the running generation, or nil while none runs.
	gen *Generation
	// err is the last startup or runtime failure. It is cleared when a
	// generation starts.
	err error
	// prefixes is the set of attached RPC service ID prefixes bound by mounts.
	prefixes map[string]struct{}
	// demands is the set of holder plugin demands.
	demands map[*PluginDemand]struct{}
	// accountPluginIDs is the account plugin list the holders last reported.
	accountPluginIDs []string
	// bindingRegistry reports binding writes and orphan deletions to Resource watches.
	bindingRegistry *process_binding.BindingRegistry
}

// NewFactory constructs the component factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{
				BusController: base,
				prefixes:      make(map[string]struct{}),
				demands:       make(map[*PluginDemand]struct{}),
			}, nil
		},
	)
}

// StartControllerWithConfig acquires the shared runtime for conf on b.
//
// Mounts with equal configs share one runtime. It returns once the controller
// is constructed, before its first generation starts. The runtime loads no
// Space plugin until a holder adds a PluginDemand, and it stops after the last
// returned reference is released.
func StartControllerWithConfig(
	ctx context.Context,
	b bus.Bus,
	conf *Config,
) (*Controller, directive.Reference, error) {
	ctrl, _, ref, err := loader.WaitExecControllerRunningTyped[*Controller](
		ctx,
		b,
		loader.NewExecController(NewFactory(b), conf),
		nil,
	)
	return ctrl, ref, err
}

// SetBindingRegistry connects this shared runtime to its Resource root's
// binding watch without retaining another runtime reference.
func (c *Controller) SetBindingRegistry(registry *process_binding.BindingRegistry) {
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		c.bindingRegistry = registry
	})
}

// GetGeneration returns the running generation and a channel closed when the
// runtime state or a process binding of the Space changes.
//
// The generation is nil while none runs. err is the last startup or runtime
// failure while the controller waits to retry. The channel still reports the
// retry.
func (c *Controller) GetGeneration() (*Generation, <-chan struct{}, error) {
	// Capture the Space generation and its next change notification together.
	var gen *Generation
	var waitCh <-chan struct{}
	var err error
	c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		gen, err = c.gen, c.err
		waitCh = getWaitCh()
	})

	return gen, waitCh, err
}

// GetAccountPluginIDs returns the account plugin list of the runtime.
func (c *Controller) GetAccountPluginIDs() []string {
	var ids []string
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ids = c.accountPluginIDs
	})
	return ids
}

// SetAccountPluginIDs replaces the account plugin list, which loads alongside
// the plugins the Space lists, and wakes every mount and the running
// generation. Every holder of the runtime reports the same account's list.
func (c *Controller) SetAccountPluginIDs(ids []string) {
	// Store the canonical list and wake the mounts when it changed.
	ids = canonicalPluginIDs(ids)
	var gen *Generation
	var changed bool
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if slices.Equal(c.accountPluginIDs, ids) {
			return
		}
		c.accountPluginIDs, gen, changed = ids, c.gen, true
		broadcast()
	})
	if changed && gen != nil {
		gen.GetSpaceController().NotifyChanged()
	}
}

// ReserveServicePrefix reserves an attached RPC service ID prefix for every
// mount of the runtime.
//
// The first binding stays authoritative: a second reservation of a bound
// prefix fails. Call release once when the binding ends.
func (c *Controller) ReserveServicePrefix(prefix string) (release func(), err error) {
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if _, ok := c.prefixes[prefix]; ok {
			err = errors.Errorf("service ID prefix %q is already bound", prefix)
			return
		}
		c.prefixes[prefix] = struct{}{}
	})
	if err != nil {
		return nil, err
	}
	return func() {
		c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			delete(c.prefixes, prefix)
		})
	}, nil
}

// NotifyProcessBindingsChanged wakes every mount and the running plugin/space
// controller after a process binding of the Space changed.
func (c *Controller) NotifyProcessBindingsChanged() {
	// Snapshot the Space generation and registry while waking runtime mounts.
	var gen *Generation
	var registry *process_binding.BindingRegistry
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		gen = c.gen
		registry = c.bindingRegistry
		broadcast()
	})

	// Notify Resource watches about the changed process bindings.
	if registry != nil {
		registry.NotifyChanged()
	}

	// Reconcile process bindings within the running Space generation.
	if gen != nil {
		gen.GetSpaceController().NotifyChanged()
	}
}

// HandleDirective holds ObjectType and operation lookups for this Space's
// engine on the daemon bus while its plugins register. A caller outside the
// Space, such as a session applying an operation, then sees a Space plugin's
// capabilities instead of finding none while the plugin starts.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	// Match only lookups for this Space's engine.
	var engineID string
	switch d := di.GetDirective().(type) {
	case objecttype.LookupObjectType:
		engineID = d.LookupObjectTypeEngineID()
	case world.LookupWorldOp:
		engineID = d.LookupWorldOpEngineID()
	}
	if engineID == "" || engineID != c.GetConfig().GetSpace().GetEngineId() {
		return nil, nil
	}
	return directive.R(directive.NewFuncResolver(c.holdWhileRegistering), nil)
}

// holdWhileRegistering keeps handler non-idle while a generation is starting
// or its desired plugins are still registering. A failed start releases the
// hold until the retry.
func (c *Controller) holdWhileRegistering(ctx context.Context, handler directive.ResolverHandler) error {
	for {
		// Read the generation, then its plugins' registration state.
		gen, genCh, err := c.GetGeneration()
		pending := gen == nil && err == nil
		var spaceCh <-chan struct{}
		if gen != nil {
			pending, spaceCh = gen.GetSpaceController().GetRegistrationPendingAndWaitCh()
		}

		// Report the registration state and wait for the generation or
		// registration state to change.
		handler.MarkIdle(!pending)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-genCh:
		case <-spaceCh:
		}
	}
}

// Execute runs generations until the last reference releases the controller.
//
// It never returns an error: the loader would publish it to every mount and
// construct a new controller, dropping the reserved prefixes.
func (c *Controller) Execute(ctx context.Context) error {
	retry := backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(time.Second),
		backoff.WithMaxInterval(30*time.Second),
		backoff.WithMaxElapsedTime(0),
	)
	for {
		err := c.runGeneration(ctx, retry)
		if ctx.Err() != nil {
			return nil
		}

		// Wait out the backoff before the next attempt.
		delay := retry.NextBackOff()
		c.GetLogger().
			WithError(err).
			WithField("backoff-duration", delay.String()).
			Warn("Space plugin runtime failed")
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// runGeneration starts one generation, publishes it, and returns the reason it
// ended. A generation that starts resets retry.
func (c *Controller) runGeneration(ctx context.Context, retry backoff.BackOff) error {
	// Start the generation, publishing a startup failure to every mount.
	gen, err := startGeneration(
		ctx,
		c.GetBus(),
		c.GetLogger(),
		c.GetConfig(),
		c.IsPluginDemanded,
		c.GetAccountPluginIDs,
		c.GetReplayPluginIDs,
		c.NotifyProcessBindingsChanged,
	)
	if err != nil {
		c.publish(nil, err)
		return err
	}

	// Publish the generation, then reconcile the demands that changed before
	// updateDemands could see it.
	c.publish(gen, nil)
	gen.GetSpaceController().NotifyChanged()
	retry.Reset()

	// Withdraw the generation before stopping it so mounts stop installing routes.
	err = gen.wait(ctx)
	c.publish(nil, err)
	gen.release()
	return err
}

// publish replaces the runtime state and wakes every mount.
func (c *Controller) publish(gen *Generation, err error) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.gen, c.err = gen, err
		broadcast()
	})
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
