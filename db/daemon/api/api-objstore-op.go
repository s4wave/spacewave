package hydra_api

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/db/volume"
)

// ObjectStoreOp performs an object store operation.
func (a *API) ObjectStoreOp(
	ctx context.Context,
	req *ObjectStoreOpRequest,
) (*ObjectStoreOpResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	av, _, diRef, err := bus.ExecOneOffTyped[volume.BuildObjectStoreAPIValue](
		ctx,
		a.bus,
		volume.NewBuildObjectStoreAPI(
			req.GetStoreName(),
			req.GetVolumeId(),
		),
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}
	defer diRef.Release()

	objStore := av.GetValue().GetObjectStore()
	if objStore == nil {
		return nil, errors.New("object store value was empty")
	}

	var write bool
	switch req.GetOp() {
	case ObjectStoreOp_ObjectStoreOp_DELETE_KEY:
		fallthrough
	case ObjectStoreOp_ObjectStoreOp_PUT_KEY:
		write = true
	}

	tx, err := objStore.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	err = nil
	resp := &ObjectStoreOpResponse{}
	reqKey := []byte(req.GetKey())
	switch req.GetOp() {
	case ObjectStoreOp_ObjectStoreOp_GET_KEY:
		resp.Data, resp.Found, err = tx.Get(ctx, reqKey)
	case ObjectStoreOp_ObjectStoreOp_PUT_KEY:
		err = tx.Set(ctx, reqKey, req.GetData())
	case ObjectStoreOp_ObjectStoreOp_DELETE_KEY:
		resp.Found, err = tx.Exists(ctx, reqKey)
		if err == nil && resp.Found {
			err = tx.Delete(ctx, reqKey)
		}
	case ObjectStoreOp_ObjectStoreOp_LIST_KEYS:
		var keys [][]byte
		err = tx.ScanPrefix(ctx, reqKey, func(key, _ []byte) error {
			keys = append(keys, key)
			return nil
		})
		resp.Keys = keys
	}
	if err == nil && write {
		err = tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	return resp, nil
}
