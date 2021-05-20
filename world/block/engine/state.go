package world_block_engine

import (
	"context"

	"github.com/aperturerobotics/hydra/object"
	"github.com/golang/protobuf/proto"
)

// defaultHeadStateKey is the default key used for head state
const defaultHeadStateKey = "world-head"

// loadHeadState loads the head ref from the store.
func (c *Controller) loadHeadState(ctx context.Context, store object.ObjectStore) (*HeadState, bool, error) {
	ktx, err := store.NewTransaction(false)
	if err != nil {
		return nil, false, err
	}
	defer ktx.Discard()

	headKey := []byte(c.conf.GetObjectStoreHeadKey())
	if len(headKey) == 0 {
		headKey = []byte(defaultHeadStateKey)
	}

	data, found, err := ktx.Get(headKey)
	if err != nil || !found {
		return nil, false, err
	}

	s := &HeadState{}
	if err := proto.Unmarshal(data, s); err != nil {
		return nil, true, err
	}
	return s, true, nil
}
