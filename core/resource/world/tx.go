package resource_world

import (
	"context"

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
}

// NewTxResource creates a new TxResource.
//
// engine is optional - if provided, TypedObjectResourceService is registered on the mux.
func NewTxResource(le *logrus.Entry, b bus.Bus, tx world.Tx, lookupOp world.LookupOp, engine world.Engine) *TxResource {
	wsResource := NewWorldStateResource(le, b, tx, lookupOp)
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
		typedResource := NewTypedObjectResource(le, b, tx, engine)
		_ = s4wave_world.SRPCRegisterTypedObjectResourceService(mux, typedResource)
	}
	return txResource
}

// Commit commits the transaction.
func (r *TxResource) Commit(ctx context.Context, req *s4wave_world.CommitRequest) (*s4wave_world.CommitResponse, error) {
	err := r.tx.Commit(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_world.CommitResponse{}, nil
}

// Discard discards the transaction without committing changes.
func (r *TxResource) Discard(ctx context.Context, req *s4wave_world.DiscardRequest) (*s4wave_world.DiscardResponse, error) {
	r.tx.Discard()
	return &s4wave_world.DiscardResponse{}, nil
}

// _ is a type assertion
var _ s4wave_world.SRPCTxResourceServiceServer = ((*TxResource)(nil))
