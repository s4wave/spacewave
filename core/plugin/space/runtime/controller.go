package plugin_space_runtime

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/directive"
	backoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
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
// A change to the daemon plugin host set replaces the running generation. A
// startup or runtime failure is published to every mount and retried with
// backoff.
type Controller struct {
	*bus.BusController[*Config]

	// bcast guards gen, err, and prefixes.
	bcast broadcast.Broadcast
	// gen is the running generation, or nil while none runs.
	gen *Generation
	// err is the last startup or runtime failure. It is cleared when a
	// generation starts or the plugin host set changes.
	err error
	// prefixes is the set of attached RPC service ID prefixes bound by mounts.
	prefixes map[string]struct{}
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
			}, nil
		},
	)
}

// StartControllerWithConfig acquires the shared runtime for conf on b.
//
// Mounts with equal configs share one runtime. It returns once the controller
// is constructed, before its first generation starts. The runtime stops after
// the last returned reference is released.
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

// GetGeneration returns the running generation and a channel closed when the
// runtime state or a process binding of the Space changes.
//
// The generation is nil while none runs. err is the last startup or runtime
// failure while the controller waits to retry; the channel still reports the
// retry.
func (c *Controller) GetGeneration() (*Generation, <-chan struct{}, error) {
	var gen *Generation
	var waitCh <-chan struct{}
	var err error
	c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		gen, err = c.gen, c.err
		waitCh = getWaitCh()
	})
	return gen, waitCh, err
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
	var gen *Generation
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		gen = c.gen
		broadcast()
	})
	if gen != nil {
		gen.GetSpaceController().NotifyChanged()
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
		if errors.Is(err, errPluginHostSetChanged) {
			c.GetLogger().Debug("daemon plugin host set changed, restarting Space runtime")
			continue
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
	gen, err := startGeneration(ctx, c.GetBus(), c.GetLogger(), c.GetConfig().GetSpace())
	if err != nil {
		c.publish(nil, err)
		return err
	}
	c.publish(gen, nil)
	retry.Reset()

	// Withdraw the generation before stopping it so no mount installs a route on
	// a stopping bus. A plugin host set change is a restart, not a failure.
	err = gen.wait(ctx)
	failure := err
	if errors.Is(err, errPluginHostSetChanged) {
		failure = nil
	}
	c.publish(nil, failure)
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
