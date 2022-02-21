package volume_world

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
)

// TODO: de-bounce writes to storage: batch updates to the root ref.

// loadHeadState loads the head ref from the world.
func (v *Volume) loadHeadState(ctx context.Context, ws world.WorldState) (*bucket.ObjectRef, bool, error) {
	obj, found, err := ws.GetObject(v.conf.GetObjectKey())
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	rootRef, _, err := obj.GetRootRef()
	if err != nil {
		return nil, true, err
	}
	return rootRef, true, nil
}

// writeHeadState writes the head state to the store.
func (v *Volume) writeHeadState(ctx context.Context, ws world.WorldState, nref *bucket.ObjectRef) error {
	objKey := v.conf.GetObjectKey()
	obj, found, err := ws.GetObject(objKey)
	if err != nil {
		return err
	}
	if !found {
		_, err = ws.CreateObject(objKey, nref)
		return err
	}

	_, err = obj.SetRootRef(nref)
	return err
}
