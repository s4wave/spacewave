package provider_local

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/world"
)

// NewObjectStoreSOStateFuncs constructs a SOHostState backed by an object store and in-memory locks.
//
// Assumes no other writers will access the object store.
// rctx is the context to use for looking up states from the object store.
func NewObjectStoreSOStateFuncs(rctx context.Context, objStore object.ObjectStore) (
	watchFn sobject.SOStateWatchFunc,
	lockFn sobject.SOStateLockFunc,
) {
	type soStateEntry struct {
		stateProm *promise.PromiseContainer[*sobject.SOState]
		stateCtr  *ccontainer.CContainer[*sobject.SOState]
		writeMtx  csync.Mutex
		key       []byte
	}

	soRc := keyed.NewKeyedRefCount(
		func(sharedObjectID string) (keyed.Routine, *soStateEntry) {
			ent := &soStateEntry{
				stateProm: promise.NewPromiseContainer[*sobject.SOState](),
				stateCtr:  ccontainer.NewCContainer[*sobject.SOState](nil),
				key:       SobjectObjectStoreHostStateKey(sharedObjectID),
			}
			return func(ctx context.Context) error {
				otx, err := objStore.NewTransaction(ctx, false)
				if err != nil {
					return err
				}

				data, found, err := otx.Get(ctx, ent.key)
				otx.Discard()
				if err != nil {
					return err
				}
				if !found {
					return world.ErrObjectNotFound
				}

				val := &sobject.SOState{}
				if err := val.UnmarshalVT(data); err != nil {
					return err
				}

				if err := val.Validate(sharedObjectID); err != nil {
					return err
				}

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

	watchFn = func(ctx context.Context, sharedObjectID string, released func()) (ccontainer.Watchable[*sobject.SOState], func(), error) {
		ref, ent, _ := soRc.AddKeyRef(sharedObjectID)
		_, err := ent.stateProm.Await(ctx)
		if err != nil {
			ref.Release()
			return nil, nil, err
		}

		return ent.stateCtr, ref.Release, nil
	}

	lockFn = func(ctx context.Context, sharedObjectID string) (sobject.SOStateLock, error) {
		ref, ent, _ := soRc.AddKeyRef(sharedObjectID)
		_, err := ent.stateProm.Await(ctx)
		if err != nil {
			ref.Release()
			return nil, err
		}

		relLock, err := ent.writeMtx.Lock(ctx)
		if err != nil {
			ref.Release()
			return nil, err
		}

		initialState := ent.stateCtr.GetValue().CloneVT()
		return sobject.NewSOStateLock(
			initialState,
			func(ctx context.Context, state *sobject.SOState) error {
				state = state.CloneVT()
				data, err := state.MarshalVT()
				if err != nil {
					return err
				}

				tx, err := objStore.NewTransaction(ctx, true)
				if err != nil {
					return err
				}
				defer tx.Discard()

				if err := tx.Set(ctx, ent.key, data); err != nil {
					return err
				}

				err = tx.Commit(ctx)
				if err != nil {
					return err
				}

				ent.stateProm.SetResult(state, nil)
				ent.stateCtr.SetValue(state)
				return nil
			},
			relLock,
		), nil
	}

	return watchFn, lockFn
}
