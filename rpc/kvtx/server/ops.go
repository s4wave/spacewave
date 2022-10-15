package rpc_kvtx_server

import (
	"context"

	rpc_kvtx "github.com/aperturerobotics/bldr/rpc/kvtx"
	"github.com/aperturerobotics/hydra/kvtx"
)

// Ops implements the kvtx transaction operations.
type Ops struct {
	// ops is the underlying ops interface.
	ops kvtx.TxOps
}

// NewOps constructs a new KvtxOps service.
func NewOps(ops kvtx.TxOps) *Ops {
	return &Ops{ops: ops}
}

// KeyCount counts the keys in the store.
func (o *Ops) KeyCount(ctx context.Context, req *rpc_kvtx.KeyCountRequest) (*rpc_kvtx.KeyCountResponse, error) {
	count, err := o.ops.Size()
	if err != nil {
		return nil, err
	}
	return &rpc_kvtx.KeyCountResponse{KeyCount: count}, nil
}

// KeyData looks up data for a key.
func (o *Ops) KeyData(ctx context.Context, req *rpc_kvtx.KvtxKeyRequest) (*rpc_kvtx.KvtxKeyDataResponse, error) {
	key := req.GetKey()
	data, found, err := o.ops.Get(key)
	resp := &rpc_kvtx.KvtxKeyDataResponse{}
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Data = data
		resp.Found = found
	}
	return resp, nil
}

// KeyExists checks if the key exists in the store.
func (o *Ops) KeyExists(ctx context.Context, req *rpc_kvtx.KvtxKeyRequest) (*rpc_kvtx.KvtxKeyExistsResponse, error) {
	key := req.GetKey()
	found, err := o.ops.Exists(key)
	resp := &rpc_kvtx.KvtxKeyExistsResponse{}
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Found = found
	}
	return resp, nil
}

// SetKey sets the key in the store.
func (o *Ops) SetKey(ctx context.Context, req *rpc_kvtx.KvtxSetKeyRequest) (*rpc_kvtx.KvtxSetKeyResponse, error) {
	err := o.ops.Set(req.GetKey(), req.GetValue())
	resp := &rpc_kvtx.KvtxSetKeyResponse{}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

func (o *Ops) DeleteKey(ctx context.Context, req *rpc_kvtx.KvtxDeleteKeyRequest) (*rpc_kvtx.KvtxDeleteKeyResponse, error) {
	err := o.ops.Delete(req.GetKey())
	resp := &rpc_kvtx.KvtxDeleteKeyResponse{}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

// _ is a type assertion
var _ rpc_kvtx.SRPCKvtxOpsServer = ((*Ops)(nil))
