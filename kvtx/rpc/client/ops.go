package kvtx_rpc_client

import (
	"context"
	"errors"
	"io"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_rpc "github.com/aperturerobotics/hydra/kvtx/rpc"
	"github.com/aperturerobotics/starpc/srpc"
)

// Ops implements TxOps with a KvtxOps service.
type Ops struct {
	// ctx is used for calls
	ctx context.Context
	// client is the service client
	client kvtx_rpc.SRPCKvtxOpsClient
}

// NewOps constructs a new TxOps.
func NewOps(ctx context.Context, client kvtx_rpc.SRPCKvtxOpsClient) *Ops {
	return &Ops{
		ctx:    ctx,
		client: client,
	}
}

// Size checks the number of key/value pairs in the store.
func (o *Ops) Size() (uint64, error) {
	resp, err := o.client.KeyCount(o.ctx, &kvtx_rpc.KeyCountRequest{})
	return resp.GetKeyCount(), err
}

// Get looks up a key and data from the store.
func (o *Ops) Get(key []byte) (data []byte, found bool, err error) {
	resp, err := o.client.KeyData(o.ctx, kvtx_rpc.NewKeyRequest(key))
	if err != nil {
		return nil, false, err
	}
	if err := o.err(err, resp.GetError()); err != nil {
		return nil, false, err
	}
	return resp.GetData(), resp.GetFound(), nil
}

// Set sets a key in the store.
func (o *Ops) Set(key []byte, value []byte) error {
	resp, err := o.client.SetKey(o.ctx, &kvtx_rpc.KvtxSetKeyRequest{
		Key:   key,
		Value: value,
	})
	if err := o.err(err, resp.GetError()); err != nil {
		return err
	}
	return nil
}

// Delete removes a key from the store.
func (o *Ops) Delete(key []byte) error {
	resp, err := o.client.DeleteKey(o.ctx, &kvtx_rpc.KvtxDeleteKeyRequest{
		Key: key,
	})
	if err := o.err(err, resp.GetError()); err != nil {
		return err
	}
	return nil
}

// Exists checks if a key exists in the store.
func (o *Ops) Exists(key []byte) (bool, error) {
	resp, err := o.client.KeyExists(o.ctx, kvtx_rpc.NewKeyRequest(key))
	if err := o.err(err, resp.GetError()); err != nil {
		return false, err
	}
	return resp.GetFound(), err
}

// Iterate iterates over the store.
func (o *Ops) Iterate(prefix []byte, sort bool, reverse bool) kvtx.Iterator {
	itClient, err := o.client.Iterate(o.ctx)
	if err != nil {
		return kvtx.NewErrIterator(err)
	}

	err = itClient.Send(&kvtx_rpc.KvtxIterateRequest{
		Body: &kvtx_rpc.KvtxIterateRequest_Init{
			Init: &kvtx_rpc.KvtxIterateInit{
				Prefix:  prefix,
				Sort:    sort,
				Reverse: reverse,
			},
		},
	})
	if err != nil {
		_ = itClient.Close()
		return kvtx.NewErrIterator(err)
	}

	// wait for init packet
	ackMsg, err := itClient.Recv()
	switch m := ackMsg.GetBody().(type) {
	case *kvtx_rpc.KvtxIterateResponse_ReqError:
		_ = itClient.Close()
		return kvtx.NewErrIterator(errors.New(m.ReqError))
	case *kvtx_rpc.KvtxIterateResponse_Ack:
		break
	default:
		_ = itClient.Close()
		return kvtx.NewErrIterator(errors.New("unexpected response to iterator init"))
	}

	// return iterator object
	return newIterator(itClient)
}

// ScanPrefix scans for key/value pairs with a key prefix.
func (o *Ops) ScanPrefix(prefix []byte, cb func(key, value []byte) error) error {
	if cb == nil {
		// nothing to do
		return nil
	}
	return o.scanPrefix(prefix, false, cb)
}

// ScanPrefixKeys scans for keys with a key prefix.
func (o *Ops) ScanPrefixKeys(prefix []byte, cb func(key []byte) error) error {
	if cb == nil {
		// nothing to do
		return nil
	}
	return o.scanPrefix(prefix, true, func(key, _ []byte) error {
		return cb(key)
	})
}

// scanPrefix performs the ScanPrefix and ScanPrefixKeys requests.
func (o *Ops) scanPrefix(prefix []byte, onlyKeys bool, cb func(key, value []byte) error) error {
	client, err := o.client.ScanPrefix(o.ctx, &kvtx_rpc.KvtxScanPrefixRequest{
		Prefix:   prefix,
		OnlyKeys: onlyKeys,
	})
	if err != nil {
		return err
	}
	defer client.Close()

	for {
		resp, err := client.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		if errStr := resp.GetError(); errStr != "" {
			return errors.New(errStr)
		}

		if key := resp.GetKey(); len(key) != 0 {
			if err := cb(key, resp.GetValue()); err != nil {
				return err
			}
		}
	}
}

// err converts an error into the appropriate error.
func (o *Ops) err(err error, errStr string) error {
	if err == nil {
		if errStr != "" {
			err = errors.New(errStr)
		} else {
			return nil
		}
	} else {
		errStr = err.Error()
	}
	switch errStr {
	case srpc.ErrCompleted.Error():
		fallthrough
	case io.EOF.Error():
		err = kvtx.ErrDiscarded
	case kvtx.ErrEmptyKey.Error():
		err = kvtx.ErrEmptyKey
	case kvtx.ErrBlockTxOpsUnimplemented.Error():
		err = kvtx.ErrBlockTxOpsUnimplemented
	case kvtx.ErrNotFound.Error():
		err = kvtx.ErrNotFound
	case kvtx.ErrNotWrite.Error():
		err = kvtx.ErrNotWrite
	}
	return err
}

// _ is a type assertion
var _ kvtx.TxOps = ((*Ops)(nil))
