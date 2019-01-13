package api_controller

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/aperturerobotics/hydra/volume"
)

// ObjectStoreOp performs an object store operation.
func (a *API) ObjectStoreOp(
	ctx context.Context,
	req *api.ObjectStoreOpRequest,
) (*api.ObjectStoreOpResponse, error) {
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

	err = nil
	resp := &api.ObjectStoreOpResponse{}
	switch req.GetOp() {
	case api.ObjectStoreOp_ObjectStoreOp_GET_KEY:
		resp.Data, resp.Found, err = os.GetObject(req.GetKey())
	case api.ObjectStoreOp_ObjectStoreOp_PUT_KEY:
		err = os.SetObject(req.GetKey(), req.GetData())
	case api.ObjectStoreOp_ObjectStoreOp_DELETE_KEY:
		err = os.DeleteObject(req.GetKey())
	case api.ObjectStoreOp_ObjectStoreOp_LIST_KEYS:
		resp.Keys, err = os.ListKeys(req.GetKey())
	}
	if err != nil {
		return nil, err
	}
	return resp, nil
}
