package world_block_engine

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
)

// defaultHeadStateKey is the default key used for head state
const defaultHeadStateKey = "world-head"

// loadHeadState loads the head ref from the store.
func (c *Controller) loadHeadState(ctx context.Context, store object.ObjectStore) (*HeadState, bool, error) {
	if err := refreshHeadStoreForCoordination(store); err != nil {
		return nil, false, err
	}
	ktx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, false, err
	}
	defer ktx.Discard()

	headKey := []byte(c.conf.GetObjectStoreHeadKey())
	if len(headKey) == 0 {
		headKey = []byte(defaultHeadStateKey)
	}

	data, found, err := ktx.Get(ctx, headKey)
	if err != nil || !found {
		return nil, false, err
	}

	if !c.conf.GetStateTransformConf().GetEmpty() {
		var err error
		data, err = c.stateXfrm.DecodeBlock(data)
		if err != nil {
			return nil, false, err
		}
	}

	s := &HeadState{}
	if err := s.UnmarshalVT(data); err != nil {
		return nil, true, err
	}
	return s, true, nil
}

// writeHeadState writes the head state to the store when the durable head still
// matches the transaction base.
func (c *Controller) writeHeadState(ctx context.Context, store object.ObjectStore, baseRef, nref *bucket.ObjectRef) error {
	if err := refreshHeadStoreForCoordination(store); err != nil {
		return err
	}
	ktx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer ktx.Discard()

	headKey := []byte(c.conf.GetObjectStoreHeadKey())
	if len(headKey) == 0 {
		headKey = []byte(defaultHeadStateKey)
	}

	data, found, err := ktx.Get(ctx, headKey)
	if err != nil {
		return err
	}
	if found {
		if !c.conf.GetStateTransformConf().GetEmpty() {
			data, err = c.stateXfrm.DecodeBlock(data)
			if err != nil {
				return err
			}
		}
		s := &HeadState{}
		if err := s.UnmarshalVT(data); err != nil {
			return err
		}
		if !headRefsEqual(s.GetHeadRef(), baseRef) {
			return coord.ErrStaleGeneration
		}
	} else if baseRef != nil && !baseRef.GetRootRef().GetEmpty() {
		return coord.ErrStaleGeneration
	}

	v := &HeadState{HeadRef: nref}
	data, err = v.MarshalVT()
	if err != nil {
		return err
	}

	if !c.conf.GetStateTransformConf().GetEmpty() {
		data, err = c.stateXfrm.EncodeBlock(data)
		if err != nil {
			return err
		}
	}

	if err := ktx.Set(ctx, headKey, data); err != nil {
		return err
	}

	return ktx.Commit(ctx)
}

func headRefsEqual(a, b *bucket.ObjectRef) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.EqualsRef(b)
}

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

func refreshHeadStoreForCoordination(store object.ObjectStore) error {
	refreshable, ok := store.(kvtx.CoordinationRefreshStore)
	if !ok {
		return nil
	}
	return refreshable.RefreshForCoordinationLock()
}
