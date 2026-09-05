package kvtx_rpc_server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_rpc "github.com/s4wave/spacewave/db/kvtx/rpc"
	kvtx_rpc_client "github.com/s4wave/spacewave/db/kvtx/rpc/client"
)

// failedOps injects a storage failure at each operation boundary.
type failedOps struct {
	err error
}

// Size returns the injected count failure.
func (o failedOps) Size(context.Context) (uint64, error) { return 0, o.err }

// Get returns the injected read failure.
func (o failedOps) Get(context.Context, []byte) ([]byte, bool, error) { return nil, false, o.err }

// Exists returns the injected existence failure.
func (o failedOps) Exists(context.Context, []byte) (bool, error) { return false, o.err }

// Set returns the injected write failure.
func (o failedOps) Set(context.Context, []byte, []byte) error { return o.err }

// Delete returns the injected deletion failure.
func (o failedOps) Delete(context.Context, []byte) error { return o.err }

// ScanPrefix returns the injected scan failure.
func (o failedOps) ScanPrefix(context.Context, []byte, func([]byte, []byte) error) error {
	return o.err
}

// ScanPrefixKeys returns the injected key scan failure.
func (o failedOps) ScanPrefixKeys(context.Context, []byte, func([]byte) error) error { return o.err }

// Iterate returns an iterator with the injected failure.
func (o failedOps) Iterate(context.Context, []byte, bool, bool) kvtx.Iterator {
	return kvtx.NewErrIterator(o.err)
}

func TestOperationSnapshotErrorsSurviveRPC(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mux := srpc.NewMux()
	failure := errors.Join(errors.New("changed generation"), kvtx.ErrInvalidSnapshot)
	if err := kvtx_rpc.SRPCRegisterKvtxOps(mux, NewOps(failedOps{err: failure})); err != nil {
		t.Fatal(err)
	}
	transport := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
	client := kvtx_rpc_client.NewOps(kvtx_rpc.NewSRPCKvtxOpsClient(transport), nil)
	checks := map[string]func() error{
		"size": func() error {
			_, err := client.Size(ctx)
			return err
		},
		"get": func() error {
			_, _, err := client.Get(ctx, []byte("key"))
			return err
		},
		"exists": func() error {
			_, err := client.Exists(ctx, []byte("key"))
			return err
		},
		"set":       func() error { return client.Set(ctx, []byte("key"), []byte("value")) },
		"delete":    func() error { return client.Delete(ctx, []byte("key")) },
		"scan":      func() error { return client.ScanPrefix(ctx, nil, func([]byte, []byte) error { return nil }) },
		"scan-keys": func() error { return client.ScanPrefixKeys(ctx, nil, func([]byte) error { return nil }) },
		"iterate": func() error {
			it := client.Iterate(ctx, nil, true, false)
			defer it.Close()
			return it.Err()
		},
	}
	for name, check := range checks {
		t.Run(name, func(t *testing.T) {
			if err := check(); !errors.Is(err, kvtx.ErrInvalidSnapshot) {
				t.Fatalf("operation error = %v, want invalid snapshot", err)
			}
		})
	}
}

// lateFailureOps opens an iterator before exposing a snapshot conflict.
type lateFailureOps struct {
	failedOps
}

// Iterate returns an initially healthy iterator.
func (o lateFailureOps) Iterate(context.Context, []byte, bool, bool) kvtx.Iterator {
	return &lateFailureIterator{Iterator: kvtx.NewErrIterator(nil), failure: o.err}
}

// lateFailureIterator distinguishes request errors from Next status errors.
type lateFailureIterator struct {
	kvtx.Iterator
	failure  error
	advanced bool
}

// Next exposes a conflict in the following status response.
func (i *lateFailureIterator) Next() bool {
	i.advanced = true
	return false
}

// Err reports a conflict only after advancing.
func (i *lateFailureIterator) Err() error {
	if i.advanced {
		return i.failure
	}
	return nil
}

// Value reports a conflict in a request response.
func (i *lateFailureIterator) Value() ([]byte, error) { return nil, i.failure }

// Seek reports a conflict in a request response.
func (i *lateFailureIterator) Seek([]byte) error { return i.failure }

func TestIteratorSnapshotErrorsSurviveRPC(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mux := srpc.NewMux()
	failure := errors.Join(errors.New("changed generation"), kvtx.ErrInvalidSnapshot)
	if err := kvtx_rpc.SRPCRegisterKvtxOps(mux, NewOps(lateFailureOps{failedOps{err: failure}})); err != nil {
		t.Fatal(err)
	}
	transport := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
	client := kvtx_rpc_client.NewOps(kvtx_rpc.NewSRPCKvtxOpsClient(transport), nil)
	checks := map[string]func(kvtx.Iterator) error{
		"next": func(it kvtx.Iterator) error {
			it.Next()
			return it.Err()
		},
		"seek": func(it kvtx.Iterator) error { return it.Seek([]byte("key")) },
		"value": func(it kvtx.Iterator) error {
			_, err := it.Value()
			return err
		},
	}
	for name, check := range checks {
		t.Run(name, func(t *testing.T) {
			it := client.Iterate(ctx, nil, true, false)
			defer it.Close()
			if err := it.Err(); err != nil {
				t.Fatal(err)
			}
			if err := check(it); !errors.Is(err, kvtx.ErrInvalidSnapshot) {
				t.Fatalf("iterator error = %v, want invalid snapshot", err)
			}
		})
	}
}

func TestKeyCountPreservesErrorsForOlderClients(t *testing.T) {
	service := NewOps(failedOps{err: kvtx.ErrInvalidSnapshot})
	response, err := service.KeyCount(context.Background(), &kvtx_rpc.KeyCountRequest{})
	if response != nil || !errors.Is(err, kvtx.ErrInvalidSnapshot) {
		t.Fatalf("old client response=%v error=%v, want RPC error", response, err)
	}
}
