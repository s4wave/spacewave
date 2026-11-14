package provider_local

import (
	"bytes"
	"context"
	"errors"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/net/peer"
)

// SOStateRuns joins the state funcs of the successive mount runs of one
// shared object. A run's load waits only for this object's own state writes,
// never for the store's writer, which other objects and the volume's
// upkeep share.
type SOStateRuns struct {
	// Writes is held shared by each state write from its mount check, taken
	// inside its write transaction, until the transaction ends. Each load
	// takes it alone and releases it at once, as a barrier.
	Writes csync.RWMutex
	// Reload restarts the mount so it loads the stored state; it is
	// required. It is called when storage holds other bytes than the mount
	// loaded or last wrote, or may: after a write fails its compare, after a
	// failure that may have committed finds neither the write nor the state
	// under it, and when such a failure's outcome cannot be read. The mount cannot write
	// again until it reloads.
	Reload func()
}

// NewObjectStoreSOStateFuncs constructs a SOHostState backed by an object store and in-memory locks.
//
// Each state write is a compare-and-set against the state this mount loaded
// or last wrote: it fails with ErrStateChanged when another writer, such as
// ReplaceSharedObjectState, changed the stored state since.
// rctx is the context to use for looking up states from the object store.
// localPeer binds private read checkpoints to the local participant; empty disables retention.
// runs is shared by every run of the object's mount (SOStateRuns); a mount
// with no other runs passes its own.
// watchFn watches the committed state.
func NewObjectStoreSOStateFuncs(rctx context.Context, objStore object.ObjectStore, localPeer peer.ID, runs *SOStateRuns) (
	watchFn sobject.SOStateWatchFunc,
	lockFn sobject.SOStateLockFunc,
	syncFuncs *sobject.SOHostSyncFuncs,
) {
	// Load one state per retained object without reading its complete history.
	soRc := keyed.NewKeyedRefCount(
		func(sharedObjectID string) (keyed.Routine, *soStateEntry) {
			ent := newSOStateEntry(SobjectObjectStoreHostStateKey(sharedObjectID))
			return func(ctx context.Context) error {
				return ent.execute(ctx, objStore, runs, sharedObjectID)
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

	// settle settles a write of data that failed with err, and returns nil
	// when it committed anyway, err when it did not, or ErrStateWriteUnknown
	// when storage cannot show which. ended reports that the transaction
	// body finished, so the failure came at or after the commit; any earlier
	// failure stored nothing. A failed compare read another writer's state
	// as stored. A failure at the commit may have stored the write, so read
	// what storage holds now: data means the write committed, and the state
	// under it or none means it did not. Any other state may follow this
	// write, so its outcome is unknown. The mount reloads whenever storage
	// holds, or may hold, another state than the one it last wrote. The
	// caller holds ent.writeMtx.
	settle := func(sharedObjectID string, ent *soStateEntry, data []byte, ended bool, err error) error {
		// A failure before the commit stored nothing.
		if errors.Is(err, ErrStateChanged) {
			runs.Reload()
			return err
		}
		if !ended {
			return err
		}

		// Read what storage holds, unless the mount has ended.
		var stored []byte
		var found bool
		rerr := rctx.Err()
		if rerr == nil && testHookStateSettling != nil {
			rerr = testHookStateSettling(sharedObjectID)
		}
		if rerr == nil {
			rerr = kvtx.RunTransaction(rctx, false,
				func(ctx context.Context) (kvtx.Tx, error) {
					return objStore.NewTransaction(ctx, false)
				},
				func(ctx context.Context, tx kvtx.Tx) error {
					var err error
					stored, found, err = tx.Get(ctx, ent.key)
					return err
				},
			)
		}

		// Settle from what it holds.
		switch {
		case rerr != nil:
			runs.Reload()
			return errors.Join(sobject.ErrStateWriteUnknown, err)
		case found && bytes.Equal(stored, data):
			return nil
		case !found || bytes.Equal(stored, ent.data):
			return err
		default:
			// Another state may be a successor another mount wrote over
			// this one after it committed, so the outcome is unknown.
			runs.Reload()
			return errors.Join(sobject.ErrStateWriteUnknown, err)
		}
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

				// Hold off a restarted run's load from the mount check until
				// this transaction ends. The write takes the gate inside its
				// transaction, holding the store's writer, so a load waits
				// only for a write already committing, not one still queued
				// behind other writers. A retried attempt drops the gate before
				// it reopens, since the discarded attempt wrote nothing, so the
				// gate is never held while waiting for the writer.
				var relWrites func()

				// Retain signed changes and state under the same commit
				// boundary. ended marks an attempt whose body finished, so
				// only its commit can have failed.
				var ended bool
				err = kvtx.RunTransaction(ctx, true,
					func(ctx context.Context) (kvtx.Tx, error) {
						if relWrites != nil {
							relWrites()
							relWrites = nil
						}
						return objStore.NewTransaction(ctx, true)
					},
					func(ctx context.Context, tx kvtx.Tx) error {
						// Hold off a restarted run's load until this transaction ends.
						ended = false
						rel, err := runs.Writes.Lock(ctx, false)
						if err != nil {
							return err
						}
						relWrites = rel

						// A deleted object stays deleted. A handle from a mount that
						// has ended cannot write: a delete restarts the mount before
						// releasing the account lock a relist needs, so a stale
						// handle never writes over a relisted object. A write that
						// raced the delete finds no state and fails.
						if err := rctx.Err(); err != nil {
							return err
						}
						if testHookStateWriting != nil {
							testHookStateWriting(sharedObjectID)
						}
						stored, found, err := tx.Get(ctx, ent.key)
						if err != nil {
							return err
						}
						if !found {
							return sobject.ErrSharedObjectNotFound
						}

						// A replaced object stays replaced. The write builds on the
						// state this entry holds, so it commits only over those
						// bytes. A replace commits between the mount's last write
						// and its restart; a write in that window finds the new
						// state and fails rather than writing the old lineage back,
						// and the mount reloads what storage holds.
						if !bytes.Equal(stored, ent.data) {
							return ErrStateChanged
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
						if err := tx.Set(ctx, ent.key, data); err != nil {
							return err
						}
						ended = true
						return nil
					},
				)
				if relWrites != nil {
					relWrites()
				}
				if err == nil && testHookStateCommitted != nil {
					err = testHookStateCommitted(sharedObjectID)
				}
				if err != nil {
					if err := settle(sharedObjectID, ent, data, ended, err); err != nil {
						return err
					}
				}

				// Make the committed snapshot visible only after the write commits.
				ent.commit(state, data)
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
