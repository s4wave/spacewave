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
	return kvtx.NewErrIterator(errors.New("TODO Iterate kvtx rpc"))
}

// ScanPrefix scans for key/value pairs with a key prefix.
func (o *Ops) ScanPrefix(prefix []byte, cb func(key []byte, value []byte) error) error {
	return errors.New("TODO ScanPrefix")
}

// ScanPrefixKeys scans for keys with a key prefix.
func (o *Ops) ScanPrefixKeys(prefix []byte, cb func(key []byte) error) error {
	return errors.New("TODO ScanPrefixKeys")
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
	}
	return err
}

// _ is a type assertion
var _ kvtx.TxOps = ((*Ops)(nil))
