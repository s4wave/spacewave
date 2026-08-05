package inmem

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/coord"
)

// Coordinator coordinates ObjectStore access within one process.
type Coordinator struct {
	mu       sync.Mutex
	cond     *sync.Cond
	scopes   map[scopeKey]*scopeState
	watchSeq uint64
}

// NewCoordinator builds an in-memory coordinator.
func NewCoordinator() *Coordinator {
	c := &Coordinator{
		scopes: make(map[scopeKey]*scopeState),
	}
	c.cond = sync.NewCond(&c.mu)
	return c
}

// Capability reports in-memory coordination support.
func (c *Coordinator) Capability(ctx context.Context, scope coord.Scope) (*coord.Capability, error) {
	// Reject canceled capability requests before reading shared state.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Snapshot the current scope generation under the coordinator lock.
	c.mu.Lock()
	state := c.getScopeLocked(scope)
	generation := state.generation
	c.mu.Unlock()

	return &coord.Capability{
		Supported:     true,
		Backend:       coord.BackendKindInMemory,
		VolumeID:      scope.VolumeID,
		ObjectStoreID: scope.ObjectStoreID,
		Generation:    generation,
		Generations:   scope.Key == "",
	}, nil
}

// Snapshot returns the latest in-memory generation and root.
func (c *Coordinator) Snapshot(ctx context.Context, scope coord.Scope) (*coord.Snapshot, error) {
	// Reject canceled or keyed snapshot requests before reading state.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if scope.Key != "" {
		return nil, coord.ErrUnsupported
	}

	// Copy the scope snapshot while holding the state lock.
	c.mu.Lock()
	snapshot := c.getScopeLocked(scope).snapshot(scope)
	c.mu.Unlock()
	return snapshot, nil
}

// Watch returns coordination events after afterGeneration.
func (c *Coordinator) Watch(ctx context.Context, scope coord.Scope, afterGeneration uint64) (coord.Watch, error) {
	// Reject canceled or keyed watch requests before registering a watcher.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if scope.Key != "" {
		return nil, coord.ErrUnsupported
	}

	// Register the watcher and replay a missed generation while locked.
	watch := &watch{
		c:     c,
		scope: scope,
		ch:    make(chan coord.Event, 16),
		done:  make(chan struct{}),
	}

	c.mu.Lock()
	c.watchSeq++
	watch.id = c.watchSeq
	state := c.getScopeLocked(scope)
	state.watchers[watch.id] = watch
	if state.generation > afterGeneration {
		watch.sendLocked(state.snapshotEvent(scope))
	}
	c.mu.Unlock()

	// Close the watcher when its request context ends.
	go func() {
		select {
		case <-ctx.Done():
			_ = watch.Close()
		case <-watch.done:
		}
	}()

	return watch, nil
}

// TryAcquireWriteLease attempts to acquire the logical write lease.
func (c *Coordinator) TryAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, bool, error) {
	// Validate the lease request before checking lock ownership.
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if scope.ObjectStoreID == "" && scope.Key == "" {
		return nil, false, coord.ErrScopeEmpty
	}

	// Publish contention or claim the scope lock atomically.
	c.mu.Lock()
	defer c.mu.Unlock()

	state := c.getScopeLocked(scope)
	if state.locked {
		state.publishLocked(coord.Event{
			ProcessID:     scope.ParticipantID,
			VolumeID:      scope.VolumeID,
			ObjectStoreID: scope.ObjectStoreID,
			Generation:    state.generation,
			WantLock:      true,
		})
		return nil, false, nil
	}

	state.locked = true
	return &lease{c: c, scope: scope, state: state, done: make(chan struct{})}, true, nil
}

// WaitAcquireWriteLease waits until the logical write lease is available.
func (c *Coordinator) WaitAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, error) {
	// Validate the wait request before installing cancellation wakeups.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if scope.ObjectStoreID == "" && scope.Key == "" {
		return nil, coord.ErrScopeEmpty
	}

	// Wake the condition wait when the context is canceled.
	cancelWake := context.AfterFunc(ctx, func() {
		c.mu.Lock()
		c.cond.Broadcast()
		c.mu.Unlock()
	})
	defer cancelWake()

	// Wait for the scope lock and recheck context after each wakeup.
	c.mu.Lock()
	defer c.mu.Unlock()

	state := c.getScopeLocked(scope)
	for state.locked {
		state.publishLocked(coord.Event{
			ProcessID:     scope.ParticipantID,
			VolumeID:      scope.VolumeID,
			ObjectStoreID: scope.ObjectStoreID,
			Generation:    state.generation,
			WantLock:      true,
		})
		c.cond.Wait()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}

	state.locked = true
	return &lease{c: c, scope: scope, state: state, done: make(chan struct{})}, nil
}

func (c *Coordinator) getScopeLocked(scope coord.Scope) *scopeState {
	key := newScopeKey(scope)
	state := c.scopes[key]
	if state == nil {
		state = newScopeState()
		c.scopes[key] = state
	}
	return state
}

// _ is a type assertion
var _ coord.Coordinator = (*Coordinator)(nil)
