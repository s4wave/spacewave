package resource_world

import (
	"context"
	"errors"

	"github.com/aperturerobotics/util/routine"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// TrackedWorldState wraps a WorldState and records all access patterns.
// Immediately starts change detection as accesses are recorded.
type TrackedWorldState struct {
	ws world.WorldState

	// stateRoutine manages change detection with current snapshot
	stateRoutine *routine.StateRoutineContainer[*s4wave_world.TrackedWorldStateSnapshot]

	// currentSnapshot is the current tracking snapshot
	currentSnapshot *s4wave_world.TrackedWorldStateSnapshot

	// changeResultCh receives error (or nil) when changes are detected
	changeResultCh chan error
}

// NewTrackedWorldState creates a new TrackedWorldState.
func NewTrackedWorldState(ws world.WorldState, initialSeqno uint64, ctx context.Context) *TrackedWorldState {
	t := &TrackedWorldState{
		ws: ws,
		currentSnapshot: &s4wave_world.TrackedWorldStateSnapshot{
			ObjectAccesses: make([]*s4wave_world.TrackedWorldStateSnapshot_ObjectAccess, 0),
			HasQuadAccess:  false,
			InitialSeqno:   initialSeqno,
		},
		changeResultCh: make(chan error, 1),
	}

	// Create StateRoutineContainer with protobuf EqualVT comparison
	t.stateRoutine = routine.NewStateRoutineContainerVT[*s4wave_world.TrackedWorldStateSnapshot]()

	// Set the state routine function that watches for changes
	t.stateRoutine.SetStateRoutine(func(ctx context.Context, snapshot *s4wave_world.TrackedWorldStateSnapshot) error {
		err := watchTrackedChanges(ctx, snapshot, ws)
		if errors.Is(err, context.Canceled) {
			return err
		}

		// Write result to channel (nil = changes detected, error = watch failed)
		select {
		case t.changeResultCh <- err:
		default:
		}
		return err
	})

	// Set context to start the routine
	t.stateRoutine.SetContext(ctx, false)

	return t
}

// WaitForChanges waits until any tracked resource changes.
// Returns nil when changes detected, error on failure or context cancel.
func (t *TrackedWorldState) WaitForChanges(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-t.changeResultCh:
		return err
	}
}

// cloneAndUpdateSnapshot creates a new snapshot with updated tracking data.
func (t *TrackedWorldState) cloneAndUpdateSnapshot(updateFn func(*s4wave_world.TrackedWorldStateSnapshot)) *s4wave_world.TrackedWorldStateSnapshot {
	// Clone the current snapshot
	newSnapshot := &s4wave_world.TrackedWorldStateSnapshot{
		ObjectAccesses: make([]*s4wave_world.TrackedWorldStateSnapshot_ObjectAccess, len(t.currentSnapshot.ObjectAccesses)),
		HasQuadAccess:  t.currentSnapshot.HasQuadAccess,
		InitialSeqno:   t.currentSnapshot.InitialSeqno,
	}
	copy(newSnapshot.ObjectAccesses, t.currentSnapshot.ObjectAccesses)

	// Apply update
	updateFn(newSnapshot)

	return newSnapshot
}

// trackObjectAccess records an object access.
func (t *TrackedWorldState) trackObjectAccess(key string, rev uint64) {
	// Clone snapshot and add new access
	newSnapshot := t.cloneAndUpdateSnapshot(func(snap *s4wave_world.TrackedWorldStateSnapshot) {
		// Check if this key already exists
		found := false
		for _, objAccess := range snap.ObjectAccesses {
			if objAccess.Key == key {
				// Update existing entry
				objAccess.Rev = rev
				found = true
				break
			}
		}
		if !found {
			// Add new entry
			snap.ObjectAccesses = append(snap.ObjectAccesses, &s4wave_world.TrackedWorldStateSnapshot_ObjectAccess{
				Key: key,
				Rev: rev,
			})
		}
	})

	// Update current snapshot and notify StateRoutine
	t.currentSnapshot = newSnapshot
	t.stateRoutine.SetState(newSnapshot)
}

// trackQuadQuery records a quad query access.
func (t *TrackedWorldState) trackQuadQuery() {
	// Clone snapshot and set quad flag
	newSnapshot := t.cloneAndUpdateSnapshot(func(snap *s4wave_world.TrackedWorldStateSnapshot) {
		snap.HasQuadAccess = true
	})

	// Update current snapshot and notify StateRoutine
	t.currentSnapshot = newSnapshot
	t.stateRoutine.SetState(newSnapshot)
}

// Close stops the change detection routine.
func (t *TrackedWorldState) Close() {
	t.stateRoutine.ClearContext()
}

// WorldState interface implementation - delegates to wrapped ws

func (t *TrackedWorldState) GetReadOnly() bool {
	return t.ws.GetReadOnly()
}

func (t *TrackedWorldState) GetSeqno(ctx context.Context) (uint64, error) {
	return t.ws.GetSeqno(ctx)
}

func (t *TrackedWorldState) WaitSeqno(ctx context.Context, seqno uint64) (uint64, error) {
	return t.ws.WaitSeqno(ctx, seqno)
}

func (t *TrackedWorldState) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return t.ws.BuildStorageCursor(ctx)
}

func (t *TrackedWorldState) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	return t.ws.AccessWorldState(ctx, ref, cb)
}

func (t *TrackedWorldState) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	obj, err := t.ws.CreateObject(ctx, key, rootRef)
	if err == nil {
		// Track this access
		_, rev, _ := obj.GetRootRef(ctx)
		t.trackObjectAccess(key, rev)
	}
	return obj, err
}

func (t *TrackedWorldState) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	obj, found, err := t.ws.GetObject(ctx, key)
	if err == nil {
		// Track this access even if not found (rev=0 means non-existent)
		rev := uint64(0)
		if found {
			_, rev, _ = obj.GetRootRef(ctx)
		}
		t.trackObjectAccess(key, rev)
	}
	return obj, found, err
}

func (t *TrackedWorldState) IterateObjects(ctx context.Context, prefix string, reversed bool) world.ObjectIterator {
	// Note: Iterator access is not tracked per-object, could track prefix
	return t.ws.IterateObjects(ctx, prefix, reversed)
}

func (t *TrackedWorldState) RenameObject(ctx context.Context, oldKey, newKey string, descendants bool) (world.ObjectState, error) {
	obj, err := t.ws.RenameObject(ctx, oldKey, newKey, descendants)
	if err == nil {
		t.trackObjectAccess(oldKey, 0)
		_, rev, _ := obj.GetRootRef(ctx)
		t.trackObjectAccess(newKey, rev)
		t.trackQuadQuery()
	}
	return obj, err
}

func (t *TrackedWorldState) DeleteObject(ctx context.Context, key string) (bool, error) {
	return t.ws.DeleteObject(ctx, key)
}

func (t *TrackedWorldState) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	// Track that quad access occurred
	t.trackQuadQuery()
	return t.ws.AccessCayleyGraph(ctx, write, cb)
}

func (t *TrackedWorldState) SetGraphQuad(ctx context.Context, q world.GraphQuad) error {
	t.trackQuadQuery()
	return t.ws.SetGraphQuad(ctx, q)
}

func (t *TrackedWorldState) DeleteGraphQuad(ctx context.Context, q world.GraphQuad) error {
	t.trackQuadQuery()
	return t.ws.DeleteGraphQuad(ctx, q)
}

func (t *TrackedWorldState) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	quads, err := t.ws.LookupGraphQuads(ctx, filter, limit)
	if err == nil {
		t.trackQuadQuery()
	}
	return quads, err
}

func (t *TrackedWorldState) DeleteGraphObject(ctx context.Context, objKey string) error {
	t.trackQuadQuery()
	return t.ws.DeleteGraphObject(ctx, objKey)
}

func (t *TrackedWorldState) ApplyWorldOp(ctx context.Context, op world.Operation, opSender peer.ID) (uint64, bool, error) {
	return t.ws.ApplyWorldOp(ctx, op, opSender)
}

// watchTrackedChanges is a StateRoutine that monitors a TrackedWorldStateSnapshot for changes.
// Returns nil when any tracked resource changes.
func watchTrackedChanges(ctx context.Context, snapshot *s4wave_world.TrackedWorldStateSnapshot, ws world.WorldState) error {
	if snapshot == nil || (len(snapshot.ObjectAccesses) == 0 && !snapshot.HasQuadAccess) {
		// Nothing to track yet, wait for context cancellation
		<-ctx.Done()
		return ctx.Err()
	}

	// Loop: wait for world changes, then check if tracked resources changed
	for {
		// First check: see if any tracked resources already changed
		changed, err := checkTrackedChanges(ctx, snapshot, ws)
		if err != nil {
			return err
		}
		if changed {
			// Something changed, return to trigger new tracked WorldState
			return nil
		}

		// Nothing changed yet, wait for world seqno to increment
		currentSeqno, err := ws.GetSeqno(ctx)
		if err != nil {
			return err
		}

		_, err = ws.WaitSeqno(ctx, currentSeqno+1)
		if err != nil {
			return err
		}

		// World changed, loop back to check tracked resources
	}
}

// checkTrackedChanges checks if any tracked resources have changed.
// Returns true if any change detected, false if all unchanged.
func checkTrackedChanges(ctx context.Context, snapshot *s4wave_world.TrackedWorldStateSnapshot, ws world.WorldState) (bool, error) {
	// Check each tracked object
	for _, objAccess := range snapshot.ObjectAccesses {
		obj, found, err := ws.GetObject(ctx, objAccess.Key)
		if err != nil {
			return false, err
		}

		trackedRev := objAccess.Rev
		currentRev := uint64(0)

		if found {
			_, currentRev, err = obj.GetRootRef(ctx)
			if err != nil {
				return false, err
			}
		}

		// Detect changes:
		// - Tracked as non-existent (rev=0) but now exists (currentRev>0)
		// - Tracked as existing (rev>0) but now deleted (currentRev=0)
		// - Revision increased (currentRev>trackedRev)
		if currentRev != trackedRev {
			return true, nil
		}
	}

	// Check world seqno if quad queries occurred
	if snapshot.HasQuadAccess {
		currentSeqno, err := ws.GetSeqno(ctx)
		if err != nil {
			return false, err
		}

		if currentSeqno > snapshot.InitialSeqno {
			// World seqno changed (coarse check for quad changes)
			return true, nil
		}
	}

	// Nothing changed
	return false, nil
}
