package kvtx_rpc_server

import (
	"context"
	"errors"

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
	count, err := o.ops.Size(ctx)
	if err != nil {
		return nil, err
	}
	return &kvtx_rpc.KeyCountResponse{KeyCount: count}, nil
}

// KeyData looks up data for a key.
func (o *Ops) KeyData(ctx context.Context, req *kvtx_rpc.KvtxKeyRequest) (*kvtx_rpc.KvtxKeyDataResponse, error) {
	key := req.GetKey()
	data, found, err := o.ops.Get(ctx, key)
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
	found, err := o.ops.Exists(ctx, key)
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
	err := o.ops.Set(ctx, req.GetKey(), req.GetValue())
	resp := &kvtx_rpc.KvtxSetKeyResponse{}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

func (o *Ops) DeleteKey(ctx context.Context, req *kvtx_rpc.KvtxDeleteKeyRequest) (*kvtx_rpc.KvtxDeleteKeyResponse, error) {
	err := o.ops.Delete(ctx, req.GetKey())
	resp := &kvtx_rpc.KvtxDeleteKeyResponse{}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

// ScanPrefix scans for key/value pairs with a key prefix.
func (o *Ops) ScanPrefix(req *kvtx_rpc.KvtxScanPrefixRequest, strm kvtx_rpc.SRPCKvtxOps_ScanPrefixStream) error {
	var err error
	if req.GetOnlyKeys() {
		err = o.ops.ScanPrefixKeys(strm.Context(), req.GetPrefix(), func(key []byte) error {
			return strm.Send(&kvtx_rpc.KvtxScanPrefixResponse{
				Key: key,
			})
		})
	} else {
		err = o.ops.ScanPrefix(strm.Context(), req.GetPrefix(), func(key, value []byte) error {
			return strm.Send(&kvtx_rpc.KvtxScanPrefixResponse{
				Key:   key,
				Value: value,
			})
		})
	}
	if err != nil {
		return strm.Send(&kvtx_rpc.KvtxScanPrefixResponse{
			Error: err.Error(),
		})
	}
	return nil
}

// Iterate iterates over the kvtx store.
func (o *Ops) Iterate(strm kvtx_rpc.SRPCKvtxOps_IterateStream) error {
	initReq, err := strm.Recv()
	if err != nil {
		return err
	}

	init := initReq.GetInit()
	it := o.ops.Iterate(strm.Context(), init.GetPrefix(), init.GetSort(), init.GetReverse())
	if it == nil {
		err = errors.New("iterate returned nil iterator")
	} else {
		err = it.Err()
		defer it.Close()
	}

	sendReqErr := func(err error) error {
		return strm.Send(&kvtx_rpc.KvtxIterateResponse{
			Body: &kvtx_rpc.KvtxIterateResponse_ReqError{
				ReqError: err.Error(),
			},
		})
	}

	if err != nil {
		return sendReqErr(err)
	} else {
		if err := strm.Send(&kvtx_rpc.KvtxIterateResponse{
			Body: &kvtx_rpc.KvtxIterateResponse_Ack{
				Ack: true,
			},
		}); err != nil {
			return err
		}
	}

	sendStatus := func(valid bool) error {
		key := it.Key()
		var itErrStr string
		if itErr := it.Err(); itErr != nil {
			itErrStr = itErr.Error()
		}
		return strm.Send(&kvtx_rpc.KvtxIterateResponse{
			Body: &kvtx_rpc.KvtxIterateResponse_Status{
				Status: &kvtx_rpc.KvtxIterateStatus{
					Error: itErrStr,
					Valid: valid,
					Key:   key,
				},
			},
		})
	}

	for {
		msg, err := strm.Recv()
		if err != nil {
			return err
		}

		switch m := msg.GetBody().(type) {
		case *kvtx_rpc.KvtxIterateRequest_Init:
			return errors.New("init sent multiple times")
		case *kvtx_rpc.KvtxIterateRequest_LookupValue:
			if !m.LookupValue {
				break
			}
			val, err := it.Value()
			if err != nil {
				if sendErr := sendReqErr(err); sendErr != nil {
					return sendErr
				}
				break
			}
			if err := strm.Send(&kvtx_rpc.KvtxIterateResponse{
				Body: &kvtx_rpc.KvtxIterateResponse_Value{
					Value: val,
				},
			}); err != nil {
				return err
			}
		case *kvtx_rpc.KvtxIterateRequest_Next:
			if !m.Next {
				continue
			}
			valid := it.Next()
			if err := sendStatus(valid); err != nil {
				return err
			}
		case *kvtx_rpc.KvtxIterateRequest_Seek:
			if len(m.Seek) == 0 {
				continue
			}
			if err := it.Seek(m.Seek); err != nil {
				if sendErr := sendReqErr(err); sendErr != nil {
					return err
				}
			} else if err := sendStatus(it.Valid()); err != nil {
				return err
			}
		case *kvtx_rpc.KvtxIterateRequest_SeekBeginning:
			if !m.SeekBeginning {
				continue
			}
			if err := it.Seek(nil); err != nil {
				if sendErr := sendReqErr(err); sendErr != nil {
					return err
				}
			} else if err := sendStatus(it.Valid()); err != nil {
				return err
			}
		case *kvtx_rpc.KvtxIterateRequest_Close:
			if !m.Close {
				continue
			}
			it.Close()
			if err := strm.Send(&kvtx_rpc.KvtxIterateResponse{
				Body: &kvtx_rpc.KvtxIterateResponse_Closed{
					Closed: true,
				},
			}); err != nil {
				return err
			}
			return nil
		}
	}
}

// _ is a type assertion
var _ kvtx_rpc.SRPCKvtxOpsServer = ((*Ops)(nil))
