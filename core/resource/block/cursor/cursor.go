package resource_block_cursor

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/hydra-exp/blocktype"
	s4wave_block_cursor "github.com/s4wave/spacewave/sdk/block/cursor"
	"github.com/sirupsen/logrus"
)

// BlockCursorResource wraps a block.Cursor for resource access.
type BlockCursorResource struct {
	le     *logrus.Entry
	mux    srpc.Invoker
	b      bus.Bus
	tx     *block.Transaction
	cursor *block.Cursor
}

// NewBlockCursorResource creates a new BlockCursorResource.
func NewBlockCursorResource(le *logrus.Entry, b bus.Bus, tx *block.Transaction, cursor *block.Cursor) *BlockCursorResource {
	bcResource := &BlockCursorResource{le: le, b: b, tx: tx, cursor: cursor}
	mux := srpc.NewMux()
	_ = s4wave_block_cursor.SRPCRegisterBlockCursorResourceService(mux, bcResource)
	bcResource.mux = mux
	return bcResource
}

// GetMux returns the rpc mux.
func (r *BlockCursorResource) GetMux() srpc.Invoker {
	return r.mux
}

// Fetch fetches the raw block data at the current position.
func (r *BlockCursorResource) Fetch(ctx context.Context, req *s4wave_block_cursor.FetchRequest) (*s4wave_block_cursor.FetchResponse, error) {
	data, found, err := r.cursor.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_block_cursor.FetchResponse{
		Data:  data,
		Found: found,
	}, nil
}

// SetBlock sets the block at the current position.
func (r *BlockCursorResource) SetBlock(ctx context.Context, req *s4wave_block_cursor.SetBlockRequest) (*s4wave_block_cursor.SetBlockResponse, error) {
	blockTypeID := req.GetBlockType()
	if blockTypeID == "" {
		// Legacy path: no block type, use raw bytes
		r.cursor.SetBlock(req.GetData(), req.GetMarkDirty())
		return &s4wave_block_cursor.SetBlockResponse{}, nil
	}

	// Use directive to look up block type
	bt, btRef, err := blocktype.ExLookupBlockType(ctx, r.b, blockTypeID)
	if err != nil {
		return nil, err
	}
	if bt == nil {
		return nil, errors.New("block type not found: " + blockTypeID)
	}
	defer btRef.Release()

	// Construct new block instance
	blk := bt.Constructor()
	if blk == nil {
		return nil, errors.New("block type constructor returned nil")
	}

	// Unmarshal data into block
	if err := blk.UnmarshalBlock(req.GetData()); err != nil {
		return nil, err
	}

	// Set the typed block
	r.cursor.SetBlock(blk, req.GetMarkDirty())
	return &s4wave_block_cursor.SetBlockResponse{}, nil
}

// FollowRef follows a reference field and returns a new cursor.
func (r *BlockCursorResource) FollowRef(ctx context.Context, req *s4wave_block_cursor.FollowRefRequest) (*s4wave_block_cursor.FollowRefResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	newCursor := r.cursor.FollowRef(req.GetRefId(), req.GetBlkRef())
	if newCursor == nil {
		return nil, block.ErrNotFound
	}

	newResource := NewBlockCursorResource(r.le, r.b, r.tx, newCursor)
	id, err := resourceCtx.AddResource(newResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_block_cursor.FollowRefResponse{ResourceId: id}, nil
}

// GetRef gets the current block reference.
func (r *BlockCursorResource) GetRef(ctx context.Context, req *s4wave_block_cursor.GetRefRequest) (*s4wave_block_cursor.GetRefResponse, error) {
	ref := r.cursor.GetRef()
	return &s4wave_block_cursor.GetRefResponse{Ref: ref}, nil
}

// IsDirty checks if the cursor has uncommitted changes.
func (r *BlockCursorResource) IsDirty(ctx context.Context, req *s4wave_block_cursor.IsDirtyRequest) (*s4wave_block_cursor.IsDirtyResponse, error) {
	dirty := r.cursor.IsDirty()
	return &s4wave_block_cursor.IsDirtyResponse{Dirty: dirty}, nil
}

// MarkDirty marks the cursor location dirty for re-writing.
func (r *BlockCursorResource) MarkDirty(ctx context.Context, req *s4wave_block_cursor.MarkDirtyRequest) (*s4wave_block_cursor.MarkDirtyResponse, error) {
	r.cursor.MarkDirty()
	return &s4wave_block_cursor.MarkDirtyResponse{}, nil
}

// GetBlock returns the current loaded block at the position.
func (r *BlockCursorResource) GetBlock(ctx context.Context, req *s4wave_block_cursor.GetBlockRequest) (*s4wave_block_cursor.GetBlockResponse, error) {
	blk, isSubBlock := r.cursor.GetBlock()
	var data []byte
	if blk != nil {
		if marshaler, ok := blk.(block.Block); ok {
			var err error
			data, err = marshaler.MarshalBlock()
			if err != nil {
				return nil, err
			}
		}
	}
	return &s4wave_block_cursor.GetBlockResponse{
		Data:       data,
		IsSubBlock: isSubBlock,
	}, nil
}

// Unmarshal fetches and unmarshals the data to a block.
func (r *BlockCursorResource) Unmarshal(ctx context.Context, req *s4wave_block_cursor.UnmarshalRequest) (*s4wave_block_cursor.UnmarshalResponse, error) {
	data, found, err := r.cursor.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_block_cursor.UnmarshalResponse{
		Data:  data,
		Found: found,
	}, nil
}

// IsSubBlock indicates if the cursor is at a sub-block position.
func (r *BlockCursorResource) IsSubBlock(ctx context.Context, req *s4wave_block_cursor.IsSubBlockRequest) (*s4wave_block_cursor.IsSubBlockResponse, error) {
	isSubBlock := r.cursor.IsSubBlock()
	return &s4wave_block_cursor.IsSubBlockResponse{IsSubBlock: isSubBlock}, nil
}

// FollowSubBlock follows a sub-block reference and returns a new cursor.
func (r *BlockCursorResource) FollowSubBlock(ctx context.Context, req *s4wave_block_cursor.FollowSubBlockRequest) (*s4wave_block_cursor.FollowSubBlockResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	newCursor := r.cursor.FollowSubBlock(req.GetRefId())
	if newCursor == nil {
		return nil, block.ErrNotFound
	}

	newResource := NewBlockCursorResource(r.le, r.b, r.tx, newCursor)
	id, err := resourceCtx.AddResource(newResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_block_cursor.FollowSubBlockResponse{ResourceId: id}, nil
}

// SetAsSubBlock sets the cursor position as a sub-block of another block.
func (r *BlockCursorResource) SetAsSubBlock(ctx context.Context, req *s4wave_block_cursor.SetAsSubBlockRequest) (*s4wave_block_cursor.SetAsSubBlockResponse, error) {
	// TODO: requires cross-resource communication API in ResourceClientContext
	_ = req
	return nil, errors.New("SetAsSubBlock not implemented: requires cross-resource communication")
}

// ClearRef removes the reference handle to the given ref ID.
func (r *BlockCursorResource) ClearRef(ctx context.Context, req *s4wave_block_cursor.ClearRefRequest) (*s4wave_block_cursor.ClearRefResponse, error) {
	r.cursor.ClearRef(req.GetRefId())
	return &s4wave_block_cursor.ClearRefResponse{}, nil
}

// ClearAllRefs clears all references.
func (r *BlockCursorResource) ClearAllRefs(ctx context.Context, req *s4wave_block_cursor.ClearAllRefsRequest) (*s4wave_block_cursor.ClearAllRefsResponse, error) {
	r.cursor.ClearAllRefs()
	return &s4wave_block_cursor.ClearAllRefsResponse{}, nil
}

// SetRef sets a block reference to the handle at the cursor.
func (r *BlockCursorResource) SetRef(ctx context.Context, req *s4wave_block_cursor.SetRefRequest) (*s4wave_block_cursor.SetRefResponse, error) {
	// TODO: requires cross-resource communication API in ResourceClientContext
	_ = req
	return nil, errors.New("SetRef not implemented: requires cross-resource communication")
}

// GetExistingRef checks if the reference has been traversed already.
func (r *BlockCursorResource) GetExistingRef(ctx context.Context, req *s4wave_block_cursor.GetExistingRefRequest) (*s4wave_block_cursor.GetExistingRefResponse, error) {
	existingCursor := r.cursor.GetExistingRef(req.GetRefId())
	if existingCursor == nil {
		return &s4wave_block_cursor.GetExistingRefResponse{ResourceId: 0}, nil
	}

	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	newResource := NewBlockCursorResource(r.le, r.b, r.tx, existingCursor)
	id, err := resourceCtx.AddResource(newResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_block_cursor.GetExistingRefResponse{ResourceId: id}, nil
}

// GetAllRefs returns cursors to all references.
func (r *BlockCursorResource) GetAllRefs(ctx context.Context, req *s4wave_block_cursor.GetAllRefsRequest) (*s4wave_block_cursor.GetAllRefsResponse, error) {
	allRefs, err := r.cursor.GetAllRefs(req.GetExistingOnly())
	if err != nil {
		return nil, err
	}

	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	refs := make(map[uint32]uint32)
	for refID, cursor := range allRefs {
		newResource := NewBlockCursorResource(r.le, r.b, r.tx, cursor)
		id, err := resourceCtx.AddResource(newResource.GetMux(), func() {})
		if err != nil {
			return nil, err
		}
		refs[refID] = id
	}

	return &s4wave_block_cursor.GetAllRefsResponse{Refs: refs}, nil
}

// Detach clones the cursor position.
func (r *BlockCursorResource) Detach(ctx context.Context, req *s4wave_block_cursor.DetachRequest) (*s4wave_block_cursor.DetachResponse, error) {
	newCursor := r.cursor.Detach(req.GetKeepRefs())
	if newCursor == nil {
		return nil, block.ErrNotFound
	}

	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	newResource := NewBlockCursorResource(r.le, r.b, r.tx, newCursor)
	id, err := resourceCtx.AddResource(newResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_block_cursor.DetachResponse{ResourceId: id}, nil
}

// DetachTransaction creates a new ephemeral transaction rooted at the cursor.
func (r *BlockCursorResource) DetachTransaction(ctx context.Context, req *s4wave_block_cursor.DetachTransactionRequest) (*s4wave_block_cursor.DetachTransactionResponse, error) {
	newCursor := r.cursor.DetachTransaction()
	if newCursor == nil {
		return nil, block.ErrNotFound
	}

	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	newResource := NewBlockCursorResource(r.le, r.b, newCursor.GetTransaction(), newCursor)
	id, err := resourceCtx.AddResource(newResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_block_cursor.DetachTransactionResponse{ResourceId: id}, nil
}

// DetachRecursive clones the cursor position and all referenced positions.
func (r *BlockCursorResource) DetachRecursive(ctx context.Context, req *s4wave_block_cursor.DetachRecursiveRequest) (*s4wave_block_cursor.DetachRecursiveResponse, error) {
	newCursor := r.cursor.DetachRecursive(req.GetDetachTx(), req.GetCloneBlocks(), req.GetMarkDirty())
	if newCursor == nil {
		return nil, block.ErrNotFound
	}

	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	newTx := r.tx
	if req.GetDetachTx() {
		newTx = newCursor.GetTransaction()
	}

	newResource := NewBlockCursorResource(r.le, r.b, newTx, newCursor)
	id, err := resourceCtx.AddResource(newResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_block_cursor.DetachRecursiveResponse{ResourceId: id}, nil
}

// Parents returns new cursors pointing to the parent blocks.
func (r *BlockCursorResource) Parents(ctx context.Context, req *s4wave_block_cursor.ParentsRequest) (*s4wave_block_cursor.ParentsResponse, error) {
	parents := r.cursor.Parents()
	if parents == nil {
		return &s4wave_block_cursor.ParentsResponse{ParentResourceIds: nil}, nil
	}

	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	parentIDs := make([]uint32, 0, len(parents))
	for _, parent := range parents {
		newResource := NewBlockCursorResource(r.le, r.b, r.tx, parent)
		id, err := resourceCtx.AddResource(newResource.GetMux(), func() {})
		if err != nil {
			return nil, err
		}
		parentIDs = append(parentIDs, id)
	}

	return &s4wave_block_cursor.ParentsResponse{ParentResourceIds: parentIDs}, nil
}

// _ is a type assertion
var _ s4wave_block_cursor.SRPCBlockCursorResourceServiceServer = ((*BlockCursorResource)(nil))
