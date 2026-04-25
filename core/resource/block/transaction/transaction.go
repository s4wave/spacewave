package resource_block_transaction

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_block_cursor "github.com/s4wave/spacewave/core/resource/block/cursor"
	"github.com/s4wave/spacewave/db/block"
	s4wave_block_transaction "github.com/s4wave/spacewave/sdk/block/transaction"
	"github.com/sirupsen/logrus"
)

// BlockTransactionResource wraps a block.Transaction for resource access.
type BlockTransactionResource struct {
	le         *logrus.Entry
	b          bus.Bus
	mux        srpc.Invoker
	tx         *block.Transaction
	rootCursor *block.Cursor
}

// NewBlockTransactionResource creates a new BlockTransactionResource.
func NewBlockTransactionResource(le *logrus.Entry, b bus.Bus, tx *block.Transaction, rootCursor *block.Cursor) *BlockTransactionResource {
	btResource := &BlockTransactionResource{le: le, b: b, tx: tx, rootCursor: rootCursor}
	mux := srpc.NewMux()
	_ = s4wave_block_transaction.SRPCRegisterBlockTransactionResourceService(mux, btResource)
	btResource.mux = mux
	return btResource
}

// GetMux returns the rpc mux.
func (r *BlockTransactionResource) GetMux() srpc.Invoker {
	return r.mux
}

// Write writes the transaction to storage and returns the root reference.
func (r *BlockTransactionResource) Write(ctx context.Context, req *s4wave_block_transaction.WriteRequest) (*s4wave_block_transaction.WriteResponse, error) {
	rootRef, rootCursor, err := r.tx.Write(ctx, req.GetClearTree())
	if err != nil {
		return nil, err
	}

	resp := &s4wave_block_transaction.WriteResponse{
		RootRef: rootRef,
	}

	// If we didn't clear the tree, return the root cursor resource
	if !req.GetClearTree() && rootCursor != nil {
		resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
		if err != nil {
			return nil, err
		}

		cursorResource := resource_block_cursor.NewBlockCursorResource(r.le, r.b, r.tx, rootCursor)
		id, err := resourceCtx.AddResource(cursorResource.GetMux(), func() {})
		if err != nil {
			return nil, err
		}
		resp.ResourceId = id
	}

	return resp, nil
}

// GetRootCursor returns the root cursor of the transaction.
func (r *BlockTransactionResource) GetRootCursor(ctx context.Context, req *s4wave_block_transaction.GetRootCursorRequest) (*s4wave_block_transaction.GetRootCursorResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	if r.rootCursor == nil {
		return nil, block.ErrNotFound
	}

	cursorResource := resource_block_cursor.NewBlockCursorResource(r.le, r.b, r.tx, r.rootCursor)
	id, err := resourceCtx.AddResource(cursorResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_block_transaction.GetRootCursorResponse{ResourceId: id}, nil
}

// _ is a type assertion
var _ s4wave_block_transaction.SRPCBlockTransactionResourceServiceServer = ((*BlockTransactionResource)(nil))
