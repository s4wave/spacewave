package device_flowgraph

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
)

// configRecorder stands in for the ConfigSet controller. It runs every entry
// of an ApplyConfigSet directive until the directive is released, and records
// the entries that run.
type configRecorder struct {
	// bcast guards held and wakes waiters when it changes.
	bcast broadcast.Broadcast
	// held contains the ConfigSet of each live directive by hold ID.
	held map[int]configset.ConfigSet
	// nextHold is the ID of the next hold.
	nextHold int
}

// newConfigRecorder constructs a configRecorder.
func newConfigRecorder() *configRecorder {
	return &configRecorder{held: make(map[int]configset.ConfigSet)}
}

// GetControllerInfo returns information about the controller.
func (r *configRecorder) GetControllerInfo() *controller.Info {
	return controller.NewInfo("spacewave/flowgraph/config-recorder", controller.MustParseVersion("0.0.1"), "records applied ConfigSets")
}

// Execute returns immediately, since the controller only handles directives.
func (r *configRecorder) Execute(context.Context) error {
	return nil
}

// Close releases the controller.
func (r *configRecorder) Close() error {
	return nil
}

// HandleDirective runs each ApplyConfigSet until its directive is released.
func (r *configRecorder) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(configset.ApplyConfigSet)
	if !ok {
		return nil, nil
	}
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		// Record the set while the directive lives.
		set := dir.GetApplyConfigSet()
		id := r.hold(set)
		defer r.release(id)

		// Report each entry as running.
		for key, conf := range set {
			handler.AddValue(configset.State(&runningEntry{runner: r, key: key, conf: conf}))
		}
		<-ctx.Done()
		return nil
	}), nil)
}

// hold records set as applied and returns its hold ID.
func (r *configRecorder) hold(set configset.ConfigSet) int {
	var id int
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.nextHold++
		id = r.nextHold
		r.held[id] = set
		broadcast()
	})
	return id
}

// release removes the set with the hold ID from the applied entries.
func (r *configRecorder) release(id int) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		delete(r.held, id)
		broadcast()
	})
}

// WaitApplied waits until the entries that run satisfy ready, then returns
// them by ConfigSet key.
func (r *configRecorder) WaitApplied(ctx context.Context, ready func(map[string]config.Config) bool) (map[string]config.Config, error) {
	var applied map[string]config.Config
	err := r.bcast.Wait(ctx, func(func(), func() <-chan struct{}) (bool, error) {
		applied = make(map[string]config.Config)
		for _, set := range r.held {
			for key, conf := range set {
				applied[key] = conf.GetConfig()
			}
		}
		return ready(applied), nil
	})
	return applied, err
}

// runningEntry is the state of an entry that runs.
type runningEntry struct {
	runner controller.Controller
	key    string
	conf   configset.ControllerConfig
}

// GetId returns the entry's ConfigSet key.
func (e *runningEntry) GetId() string {
	return e.key
}

// GetControllerConfig returns the entry's config.
func (e *runningEntry) GetControllerConfig() configset.ControllerConfig {
	return e.conf
}

// GetController returns the controller that runs the entry.
func (e *runningEntry) GetController() controller.Controller {
	return e.runner
}

// GetError returns nil, since every entry runs.
func (e *runningEntry) GetError() error {
	return nil
}
