package provider_local

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// NewObjectStoreSOStateFuncs constructs a SOHostState backed by an object store and in-memory locks.
//
// Assumes no other writers will access the object store.
// rctx is the context to use for looking up states from the object store.
// localPeer binds private read checkpoints to the local participant; empty disables retention.
func NewObjectStoreSOStateFuncs(rctx context.Context, objStore object.ObjectStore, localPeer peer.ID) (
	watchFn sobject.SOStateWatchFunc,
	lockFn sobject.SOStateLockFunc,
	syncFuncs *sobject.SOHostSyncFuncs,
) {
	// Keep each mounted state and its write lock under one reference-counted entry.
	type soStateEntry struct {
		// stateProm completes after loading the persisted state.
		stateProm *promise.PromiseContainer[*sobject.SOState]
		// stateCtr publishes only committed state snapshots.
		stateCtr *ccontainer.CContainer[*sobject.SOState]
		// writeMtx serializes mutations while the state entry is retained.
		writeMtx csync.Mutex
		// key addresses the serialized SOState in the object store.
		key []byte
	}

	// Load one state per retained object without reading its complete history.
	soRc := keyed.NewKeyedRefCount(
		func(sharedObjectID string) (keyed.Routine, *soStateEntry) {
			ent := &soStateEntry{
				stateProm: promise.NewPromiseContainer[*sobject.SOState](),
				stateCtr:  ccontainer.NewCContainer[*sobject.SOState](nil),
				key:       SobjectObjectStoreHostStateKey(sharedObjectID),
			}
			return func(ctx context.Context) error {
				// Read the durable state through the store transaction boundary.
				var data []byte
				var found bool
				err := kvtx.RunTransaction(ctx, false,
					func(ctx context.Context) (kvtx.Tx, error) {
						return objStore.NewTransaction(ctx, false)
					},
					func(ctx context.Context, tx kvtx.Tx) error {
						var err error
						data, found, err = tx.Get(ctx, ent.key)
						return err
					},
				)
				if err != nil {
					return err
				}
				if !found {
					return world.ErrObjectNotFound
				}

				// Validate the persisted state before exposing it to consumers.
				val := &sobject.SOState{}
				if err := val.UnmarshalVT(data); err != nil {
					return err
				}
				if err := val.Validate(sharedObjectID); err != nil {
					return err
				}

				// Publish the validated initial state to waiters and watches.
				ent.stateProm.SetResult(val, nil)
				ent.stateCtr.SetValue(val)
				return nil
			}, ent
		},
		keyed.WithExitCb(func(sharedObjectID string, _ keyed.Routine, ent *soStateEntry, err error) {
			if err != nil {
				ent.stateProm.SetResult(nil, err)
			}
		}),
	)
	soRc.SetContext(rctx, true)

	// Bind watched state to the caller's reference lifetime.
	watchFn = func(ctx context.Context, sharedObjectID string, released func()) (ccontainer.Watchable[*sobject.SOState], func(), error) {
		ref, ent, _ := soRc.AddKeyRef(sharedObjectID)
		_, err := ent.stateProm.Await(ctx)
		if err != nil {
			ref.Release()
			return nil, nil, err
		}

		return ent.stateCtr, ref.Release, nil
	}

	// Retain the state entry until the locked write has completed.
	newLock := func(ctx context.Context, sharedObjectID string, checkpoint, peerImport bool) (sobject.SOStateLock, error) {
		ref, ent, _ := soRc.AddKeyRef(sharedObjectID)
		_, err := ent.stateProm.Await(ctx)
		if err != nil {
			ref.Release()
			return nil, err
		}

		// Serialize writes after the initial state load succeeds.
		relLock, err := ent.writeMtx.Lock(ctx)
		if err != nil {
			ref.Release()
			return nil, err
		}

		// Commit state and supplied configuration history before updating watchers.
		initialState := ent.stateCtr.GetValue().CloneVT()
		return sobject.NewSOStateLock(
			initialState,
			func(ctx context.Context, state *sobject.SOState, changes ...*sobject.SOConfigChange) error {
				// Serialize an independent state snapshot for replayable transaction writes.
				ctx, task := trace.NewTask(ctx, "alpha/local-so/write-so-state")
				defer task.End()
				state = state.CloneVT()
				data, err := state.MarshalVT()
				if err != nil {
					return err
				}

				// Retain signed changes and state under the same commit boundary.
				// Only an ordinary local operation may commit ordered; imports
				// and checkpoints always commit in full.
				ordered := sobject.OrderedOperation(ctx) && !checkpoint && !peerImport
				err = kvtx.RunTransaction(ctx, true,
					func(ctx context.Context) (kvtx.Tx, error) {
						tx, err := objStore.NewTransaction(ctx, true)
						if err != nil || !ordered {
							return tx, err
						}
						return kvtx.WithOrderedCommit(tx), nil
					},
					func(ctx context.Context, tx kvtx.Tx) error {
						if checkpoint || peerImport {
							config := initialState.GetConfig()
							if checkpoint {
								config = state.GetConfig()
							}
							if err := WriteSOConfigCheckpoint(ctx, tx, sharedObjectID, config, checkpoint); err != nil {
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
				ent.stateProm.SetResult(state, nil)
				ent.stateCtr.SetValue(state)
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
