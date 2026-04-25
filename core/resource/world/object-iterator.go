package resource_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// ObjectIteratorResource wraps an ObjectIterator for resource access.
type ObjectIteratorResource struct {
	le   *logrus.Entry
	b    bus.Bus
	mux  srpc.Invoker
	iter world.ObjectIterator
}

// NewObjectIteratorResource creates a new ObjectIteratorResource.
func NewObjectIteratorResource(le *logrus.Entry, b bus.Bus, iter world.ObjectIterator) *ObjectIteratorResource {
	iterResource := &ObjectIteratorResource{le: le, b: b, iter: iter}
	mux := srpc.NewMux()
	_ = s4wave_world.SRPCRegisterObjectIteratorResourceService(mux, iterResource)
	iterResource.mux = mux
	return iterResource
}

// GetMux returns the rpc mux.
func (r *ObjectIteratorResource) GetMux() srpc.Invoker {
	return r.mux
}

// Err returns any error that has closed the iterator.
func (r *ObjectIteratorResource) Err(ctx context.Context, req *s4wave_world.ErrRequest) (*s4wave_world.ErrResponse, error) {
	err := r.iter.Err()
	if err != nil {
		return &s4wave_world.ErrResponse{Error: err.Error()}, nil
	}
	return &s4wave_world.ErrResponse{}, nil
}

// Valid returns if the iterator points to a valid entry.
func (r *ObjectIteratorResource) Valid(ctx context.Context, req *s4wave_world.ValidRequest) (*s4wave_world.ValidResponse, error) {
	return &s4wave_world.ValidResponse{Valid: r.iter.Valid()}, nil
}

// Key returns the current entry key, or empty if not valid.
func (r *ObjectIteratorResource) Key(ctx context.Context, req *s4wave_world.KeyRequest) (*s4wave_world.KeyResponse, error) {
	return &s4wave_world.KeyResponse{ObjectKey: r.iter.Key()}, nil
}

// Next advances to the next entry and returns Valid.
func (r *ObjectIteratorResource) Next(ctx context.Context, req *s4wave_world.NextRequest) (*s4wave_world.NextResponse, error) {
	return &s4wave_world.NextResponse{Valid: r.iter.Next()}, nil
}

// Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
func (r *ObjectIteratorResource) Seek(ctx context.Context, req *s4wave_world.SeekRequest) (*s4wave_world.SeekResponse, error) {
	r.iter.Seek(req.GetObjectKey())
	return &s4wave_world.SeekResponse{}, nil
}

// Close closes the iterator.
func (r *ObjectIteratorResource) Close(ctx context.Context, req *s4wave_world.CloseRequest) (*s4wave_world.CloseResponse, error) {
	r.iter.Close()
	return &s4wave_world.CloseResponse{}, nil
}

// _ is a type assertion
var _ s4wave_world.SRPCObjectIteratorResourceServiceServer = ((*ObjectIteratorResource)(nil))
