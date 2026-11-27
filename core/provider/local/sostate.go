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
	// Reload restarts the mount so it loads the stored state. Reload is
	// required. It is called when storage holds other bytes than the mount
	// loaded or last wrote, or may: after a write fails its compare, after
	// a failure that may have committed finds neither the write nor the
	// state under it, and when such a failure's outcome cannot be read. The
	// mount's later writes fail with sobject.ErrMountReloading from the call
	// on, even while a backoff postpones the restart.
	Reload func()
}

// NewObjectStoreSOStateFuncs constructs a SOHostState backed by an object store
// and in-memory locks.
//
// Each state write is a compare-and-set against the state this mount loaded
// or last wrote: it fails with ErrStateChanged when another writer, such as
// ReplaceSharedObjectState, changed the stored state since.
// rctx is the context to use for looking up states from the object store.
// localPeer binds private read checkpoints to the local participant. An empty
// localPeer disables retention.
// runs is shared by every run of the object's mount (SOStateRuns). A mount
// with no other runs passes its own.
// layout is the layout states are loaded from and written in. A state stored
// in the other layout does not load, so the caller converts it first.
// watchFn watches the committed state.
func NewObjectStoreSOStateFuncs(rctx context.Context, objStore object.ObjectStore, localPeer peer.ID, runs *SOStateRuns, layout SOStateLayout) (
	watchFn sobject.SOStateWatchFunc,
	lockFn sobject.SOStateLockFunc,
	syncFuncs *sobject.SOHostSyncFuncs,
) {
	// Load one state per retained object without reading its complete history.
	reads := mountTransactions{ObjectStore: objStore, ctx: rctx}
	soRc := keyed.NewKeyedRefCount(
		func(sharedObjectID string) (keyed.Routine, *soStateEntry) {
			selected, _ := layout.layouts(sharedObjectID)
			ent := newSOStateEntry(selected)
			return func(ctx context.Context) error {
				return ent.execute(ctx, reads, runs, sharedObjectID)
			}, ent
		},
		keyed.WithExitCb(func(sharedObjectID string, _ keyed.Routine, ent *soStateEntry, err error) {
			if err != nil {
				ent.stateProm.SetResult(nil, err)
			}
		}),
	)
	soRc.SetContext(rctx, true)

	// writable ends when this run calls Reload, before the restart that ends
	// rctx, which a backoff may postpone by seconds.
	//
	// Invariant: once a write's outcome is unknown, or storage holds a state
	// this run did not load or write, this run commits no further write. The
	// restarted run settles what storage holds, so a write the old run
	// committed in the window would extend a state no reader has settled,
	// and the settled head would not be the state at the unknown with or
	// without the unknown write. Each write checks writable inside its
	// transaction, after taking runs.Writes, so a write that checked before
	// the fence never commits after a later load either.
	writable, endWrites := context.WithCancelCause(rctx)
	reload := func() {
		if !sobject.ReloadFenceOff {
			endWrites(sobject.ErrMountReloading)
		}
		runs.Reload()
	}

	// writeErr returns why this run cannot write, or nil.
	writeErr := func() error {
		if err := rctx.Err(); err != nil {
			return err
		}
		if writable.Err() != nil {
			return context.Cause(writable)
		}
		return nil
	}

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

	// settle settles a write of token that failed with err, and returns nil
	// when it committed anyway, err when it did not, or ErrStateWriteUnknown
	// when storage cannot show which. ended reports that the transaction body
	// finished, so the failure came at or after the commit. Any earlier failure
	// stored nothing. A failed compare read another writer's state as stored. A
	// failure at the commit may have stored the write, so read the token key
	// now: token means the write committed, and the state's token under it or
	// none means it did not. Any other state may follow this write, so its
	// outcome is unknown. The read is sound because the store shows no failed
	// commit as stored (kvtx.AtomicCommitStore): s4db refuses every new
	// transaction once a flush fails, so the read fails and the outcome is
	// unknown, rather than finding a token a power loss would drop. The mount reloads whenever storage holds, or may
	// hold, another state than the one it last wrote. The caller holds
	// ent.writeMtx.
	settle := func(sharedObjectID string, ent *soStateEntry, token []byte, ended bool, err error) error {
		// A failure before the commit stored nothing.
		if errors.Is(err, ErrStateChanged) {
			reload()
			return err
		}
		if !ended {
			return err
		}

		// Read what storage holds, unless the mount has ended.
		var stored []byte
		var found bool
		rerr := rctx.Err()
		if hook := testHookStateSettling.Load(); rerr == nil && hook != nil {
			rerr = (*hook)(sharedObjectID)
		}
		if rerr == nil {
			rerr = kvtx.RunTransaction(rctx, false,
				func(ctx context.Context) (kvtx.Tx, error) {
					return reads.NewTransaction(ctx, false)
				},
				func(ctx context.Context, tx kvtx.Tx) error {
					var err error
					stored, found, err = tx.Get(ctx, ent.layout.tokenKey())
					return err
				},
			)
		}

		// Settle from what storage holds.
		switch {
		case rerr != nil:
			reload()
			return errors.Join(sobject.ErrStateWriteUnknown, err)
		case found && bytes.Equal(stored, token):
			return nil
		case !found || bytes.Equal(stored, ent.token):
			return err
		default:
			// Another state may be a successor another mount wrote over
			// this one after it committed, so the outcome is unknown.
			reload()
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
		if err := writeErr(); err != nil {
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
				// Store an independent state snapshot as its delta from the
				// committed state, which no writer modifies.
				ctx, task := trace.NewTask(ctx, "alpha/local-so/write-so-state")
				defer task.End()
				state = state.Clone()
				committed := ent.stateCtr.GetValue()
				var token []byte

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
				err := kvtx.RunTransaction(ctx, true,
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
						// raced the delete finds no state and fails. A run that
						// called Reload writes nothing more (writable).
						if err := writeErr(); err != nil {
							return err
						}
						if hook := testHookStateWriting.Load(); hook != nil {
							(*hook)(sharedObjectID)
						}
						stored, found, err := tx.Get(ctx, ent.layout.tokenKey())
						if err != nil {
							return err
						}
						if !found {
							return sobject.ErrSharedObjectNotFound
						}

						// A replaced object stays replaced. The write builds on the state this
						// entry holds, so it commits only over its token. A replace commits
						// between the mount's last write and its restart. A write in that window
						// finds the new state and fails rather than writing the old lineage back,
						// and the mount reloads what storage holds.
						if !bytes.Equal(stored, ent.token) {
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

						// Write the state over the entry's token. A test may discard the
						// transaction after the write.
						token, err = ent.layout.write(ctx, tx, ent.token, committed, state)
						if err != nil {
							return err
						}
						ended = true
						if hook := testHookStateCommit.Load(); hook != nil {
							return (*hook)(sharedObjectID, initialState, state)
						}
						return nil
					},
				)
				if relWrites != nil {
					relWrites()
				}
				if hook := testHookStateCommitted.Load(); err == nil && hook != nil {
					err = (*hook)(sharedObjectID)
				}
				if err != nil {
					if err := settle(sharedObjectID, ent, token, ended, err); err != nil {
						return err
					}
				}

				// Make the committed snapshot visible only after the write commits.
				ent.commit(state, token)
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
	syncFuncs = NewSOHostSyncFuncs(reads)
	syncFuncs.Lock = func(ctx context.Context, id string) (sobject.SOStateLock, error) {
		return newLock(ctx, id, false, true)
	}
	syncFuncs.CheckpointLock = func(ctx context.Context, id string) (sobject.SOStateLock, error) {
		return newLock(ctx, id, true, false)
	}
	return watchFn, lockFn, syncFuncs
}
