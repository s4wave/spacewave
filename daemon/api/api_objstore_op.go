package hydra_api

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/hydra/volume"
)

// ObjectStoreOp performs an object store operation.
func (a *API) ObjectStoreOp(
	ctx context.Context,
	req *ObjectStoreOpRequest,
) (*ObjectStoreOpResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	av, diRef, err := bus.ExecOneOff(
		ctx,
		a.bus,
		volume.NewBuildObjectStoreAPI(
			req.GetStoreName(),
			req.GetVolumeId(),
		),
		nil,
	)
	if err != nil {
		return nil, err
	}
	defer diRef.Release()
	bv, ok := av.GetValue().(volume.BuildObjectStoreAPIValue)
	if !ok {
		return nil, errors.New("object store api value was invalid")
	}
	if err := bv.GetError(); err != nil {
		return nil, err
	}
	os := bv.GetObjectStore()
	if os == nil {
		return nil, errors.New("object store value was empty")
	}

	var write bool
	switch req.GetOp() {
	case ObjectStoreOp_ObjectStoreOp_DELETE_KEY:
		fallthrough
	case ObjectStoreOp_ObjectStoreOp_PUT_KEY:
		write = true
	}

	tx, err := os.NewTransaction(write)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	err = nil
	resp := &ObjectStoreOpResponse{}
	reqKey := []byte(req.GetKey())
	switch req.GetOp() {
	case ObjectStoreOp_ObjectStoreOp_GET_KEY:
		resp.Data, resp.Found, err = tx.Get(reqKey)
	case ObjectStoreOp_ObjectStoreOp_PUT_KEY:
		err = tx.Set(reqKey, req.GetData())
	case ObjectStoreOp_ObjectStoreOp_DELETE_KEY:
		err = tx.Delete(reqKey)
	case ObjectStoreOp_ObjectStoreOp_LIST_KEYS:
		var keys []string
		err = tx.ScanPrefix([]byte(reqKey), func(key, _ []byte) error {
			keys = append(keys, string(key))
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
