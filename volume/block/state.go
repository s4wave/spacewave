package volume_block

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/object"
	"github.com/golang/protobuf/proto"
)

// defaultHeadStateKey is the default key used for head state
const defaultHeadStateKey = "volume-head"

// loadHeadState loads the head ref from the store.
func (v *Volume) loadHeadState(ctx context.Context, store object.ObjectStore) (*HeadState, bool, error) {
	ktx, err := store.NewTransaction(false)
	if err != nil {
		return nil, false, err
	}
	defer ktx.Discard()

	headKey := []byte(v.conf.GetObjectStoreHeadKey())
	if len(headKey) == 0 {
		headKey = []byte(defaultHeadStateKey)
	}

	data, found, err := ktx.Get(headKey)
	if err != nil || !found {
		return nil, false, err
	}

	decData, err := v.stateXfrm.DecodeBlock(data)
	if err != nil {
		return nil, false, err
	}

	s := &HeadState{}
	if err := proto.Unmarshal(decData, s); err != nil {
		return nil, true, err
	}
	return s, true, nil
}

// writeHeadState writes the head state to the store.
func (v *Volume) writeHeadState(ctx context.Context, store object.ObjectStore, nref *bucket.ObjectRef) error {
	ktx, err := store.NewTransaction(true)
	if err != nil {
		return err
	}
	defer ktx.Discard()

	headKey := []byte(v.conf.GetObjectStoreHeadKey())
	if len(headKey) == 0 {
		headKey = []byte(defaultHeadStateKey)
	}

	hs := &HeadState{HeadRef: nref}
	data, err := proto.Marshal(hs)
	if err != nil {
		return err
	}

	encData, err := v.stateXfrm.EncodeBlock(data)
	if err != nil {
		return err
	}

	if err := ktx.Set(headKey, encData); err != nil {
		return err
	}

	return ktx.Commit(ctx)
}
