package sdk_world_engine

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// SDKObjectIterator implements world.ObjectIterator over SRPC by
// delegating to ObjectIteratorResourceService calls on a remote resource.
//
// SDKObjectIterator functions are NOT thread safe, use from one goroutine at a time.
//
// Note: the world.ObjectIterator interface does not pass context on
// its methods, but SRPC calls require one. The context from the
// IterateObjects call is captured and used for all subsequent calls.
type SDKObjectIterator struct {
	ctx     context.Context
	ref     resource_client.ResourceRef
	service s4wave_world.SRPCObjectIteratorResourceServiceClient
	err     error
}

// NewSDKObjectIterator creates a new SDKObjectIterator wrapping a resource reference.
// The context is used for all subsequent SRPC calls on the iterator.
func NewSDKObjectIterator(ctx context.Context, ref resource_client.ResourceRef) (*SDKObjectIterator, error) {
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	return &SDKObjectIterator{
		ctx:     ctx,
		ref:     ref,
		service: s4wave_world.NewSRPCObjectIteratorResourceServiceClient(srpcClient),
	}, nil
}

// Err returns any error that has closed the iterator.
func (it *SDKObjectIterator) Err() error {
	if it.err != nil {
		return it.err
	}
	if it.service == nil {
		return it.err
	}
	resp, err := it.service.Err(it.ctx, &s4wave_world.ErrRequest{})
	if err != nil {
		it.err = err
		return err
	}
	if resp.Error != "" {
		it.err = &iterError{msg: resp.Error}
		return it.err
	}
	return nil
}

// Valid returns if the iterator points to a valid entry.
func (it *SDKObjectIterator) Valid() bool {
	if it.err != nil || it.service == nil {
		return false
	}
	resp, err := it.service.Valid(it.ctx, &s4wave_world.ValidRequest{})
	if err != nil {
		it.err = err
		return false
	}
	return resp.Valid
}

// Key returns the current entry key, or empty string if not valid.
func (it *SDKObjectIterator) Key() string {
	if it.err != nil || it.service == nil {
		return ""
	}
	resp, err := it.service.Key(it.ctx, &s4wave_world.KeyRequest{})
	if err != nil {
		it.err = err
		return ""
	}
	return resp.ObjectKey
}

// Next advances to the next entry and returns Valid.
func (it *SDKObjectIterator) Next() bool {
	if it.err != nil || it.service == nil {
		return false
	}
	resp, err := it.service.Next(it.ctx, &s4wave_world.NextRequest{})
	if err != nil {
		it.err = err
		return false
	}
	return resp.Valid
}

// Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
// Pass empty string to seek to the beginning (or end if reversed).
func (it *SDKObjectIterator) Seek(k string) error {
	if it.err != nil {
		return it.err
	}
	if it.service == nil {
		return it.err
	}
	_, err := it.service.Seek(it.ctx, &s4wave_world.SeekRequest{ObjectKey: k})
	if err != nil {
		it.err = err
	}
	return err
}

// Close releases the iterator.
func (it *SDKObjectIterator) Close() {
	if it.service != nil {
		_, _ = it.service.Close(it.ctx, &s4wave_world.CloseRequest{})
	}
	if it.ref != nil {
		it.ref.Release()
	}
}

// iterError wraps a string error message from the server.
type iterError struct {
	msg string
}

// Error returns the error message.
func (e *iterError) Error() string {
	return e.msg
}

// _ is a type assertion
var _ world.ObjectIterator = (*SDKObjectIterator)(nil)
