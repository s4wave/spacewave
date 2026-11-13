package provider_local

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/net/peer"
)

// NewObjectStoreSOStateFuncs constructs a SOHostState backed by an object store and in-memory locks.
//
// Assumes no other writers will access the object store.
// rctx is the context to use for looking up states from the object store.
// localPeer binds private read checkpoints to the local participant; empty disables retention.
// watchFn watches the committed state.
func NewObjectStoreSOStateFuncs(rctx context.Context, objStore object.ObjectStore, localPeer peer.ID) (
	watchFn sobject.SOStateWatchFunc,
	lockFn sobject.SOStateLockFunc,
	syncFuncs *sobject.SOHostSyncFuncs,
) {
	// Load one state per retained object without reading its complete history.
	soRc := keyed.NewKeyedRefCount(
		func(sharedObjectID string) (keyed.Routine, *soStateEntry) {
			ent := newSOStateEntry(SobjectObjectStoreHostStateKey(sharedObjectID))
			return func(ctx context.Context) error {
				return ent.execute(ctx, objStore, sharedObjectID)
			}, ent
		},
		keyed.WithExitCb(func(sharedObjectID string, _ keyed.Routine, ent *soStateEntry, err error) {
			if err != nil {
				ent.stateProm.SetResult(nil, err)
			}
		}),
	)
	soRc.SetContext(rctx, true)

	// awaitLoaded waits for ent's loaded state. An ended mount errors at once,
	// even when another handle keeps its entry loaded, so a lock or a watch
	// never starts for it. No entry loads once the mount ends, so the wait
	// ends with rctx too and returns its error, rather than holding the caller
	// until ctx ends. The mount can still end after this returns, so the write
	// transaction checks rctx again.
	awaitLoaded := func(ctx context.Context, ent *soStateEntry) error {
		// Refuse an ended mount before waiting.
		if err := rctx.Err(); err != nil {
			return err
		}

		// Wait for the load, ending the wait when the mount ends.
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		stop := context.AfterFunc(rctx, cancel)
		defer stop()
		_, err := ent.stateProm.Await(ctx)
		if rerr := rctx.Err(); err != nil && rerr != nil {
			return rerr
		}
		return err
	}

	// Bind the watched state of the entry to the caller's reference lifetime.
	watchFn = func(ctx context.Context, sharedObjectID string, _ func()) (ccontainer.Watchable[*sobject.SOState], func(), error) {
		ref, ent, _ := soRc.AddKeyRef(sharedObjectID)
		if err := awaitLoaded(ctx, ent); err != nil {
			ref.Release()
			return nil, nil, err
		}
		return ent.stateCtr, ref.Release, nil
	}

	// Hold the provider writer from the lock until the write commits, so each
	// write builds on the state the one before it committed.
	newLock := func(ctx context.Context, sharedObjectID string, checkpoint, peerImport bool) (sobject.SOStateLock, error) {
		// Retain the loaded state entry until the write lock is released.
		ref, ent, _ := soRc.AddKeyRef(sharedObjectID)
		if err := awaitLoaded(ctx, ent); err != nil {
			ref.Release()
			return nil, err
		}
		relLock, err := ent.writeMtx.Lock(ctx)
		if err != nil {
			ref.Release()
			return nil, err
		}

		// Commit state and supplied configuration history before updating watchers.
		initialState := ent.stateCtr.GetValue().Clone()
		return sobject.NewSOStateLock(
			initialState,
			func(ctx context.Context, state *sobject.SOState, changes ...*sobject.SOConfigChange) error {
				// Serialize an independent state snapshot.
				ctx, task := trace.NewTask(ctx, "alpha/local-so/write-so-state")
				defer task.End()
				state = state.Clone()
				data, err := state.MarshalVT()
				if err != nil {
					return err
				}

				// Retain signed changes and state under the same commit boundary.
				err = kvtx.RunTransaction(ctx, true,
					func(ctx context.Context) (kvtx.Tx, error) {
						return objStore.NewTransaction(ctx, true)
					},
					func(ctx context.Context, tx kvtx.Tx) error {
						// A deleted object stays deleted. A handle from a mount that
						// has ended cannot write: a delete restarts the mount before
						// releasing the account lock a relist needs, so a stale
						// handle never writes over a relisted object. A write that
						// raced the delete finds no state and fails.
						if err := rctx.Err(); err != nil {
							return err
						}
						found, err := tx.Exists(ctx, ent.key)
						if err != nil {
							return err
						}
						if !found {
							return sobject.ErrSharedObjectNotFound
						}

						// Record the config history and the read checkpoint with the state.
						if checkpoint {
							if err := WriteSOConfigLineage(ctx, tx, sharedObjectID, state.GetConfig(), changes); err != nil {
								return err
							}
						}
						if peerImport {
							if err := WriteSOConfigCheckpoint(ctx, tx, sharedObjectID, initialState.GetConfig(), false); err != nil {
								return err
							}
						}
						if !checkpoint {
							if err := WriteSOConfigHistory(ctx, tx, sharedObjectID, initialState.GetConfig(), state.GetConfig(), changes); err != nil {
								return err
							}
						}
						if err := writeReadCheckpoint(ctx, tx, sharedObjectID, localPeer, initialState, state); err != nil {
							return err
						}
						return tx.Set(ctx, ent.key, data)
					},
				)
				if err != nil {
					return err
				}

				// Make the committed snapshot visible only after the transaction succeeds.
				ent.commit(state)
				return nil
			},
			func() {
				relLock()
				ref.Release()
			},
		), nil
	}

	// All three operations share the same retained state and serialization lock.
	lockFn = func(ctx context.Context, id string) (sobject.SOStateLock, error) {
		return newLock(ctx, id, false, false)
	}
	syncFuncs = NewSOHostSyncFuncs(objStore)
	syncFuncs.Lock = func(ctx context.Context, id string) (sobject.SOStateLock, error) {
		return newLock(ctx, id, false, true)
	}
	syncFuncs.CheckpointLock = func(ctx context.Context, id string) (sobject.SOStateLock, error) {
		return newLock(ctx, id, true, false)
	}
	return watchFn, lockFn, syncFuncs
}
