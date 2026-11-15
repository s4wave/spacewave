package space_world

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/world"
)

// handlerWait is replay's wait at an operation whose type no handler resolves
// on this device. While it lasts a LoadOpPlugins asks the Space's plugin
// runtime for the plugins that could handle the type, and a lookup on the bus
// watches for the handler. The body driver's goroutine owns the fields
// outside bcast.
type handlerWait struct {
	// opTypeID is the operation type replay waits for.
	opTypeID string
	// pluginID is the plugin a pinned type names, or empty.
	pluginID string
	// pluginIDs are the plugins asked for.
	pluginIDs []string
	// cancel ends the watch.
	cancel context.CancelFunc
	// bcast guards awaiting and done, and wakes the driver on a change.
	bcast broadcast.Broadcast
	// awaiting are the sorted asked plugins this device has not approved.
	awaiting []string
	// done is set once the watch ended: a handler resolved, or the lookup
	// failed and the next wait starts a new watch.
	done bool
}

// waitForHandler returns the wait of replay at an operation u names, starting
// its watch unless replay already waits for the same type. The wait's Wake
// closes when a handler registers or the approvals it waits for change, so the
// driver steps again with no lock held while it waits.
func (a *Adapter) waitForHandler(u *unresolvedOpError) *sobject.BodyWait {
	// Start the watch of a new wait.
	w := a.handlerWait
	if w == nil || w.ended() || w.opTypeID != u.opTypeID || !slices.Equal(w.pluginIDs, u.pluginIDs) {
		a.endHandlerWait()
		ctx, cancel := context.WithCancel(a.runCtx)
		w = &handlerWait{opTypeID: u.opTypeID, pluginID: u.pluginID, pluginIDs: u.pluginIDs, cancel: cancel}
		a.handlerWait = w
		a.le.WithField("op-type", u.opTypeID).WithField("reason", "no-handler").Debug("replay waits at an operation")
		a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			a.waiting = true
			broadcast()
		})
		if !a.opts.DisableLookup && a.opts.Bus != nil {
			go w.watch(ctx, a.opts.Bus, a.opts.EngineID)
		}
	}

	// Report what the wait waits for now.
	wait := &sobject.SOReplayWait{OpTypeId: w.opTypeID, PluginId: w.pluginID}
	var wake <-chan struct{}
	w.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		if len(w.awaiting) != 0 {
			wait.State = &sobject.SOReplayWait_Approval{Approval: &sobject.SOReplayWaitApproval{PluginIds: slices.Clone(w.awaiting)}}
		} else {
			wait.State = &sobject.SOReplayWait_Handler{Handler: &sobject.SOReplayWaitHandler{}}
		}
		wake = getWaitCh()
	})
	return &sobject.BodyWait{Wait: wait, Wake: wake}
}

// endHandlerWait ends replay's wait at an operation, if any.
func (a *Adapter) endHandlerWait() {
	if a.handlerWait == nil {
		return
	}
	a.handlerWait.cancel()
	a.handlerWait = nil
	a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.waiting = false
		broadcast()
	})
}

// watch holds a LoadOpPlugins for the wait's plugins, recording the ones
// awaiting approval, until a handler for the type resolves on b, the lookup
// fails or ctx ends. It then marks the wait done and wakes the driver, which
// applies the operation or starts a new wait.
func (w *handlerWait) watch(ctx context.Context, b bus.Bus, engineID string) {
	// Ask for the plugins, following which await approval.
	if len(w.pluginIDs) != 0 {
		di, ref, err := b.AddDirective(ctx, NewLoadOpPlugins(engineID, w.pluginIDs), nil)
		if err == nil {
			defer ref.Release()
			defer di.AddStateCallback(func(_ bool, _ []error, vals []directive.AttachedValue) {
				// Collect the plugins each loader reports awaiting approval.
				var awaiting []string
				for _, val := range vals {
					if ids, ok := val.GetValue().(LoadOpPluginsValue); ok {
						awaiting = append(awaiting, ids...)
					}
				}
				slices.Sort(awaiting)
				awaiting = slices.Compact(awaiting)
				w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
					if !slices.Equal(w.awaiting, awaiting) {
						w.awaiting = awaiting
						broadcast()
					}
				})
			})()
		}
	}

	// Wait for a lookup value that resolves the type. Other values, such as
	// the built-in operations, resolve nothing for it.
	_, _, ref, err := bus.ExecWaitValue[world.LookupWorldOpValue](
		ctx,
		b,
		world.NewLookupWorldOp(w.opTypeID, engineID),
		nil,
		nil,
		func(lookup world.LookupWorldOpValue) (bool, error) {
			op, err := lookup(ctx, w.opTypeID)
			return op != nil && err == nil, nil
		},
	)
	if err == nil {
		ref.Release()
	}
	w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		w.done = true
		broadcast()
	})
}

// ended reports whether the wait's watch ended.
func (w *handlerWait) ended() bool {
	var done bool
	w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		done = w.done
	})
	return done
}
