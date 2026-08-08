package resource_world

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// TxResource wraps a Tx for resource access.
// It embeds WorldStateResource and adds Commit/Discard operations.
type TxResource struct {
	*WorldStateResource
	tx     world.Tx
	mux    srpc.Mux
	engine world.Engine

	typedResource *TypedObjectResource
	releaseOnce   sync.Once
}

// NewTxResource creates a new TxResource.
//
// engine is optional - if provided, TypedObjectResourceService is registered on the mux.
func NewTxResource(
	le *logrus.Entry,
	b bus.Bus,
	tx world.Tx,
	lookupOp world.LookupOp,
	engine world.Engine,
	opts ...WorldStateResourceOption,
) *TxResource {
	wsResource := NewWorldStateResource(le, b, tx, lookupOp, opts...)
	mux := wsResource.mux.(srpc.Mux)
	txResource := &TxResource{
		WorldStateResource: wsResource,
		tx:                 tx,
		mux:                mux,
		engine:             engine,
	}
	// Register TxResourceService on the same mux
	_ = s4wave_world.SRPCRegisterTxResourceService(mux, txResource)
	// Register TypedObjectResourceService if engine is available
	if engine != nil {
		typedResource := newTypedObjectResourceWithSessionPeerID(
			le,
			b,
			tx,
			engine,
			wsResource.sessionPeerID,
			wsResource.sessionPeerIDBound,
		)
		txResource.typedResource = typedResource
		_ = s4wave_world.SRPCRegisterTypedObjectResourceService(mux, typedResource)
	}
	return txResource
}

// Commit commits the transaction.
func (r *TxResource) Commit(ctx context.Context, req *s4wave_world.CommitRequest) (*s4wave_world.CommitResponse, error) {
	if r.typedResource != nil {
		r.typedResource.Close()
	}
	err := r.tx.Commit(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_world.CommitResponse{}, nil
}

// Discard discards the transaction without committing changes.
func (r *TxResource) Discard(ctx context.Context, req *s4wave_world.DiscardRequest) (*s4wave_world.DiscardResponse, error) {
	r.Release()
	return &s4wave_world.DiscardResponse{}, nil
}

// Release discards the underlying transaction exactly once.
func (r *TxResource) Release() {
	r.releaseOnce.Do(func() {
		if r.typedResource != nil {
			r.typedResource.Close()
		}
		r.tx.Discard()
	})
}

// _ is a type assertion
var _ s4wave_world.SRPCTxResourceServiceServer = (*TxResource)(nil)
