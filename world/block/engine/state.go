package world_block_engine

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/object"
)

// defaultHeadStateKey is the default key used for head state
const defaultHeadStateKey = "world-head"

// loadHeadState loads the head ref from the store.
func (c *Controller) loadHeadState(ctx context.Context, store object.ObjectStore) (*HeadState, bool, error) {
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

	decData, err := c.stateXfrm.DecodeBlock(data)
	if err != nil {
		return nil, false, err
	}

	s := &HeadState{}
	if err := s.UnmarshalVT(decData); err != nil {
		return nil, true, err
	}
	return s, true, nil
}

// writeHeadState writes the head state to the store.
func (c *Controller) writeHeadState(ctx context.Context, store object.ObjectStore, nref *bucket.ObjectRef) error {
	ktx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer ktx.Discard()

	headKey := []byte(c.conf.GetObjectStoreHeadKey())
	if len(headKey) == 0 {
		headKey = []byte(defaultHeadStateKey)
	}

	v := &HeadState{HeadRef: nref}
	data, err := v.MarshalVT()
	if err != nil {
		return err
	}

	encData, err := c.stateXfrm.EncodeBlock(data)
	if err != nil {
		return err
	}

	if err := ktx.Set(ctx, headKey, encData); err != nil {
		return err
	}

	return ktx.Commit(ctx)
}
