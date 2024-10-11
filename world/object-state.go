package world

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/aperturerobotics/util/ccontainer"
)

// ObjectState contains the object state interface.
// Represents a handle a object in the store.
type ObjectState interface {
	// GetKey returns the key this state object is for.
	GetKey() string
	// GetRootRef returns the root reference.
	// Returns the revision number.
	GetRootRef(ctx context.Context) (*bucket.ObjectRef, uint64, error)

	// AccessWorldState builds a bucket lookup cursor with an optional ref.
	// If the ref is empty, will default to the object RootRef.
	// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
	// The lookup cursor will be released after cb returns.
	AccessWorldState(
		ctx context.Context,
		ref *bucket.ObjectRef,
		cb func(*bucket_lookup.Cursor) error,
	) error

	// SetRootRef changes the root reference of the object.
	// Increments the revision of the object if changed.
	// Returns revision just after the change was applied.
	SetRootRef(ctx context.Context, nref *bucket.ObjectRef) (uint64, error)

	// ApplyObjectOp applies a batch operation at the object level.
	// The handling of the operation is operation-type specific.
	// Returns the revision following the operation execution.
	// If nil is returned for the error, implies success.
	// If sysErr is set, the error is treated as a transient system error.
	// Returns rev, sysErr, err
	ApplyObjectOp(
		ctx context.Context,
		op Operation,
		opSender peer.ID,
	) (rev uint64, sysErr bool, err error)

	// IncrementRev increments the revision of the object.
	// Returns revision just after the change was applied.
	IncrementRev(ctx context.Context) (uint64, error)

	// WaitRev waits until the object rev is >= the specified.
	// Returns ErrObjectNotFound if the object is deleted.
	// If ignoreNotFound is set, waits for the object to exist.
	// Returns the new rev.
	WaitRev(ctx context.Context, rev uint64, ignoreNotFound bool) (uint64, error)
}

// NewAccessWatchableObjectState creates an access func for a watchable from the object state.
// The object state usually is constructed with NewEngineWorldState => GetObject.
// The initial version will be looked up and returned as part of the access func.
// released is called when the watch loop exits for any reason.
// released can be nil
func NewAccessWatchableObjectState[T protobuf_go_lite.EqualVT[T]](
	objState ObjectState,
	unmarshal func(ctx context.Context, bcs *block.Cursor) (T, error),
) func(ctx context.Context, released func()) (ccontainer.Watchable[T], func(), error) {
	return func(rctx context.Context, released func()) (ccontainer.Watchable[T], func(), error) {
		// Look up the initial state.
		initRef, initRev, err := objState.GetRootRef(rctx)
		if err != nil {
			return nil, nil, err
		}

		// Unmarshal the state.
		var initState T
		_, err = AccessObject(rctx, objState.AccessWorldState, initRef, func(bcs *block.Cursor) error {
			var uerr error
			initState, uerr = unmarshal(rctx, bcs)
			return uerr
		})
		if err != nil {
			return nil, nil, err
		}

		// Spawn goroutine and return state container.
		stateCtr := ccontainer.NewCContainerVT[T](initState)
		ctx, ctxCancel := context.WithCancel(rctx)
		go func() {
			defer ctxCancel()
			if released != nil {
				defer released()
			}

			prevRev, prevRef := initRev, initRef
			for {
				// wait for rev increment
				_, err := objState.WaitRev(ctx, prevRev+1, false)
				if err != nil {
					return
				}

				// get root ref
				currRef, currRev, err := objState.GetRootRef(ctx)
				if err != nil {
					return
				}

				// check equality, set prev
				wasEqual := currRef.EqualVT(prevRef)
				prevRev, prevRef = currRev, currRef
				if wasEqual {
					continue
				}

				// changed
				var nextState T
				_, err = AccessObject(ctx, objState.AccessWorldState, currRef, func(bcs *block.Cursor) error {
					var uerr error
					nextState, uerr = unmarshal(ctx, bcs)
					return uerr
				})
				if err != nil {
					return
				}

				// update ccontainer
				stateCtr.SetValue(nextState)
			}
		}()

		return stateCtr, ctxCancel, nil
	}
}
