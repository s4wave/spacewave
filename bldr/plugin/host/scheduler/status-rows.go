package plugin_host_scheduler

import (
	"context"
	"slices"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// pluginStatusRows holds the scheduler's plugin status rows by
// pluginInstanceKey and projects them into PluginStatusSnapshot values.
//
// Writers change rows under bcast and bump rev; they never build a snapshot.
// Readers build the cloned, sorted snapshot outside bcast, once per revision,
// and every reader of that revision shares it.
//
// Invariants:
//
// A stored row is never mutated: a writer stores a new row in its place.
// A reader can therefore copy row pointers under bcast and clone them after.
//
// A snapshot holds the rows of exactly one revision, and a reader gets the
// snapshot of the revision current when it asks. Revisions only grow, so a
// reader never sees an older snapshot after a newer one.
//
// Watchers wake on each revision and return only a snapshot that differs from
// the one they hold, so they see each published state or a later one.
//
// The zero value is ready to use.
type pluginStatusRows struct {
	// bcast guards the fields below and wakes watchers on each revision.
	bcast broadcast.Broadcast
	// rev counts row changes.
	rev uint64
	// plugins stores live plugin instance states.
	plugins map[string]*bldr_plugin.PluginStatus
	// recovery stores retained Manifest selection and eligibility facts.
	recovery map[string]*bldr_plugin.PluginManifestRecoveryStatus
	// executions stores the running manifest and swap state of each logical
	// plugin instance.
	executions map[string]*bldr_plugin.PluginExecutionStatus
	// snap is the last built snapshot, holding the rows of snapRev.
	snap *bldr_plugin.PluginStatusSnapshot
	// snapRev is the revision snap holds.
	snapRev uint64
	// building is set while a reader builds a snapshot outside bcast.
	building bool
}

// write applies fn to the rows under bcast.
// fn reports whether it changed a row; a change bumps the revision and wakes
// watchers.
func (r *pluginStatusRows) write(fn func() bool) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Create the row maps on first use.
		if r.plugins == nil {
			r.plugins = make(map[string]*bldr_plugin.PluginStatus)
			r.recovery = make(map[string]*bldr_plugin.PluginManifestRecoveryStatus)
			r.executions = make(map[string]*bldr_plugin.PluginExecutionStatus)
		}

		// Apply the change and publish its revision.
		if fn() {
			r.rev++
			broadcast()
		}
	})
}

// recordRecovery replaces the manifest selection facts for one instance.
// The caller transfers the row to the status store and must not mutate it.
func (r *pluginStatusRows) recordRecovery(row *bldr_plugin.PluginManifestRecoveryStatus) {
	key := pluginInstanceKey(row.GetPluginId(), row.GetInstanceKey())
	r.write(func() bool {
		if r.recovery[key].EqualVT(row) {
			return false
		}
		r.recovery[key] = row
		return true
	})
}

// updatePluginExecutionStatus applies apply to a copy of a live plugin
// instance's execution status and stores it. It does nothing once the
// instance's status entry is gone, so a late update cannot revive it.
// apply runs under the status lock and must only edit the row.
func (r *pluginStatusRows) updatePluginExecutionStatus(
	pluginID,
	instanceKey string,
	apply func(status *bldr_plugin.PluginExecutionStatus),
) {
	key := pluginInstanceKey(pluginID, instanceKey)
	r.write(func() bool {
		// Skip an instance whose status entry is gone.
		if r.plugins[key] == nil {
			return false
		}

		// Apply the mutation to a copy of the row and store the copy.
		current := r.executions[key]
		next := current.CloneVT()
		if next == nil {
			next = &bldr_plugin.PluginExecutionStatus{PluginId: pluginID, InstanceKey: instanceKey}
		}
		apply(next)
		if current.EqualVT(next) {
			return false
		}
		r.executions[key] = next
		return true
	})
}

// updatePluginStatus applies one status mutation to a plugin's entry.
func (r *pluginStatusRows) updatePluginStatus(
	pluginID,
	instanceKey string,
	state bldr_plugin.PluginState,
	lastErrorMessage string,
	lastErrorAt *timestamp.Timestamp,
	recordError,
	clearError bool,
) {
	// Apply the status mutation under the status lock.
	key := pluginInstanceKey(pluginID, instanceKey)
	r.write(func() bool {
		// Read the instance's current row.
		current := r.plugins[key]

		// A plain UNKNOWN state removes the instance's rows.
		if state == bldr_plugin.PluginState_PluginState_UNKNOWN && !recordError && !clearError {
			_, hadRecovery := r.recovery[key]
			_, hadExecution := r.executions[key]
			delete(r.plugins, key)
			delete(r.recovery, key)
			delete(r.executions, key)
			return current != nil || hadRecovery || hadExecution
		}

		// Successful completion can race with plugin reference cleanup. If the
		// instance is already gone, clearing metadata should not recreate it.
		if state == bldr_plugin.PluginState_PluginState_UNKNOWN && clearError && current == nil {
			return false
		}

		// Keep the current state and error unless the mutation sets them.
		if state == bldr_plugin.PluginState_PluginState_UNKNOWN {
			state = bldr_plugin.PluginState_PluginState_REQUESTED
			if current != nil {
				state = current.State
			}
		}
		if !recordError {
			if clearError {
				lastErrorMessage = ""
				lastErrorAt = nil
			} else if current != nil {
				lastErrorMessage = current.LastErrorMessage
				lastErrorAt = current.LastErrorAt
			}
		}

		// Store the new row when it differs from the current one.
		next := &bldr_plugin.PluginStatus{
			PluginId:         pluginID,
			InstanceKey:      instanceKey,
			Running:          state == bldr_plugin.PluginState_PluginState_RUNNING,
			State:            state,
			LastErrorMessage: lastErrorMessage,
			LastErrorAt:      lastErrorAt.CloneVT(),
		}
		if current.EqualVT(next) {
			return false
		}
		r.plugins[key] = next
		return true
	})
}

// plugin returns the stored status row for key, or nil.
// The row must not be mutated.
func (r *pluginStatusRows) plugin(key string) *bldr_plugin.PluginStatus {
	var row *bldr_plugin.PluginStatus
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		row = r.plugins[key]
	})
	return row
}

// snapshot returns the snapshot of the current revision and that revision.
// It builds the snapshot outside bcast when no reader has built it yet.
func (r *pluginStatusRows) snapshot() (*bldr_plugin.PluginStatusSnapshot, uint64) {
	for {
		// Return the cached snapshot, wait for a build in flight, or claim the
		// build and copy the current rows.
		var snap *bldr_plugin.PluginStatusSnapshot
		var rev uint64
		var waitCh <-chan struct{}
		var plugins []*bldr_plugin.PluginStatus
		var recovery []*bldr_plugin.PluginManifestRecoveryStatus
		var executions []*bldr_plugin.PluginExecutionStatus
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			// Reuse the snapshot of the current revision.
			rev = r.rev
			if r.snap != nil && r.snapRev == rev {
				snap = r.snap
				return
			}

			// Wait for a build in flight.
			if r.building {
				waitCh = getWaitCh()
				return
			}

			// Claim the build and copy the row pointers.
			r.building = true
			plugins = mapValues(r.plugins)
			recovery = mapValues(r.recovery)
			executions = mapValues(r.executions)
		})
		if snap != nil {
			return snap, rev
		}
		if waitCh != nil {
			<-waitCh
			continue
		}

		// Clone and sort the copied rows without holding bcast.
		built := buildPluginStatusSnapshot(plugins, recovery, executions)

		// Cache the build, reusing an equal snapshot. Builds are serialized and
		// each claims the revision current at its start, which is newer than
		// the cached one, so the cache only moves forward.
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.building = false
			if r.snap != nil && r.snap.EqualVT(built) {
				built = r.snap
			}
			r.snap, r.snapRev = built, rev
			broadcast()
		})
		return built, rev
	}
}

// waitRev waits until the revision passes rev.
func (r *pluginStatusRows) waitRev(ctx context.Context, rev uint64, errCh <-chan error) error {
	for {
		// Return once a newer revision exists.
		var waitCh <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if r.rev == rev {
				waitCh = getWaitCh()
			}
		})
		if waitCh == nil {
			return nil
		}

		// Wait for the next broadcast, the context or an error.
		select {
		case <-ctx.Done():
			return context.Canceled
		case err, ok := <-errCh:
			if !ok {
				return context.Canceled
			}
			return err
		case <-waitCh:
		}
	}
}

// GetValue returns the snapshot of the current rows.
func (r *pluginStatusRows) GetValue() *bldr_plugin.PluginStatusSnapshot {
	snap, _ := r.snapshot()
	return snap
}

// WaitValueWithValidator waits for a snapshot that valid accepts.
func (r *pluginStatusRows) WaitValueWithValidator(
	ctx context.Context,
	valid func(v *bldr_plugin.PluginStatusSnapshot) (bool, error),
	errCh <-chan error,
) (*bldr_plugin.PluginStatusSnapshot, error) {
	for {
		// Check the current snapshot.
		snap, rev := r.snapshot()
		ok, err := valid(snap)
		if err != nil {
			return nil, err
		}
		if ok {
			return snap, nil
		}

		// Wait for the next revision.
		if err := r.waitRev(ctx, rev, errCh); err != nil {
			return nil, err
		}
	}
}

// WaitValue returns the current snapshot, which is never nil.
func (r *pluginStatusRows) WaitValue(ctx context.Context, errCh <-chan error) (*bldr_plugin.PluginStatusSnapshot, error) {
	return r.WaitValueWithValidator(ctx, func(v *bldr_plugin.PluginStatusSnapshot) (bool, error) {
		return v != nil, nil
	}, errCh)
}

// WaitValueChange waits for a snapshot whose rows differ from old.
func (r *pluginStatusRows) WaitValueChange(
	ctx context.Context,
	old *bldr_plugin.PluginStatusSnapshot,
	errCh <-chan error,
) (*bldr_plugin.PluginStatusSnapshot, error) {
	return r.WaitValueWithValidator(ctx, func(v *bldr_plugin.PluginStatusSnapshot) (bool, error) {
		return !v.EqualVT(old), nil
	}, errCh)
}

// WaitValueEmpty waits for a nil snapshot. The snapshot is never nil, so it
// returns only when ctx ends or errCh yields.
func (r *pluginStatusRows) WaitValueEmpty(ctx context.Context, errCh <-chan error) error {
	_, err := r.WaitValueWithValidator(ctx, func(v *bldr_plugin.PluginStatusSnapshot) (bool, error) {
		return v == nil, nil
	}, errCh)
	return err
}

// buildPluginStatusSnapshot clones rows into a snapshot sorted by plugin and
// instance key.
func buildPluginStatusSnapshot(
	plugins []*bldr_plugin.PluginStatus,
	recovery []*bldr_plugin.PluginManifestRecoveryStatus,
	executions []*bldr_plugin.PluginExecutionStatus,
) *bldr_plugin.PluginStatusSnapshot {
	// Clone every row so a reader cannot reach a stored row.
	snap := &bldr_plugin.PluginStatusSnapshot{
		Plugins:          cloneRows(plugins),
		ManifestRecovery: cloneRows(recovery),
		Executions:       cloneRows(executions),
	}

	// Sort each list by plugin and instance key.
	slices.SortFunc(snap.Plugins, func(a, b *bldr_plugin.PluginStatus) int {
		return comparePluginInstance(a.GetPluginId(), a.GetInstanceKey(), b.GetPluginId(), b.GetInstanceKey())
	})
	slices.SortFunc(snap.ManifestRecovery, func(a, b *bldr_plugin.PluginManifestRecoveryStatus) int {
		return comparePluginInstance(a.GetPluginId(), a.GetInstanceKey(), b.GetPluginId(), b.GetInstanceKey())
	})
	slices.SortFunc(snap.Executions, func(a, b *bldr_plugin.PluginExecutionStatus) int {
		return comparePluginInstance(a.GetPluginId(), a.GetInstanceKey(), b.GetPluginId(), b.GetInstanceKey())
	})
	return snap
}

// mapValues returns the non-nil values of m.
func mapValues[T any](m map[string]*T) []*T {
	out := make([]*T, 0, len(m))
	for _, v := range m {
		if v != nil {
			out = append(out, v)
		}
	}
	return out
}

// cloneRows returns a deep copy of each row.
func cloneRows[T interface{ CloneVT() T }](rows []T) []T {
	out := make([]T, len(rows))
	for i, row := range rows {
		out[i] = row.CloneVT()
	}
	return out
}

// _ is a type assertion
var _ ccontainer.Watchable[*bldr_plugin.PluginStatusSnapshot] = (*pluginStatusRows)(nil)
