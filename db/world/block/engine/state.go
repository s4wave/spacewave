package world_block_engine

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
)

// defaultHeadStateKey is the default key used for head state.
const defaultHeadStateKey = "world-head"

// loadHeadState loads the head ref from the store.
func (c *Controller) loadHeadState(ctx context.Context, store object.ObjectStore) (*HeadState, bool, error) {
	// The store's read transaction observes committed metadata without a writer refresh.
	// A coordination refresh can remap storage and wait for this caller's other readers.
	ktx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, false, err
	}
	defer ktx.Discard()

	// Resolve the persisted head key within this object store.
	headKey := []byte(c.conf.GetObjectStoreHeadKey())
	if len(headKey) == 0 {
		headKey = []byte(defaultHeadStateKey)
	}

	// Read the current committed record through the read snapshot.
	data, found, err := ktx.Get(ctx, headKey)
	if err != nil || !found {
		return nil, false, err
	}

	// Decode the configured storage transform before parsing the head.
	if !c.conf.GetStateTransformConf().GetEmpty() {
		var err error
		data, err = c.stateXfrm.DecodeBlock(data)
		if err != nil {
			return nil, false, err
		}
	}

	// Return the durable head without changing coordination or storage state.
	s := &HeadState{}
	if err := s.UnmarshalVT(data); err != nil {
		return nil, true, err
	}
	return s, true, nil
}

// writeHeadState writes the head state to the store when the durable head still
// matches the transaction base.
func (c *Controller) writeHeadState(ctx context.Context, store object.ObjectStore, baseRef, nref *bucket.ObjectRef) error {
	// Refresh the coordinated writer before beginning its compare-and-swap transaction.
	if err := refreshHeadStoreForCoordination(store); err != nil {
		return err
	}
	ktx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer ktx.Discard()

	// Resolve the persisted head key within this object store.
	headKey := []byte(c.conf.GetObjectStoreHeadKey())
	if len(headKey) == 0 {
		headKey = []byte(defaultHeadStateKey)
	}

	// Reject publication when another writer replaced the transaction's base.
	data, found, err := ktx.Get(ctx, headKey)
	if err != nil {
		return err
	}
	data, err = c.replaceHeadState(ctx, data, found, baseRef, nref)
	if err != nil {
		return err
	}

	// Publish the validated replacement in the same transaction.
	if err := ktx.Set(ctx, headKey, data); err != nil {
		return err
	}
	return ktx.Commit(ctx)
}

// replaceHeadState is the common decoded CAS/encoding step. It performs no
// I/O and can run inside a volume-owned atomic publication transaction.
func (c *Controller) replaceHeadState(ctx context.Context, data []byte, found bool, baseRef, nref *bucket.ObjectRef) ([]byte, error) {
	var err error
	if found {
		if !c.conf.GetStateTransformConf().GetEmpty() {
			data, err = c.stateXfrm.DecodeBlock(data)
			if err != nil {
				return nil, err
			}
		}
		s := &HeadState{}
		if err := s.UnmarshalVT(data); err != nil {
			return nil, err
		}
		if !headRefsEqual(s.GetHeadRef(), baseRef) {
			return nil, coord.ErrStaleGeneration
		}
	} else if baseRef != nil && !baseRef.GetRootRef().GetEmpty() {
		return nil, coord.ErrStaleGeneration
	}

	// Encode the replacement head under the configured storage transform.
	v := &HeadState{HeadRef: nref}
	data, err = v.MarshalVT()
	if err != nil {
		return nil, err
	}

	if !c.conf.GetStateTransformConf().GetEmpty() {
		data, err = c.stateXfrm.EncodeBlock(data)
		if err != nil {
			return nil, err
		}
	}

	return data, nil
}

// headRefsEqual compares optional persisted World references.
func headRefsEqual(a, b *bucket.ObjectRef) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.EqualsRef(b)
}

// objectStoreHeadKeyPrefix identifies the head's key in the backing store.
func (c *Controller) objectStoreHeadKeyPrefix() []byte {
	headKey := []byte(c.conf.GetObjectStoreHeadKey())
	if len(headKey) == 0 {
		headKey = []byte(defaultHeadStateKey)
	}
	prefix := []byte(c.conf.GetObjectStorePrefix())
	out := make([]byte, 0, len(prefix)+len(headKey))
	out = append(out, prefix...)
	out = append(out, headKey...)
	return out
}

// refreshHeadStoreForCoordination prepares supported stores for coordinated writes.
func refreshHeadStoreForCoordination(store object.ObjectStore) error {
	refreshable, ok := store.(kvtx.CoordinationRefreshStore)
	if !ok {
		return nil
	}
	return refreshable.RefreshForCoordinationLock()
}
