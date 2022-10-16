package kvtx_rpc_server

import (
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_rpc "github.com/aperturerobotics/hydra/kvtx/rpc"
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
func (o *Ops) KeyCount(ctx context.Context, req *kvtx_rpc.KeyCountRequest) (*kvtx_rpc.KeyCountResponse, error) {
	count, err := o.ops.Size()
	if err != nil {
		return nil, err
	}
	return &kvtx_rpc.KeyCountResponse{KeyCount: count}, nil
}

// KeyData looks up data for a key.
func (o *Ops) KeyData(ctx context.Context, req *kvtx_rpc.KvtxKeyRequest) (*kvtx_rpc.KvtxKeyDataResponse, error) {
	key := req.GetKey()
	data, found, err := o.ops.Get(key)
	resp := &kvtx_rpc.KvtxKeyDataResponse{}
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Data = data
		resp.Found = found
	}
	return resp, nil
}

// KeyExists checks if the key exists in the store.
func (o *Ops) KeyExists(ctx context.Context, req *kvtx_rpc.KvtxKeyRequest) (*kvtx_rpc.KvtxKeyExistsResponse, error) {
	key := req.GetKey()
	found, err := o.ops.Exists(key)
	resp := &kvtx_rpc.KvtxKeyExistsResponse{}
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Found = found
	}
	return resp, nil
}

// SetKey sets the key in the store.
func (o *Ops) SetKey(ctx context.Context, req *kvtx_rpc.KvtxSetKeyRequest) (*kvtx_rpc.KvtxSetKeyResponse, error) {
	err := o.ops.Set(req.GetKey(), req.GetValue())
	resp := &kvtx_rpc.KvtxSetKeyResponse{}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

func (o *Ops) DeleteKey(ctx context.Context, req *kvtx_rpc.KvtxDeleteKeyRequest) (*kvtx_rpc.KvtxDeleteKeyResponse, error) {
	err := o.ops.Delete(req.GetKey())
	resp := &kvtx_rpc.KvtxDeleteKeyResponse{}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

// ScanPrefix scans for key/value pairs with a key prefix.
func (o *Ops) ScanPrefix(req *kvtx_rpc.KvtxScanPrefixRequest, strm kvtx_rpc.SRPCKvtxOps_ScanPrefixStream) error {
	err := o.ops.ScanPrefix(req.GetPrefix(), func(key, value []byte) error {
		return strm.Send(&kvtx_rpc.KvtxScanPrefixResponse{
			Key:   key,
			Value: value,
		})
	})
	if err != nil {
		return strm.Send(&kvtx_rpc.KvtxScanPrefixResponse{
			Error: err.Error(),
		})
	}
	return nil
}

// _ is a type assertion
var _ kvtx_rpc.SRPCKvtxOpsServer = ((*Ops)(nil))
