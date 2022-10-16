package kvtx_rpc_client

import (
	"errors"
	"net/textproto"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_rpc "github.com/aperturerobotics/hydra/kvtx/rpc"
)

// Iterator implements the kvtx iterator handle.
type Iterator struct {
	// pipeline is the ops pipeline
	pipeline *textproto.Pipeline
	// client is the iterate client
	client kvtx_rpc.SRPCKvtxOps_IterateClient
	// closed indicates the iterator is closed
	closed atomic.Bool
	// status is the last received status object
	status atomic.Pointer[kvtx_rpc.KvtxIterateStatus]
	// value is the current value, if known.
	value atomic.Pointer[itValue]
}

// itValue contains the iterator value.
type itValue struct {
	// value is the value buf
	value []byte
}

// newIterator constructs a new iterator handle.
func newIterator(client kvtx_rpc.SRPCKvtxOps_IterateClient) *Iterator {
	return &Iterator{
		pipeline: &textproto.Pipeline{},
		client:   client,
	}
}

// Err returns any error that has closed the iterator.
// May return context.Canceled if closed.
func (i *Iterator) Err() error {
	status := i.status.Load()
	if errStr := status.GetError(); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

// Valid returns if the iterator points to a valid entry.
//
// If err is set, returns false.
func (i *Iterator) Valid() bool {
	status := i.status.Load()
	return status.GetValid()
}

// Key returns the current entry key, or nil if not valid.
func (i *Iterator) Key() []byte {
	status := i.status.Load()
	return status.GetKey()
}

// Value returns the current entry value, or nil if not valid.
//
// May cache the value between calls, copy if modifying.
func (i *Iterator) Value() ([]byte, error) {
	currVal := i.value.Load()
	if currVal != nil {
		return currVal.value, nil
	}

	id := i.pipeline.Next()
	i.pipeline.StartRequest(id)
	defer i.pipeline.EndRequest(id)

	if i.closed.Load() {
		return nil, kvtx.ErrDiscarded
	}

	if err := i.client.Send(&kvtx_rpc.KvtxIterateRequest{
		Body: &kvtx_rpc.KvtxIterateRequest_LookupValue{LookupValue: true},
	}); err != nil {
		return nil, err
	}

	// allow skipping 1 unexpected packet
	for x := 0; x < 2; x++ {
		resp, err := i.client.Recv()
		if err != nil {
			return nil, err
		}
		switch b := resp.Body.(type) {
		case *kvtx_rpc.KvtxIterateResponse_Value:
			value := b.Value
			i.value.Store(&itValue{value: value})
			return value, nil
		case *kvtx_rpc.KvtxIterateResponse_ReqError:
			return nil, errors.New(b.ReqError)
		}
	}

	return nil, errors.New("server did not return a value response")
}

// ValueCopy copies the key to the given byte slice and returns it.
// If the slice is not big enough (cap), it must create a new one and return it.
// May use the value cached from Value() call as the source of the data.
// May return nil if !Valid().
func (i *Iterator) ValueCopy(buf []byte) ([]byte, error) {
	val, err := i.Value()
	if err != nil {
		return nil, err
	}
	if cap(buf) < len(val) {
		buf = make([]byte, len(val))
	} else {
		buf = buf[:len(val)]
	}
	copy(buf, val)
	return buf, nil
}

// storeErr stores an error status (if err != nil).
func (i *Iterator) storeErr(err error) {
	if err != nil {
		_ = i.client.Close()
		i.status.Store(&kvtx_rpc.KvtxIterateStatus{
			Error: err.Error(),
		})
	}
}

// Next advances to the next entry and returns Valid.
func (i *Iterator) Next() bool {
	id := i.pipeline.Next()
	i.pipeline.StartRequest(id)
	defer i.pipeline.EndRequest(id)

	if i.closed.Load() {
		return false
	}

	i.value.Store(nil)
	if err := i.client.Send(&kvtx_rpc.KvtxIterateRequest{
		Body: &kvtx_rpc.KvtxIterateRequest_Next{Next: true},
	}); err != nil {
		i.storeErr(err)
		return false
	}

	// allow skipping 1 unexpected packet
	for x := 0; x < 2; x++ {
		resp, err := i.client.Recv()
		if err != nil {
			i.storeErr(err)
			return false
		}
		switch b := resp.Body.(type) {
		case *kvtx_rpc.KvtxIterateResponse_Status:
			status := b.Status
			i.status.Store(status)
			return status.GetValid()
		case *kvtx_rpc.KvtxIterateResponse_ReqError:
			i.storeErr(errors.New(b.ReqError))
			return false
		}
	}

	i.storeErr(errors.New("unexpected iterator response: next"))
	return false
}

// Seek moves the iterator to the first key >= the provided key.
// Pass nil to seek to the beginning (or end if reversed).
func (i *Iterator) Seek(k []byte) error {
	id := i.pipeline.Next()
	i.pipeline.StartRequest(id)
	defer i.pipeline.EndRequest(id)

	if i.closed.Load() {
		return kvtx.ErrDiscarded
	}

	i.value.Store(nil)
	req := &kvtx_rpc.KvtxIterateRequest{}
	if len(k) != 0 {
		req.Body = &kvtx_rpc.KvtxIterateRequest_Seek{Seek: k}
	} else {
		req.Body = &kvtx_rpc.KvtxIterateRequest_SeekBeginning{SeekBeginning: true}
	}
	if err := i.client.Send(req); err != nil {
		i.storeErr(err)
		return err
	}

	// allow skipping 1 unexpected packet
	for x := 0; x < 2; x++ {
		resp, err := i.client.Recv()
		if err != nil {
			i.storeErr(err)
			return err
		}
		switch b := resp.Body.(type) {
		case *kvtx_rpc.KvtxIterateResponse_Status:
			status := b.Status
			i.status.Store(status)
			var err error
			if errStr := status.GetError(); errStr != "" {
				err = errors.New(errStr)
				_ = i.client.Close()
			}
			return err
		case *kvtx_rpc.KvtxIterateResponse_ReqError:
			err := errors.New(b.ReqError)
			i.storeErr(err)
			return err
		}
	}

	err := errors.New("unexpected iterator response: seek")
	i.storeErr(err)
	return err
}

// Close closes the iterator.
func (i *Iterator) Close() {
	if i.closed.Swap(true) {
		return
	}

	id := i.pipeline.Next()
	i.pipeline.StartRequest(id)
	defer i.pipeline.EndRequest(id)

	i.value.Store(nil)
	i.status.Store(&kvtx_rpc.KvtxIterateStatus{
		Error: kvtx.ErrDiscarded.Error(),
	})

	// timeout the close call
	time.AfterFunc(time.Second, func() {
		_ = i.client.Close()
	})
	// write Close and expect Recv or an error.
	_ = i.client.Send(&kvtx_rpc.KvtxIterateRequest{
		Body: &kvtx_rpc.KvtxIterateRequest_Close{Close: true},
	})
	_, _ = i.client.Recv()

}

// _ is a type assertion
var _ kvtx.Iterator = (*Iterator)(nil)
