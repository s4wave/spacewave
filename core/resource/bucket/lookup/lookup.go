package resource_bucket_lookup

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_block_cursor "github.com/s4wave/spacewave/core/resource/block/cursor"
	resource_block_transaction "github.com/s4wave/spacewave/core/resource/block/transaction"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/blocktype"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	s4wave_bucket_lookup "github.com/s4wave/spacewave/sdk/bucket/lookup"
	"github.com/sirupsen/logrus"
)

// BucketLookupCursorResource wraps a bucket_lookup.Cursor for resource access.
type BucketLookupCursorResource struct {
	le     *logrus.Entry
	b      bus.Bus
	mux    srpc.Invoker
	cursor *bucket_lookup.Cursor
	owned  *world.OwnedLookupCursor
}

// NewBucketLookupCursorResource creates a new BucketLookupCursorResource.
func NewBucketLookupCursorResource(le *logrus.Entry, b bus.Bus, cursor *bucket_lookup.Cursor) *BucketLookupCursorResource {
	return (&BucketLookupCursorResource{le: le, b: b, cursor: cursor}).initMux()
}

// NewOwnedBucketLookupCursorResource creates a resource backed by an owned
// cursor whose authority can be retained by child resources.
func NewOwnedBucketLookupCursorResource(
	le *logrus.Entry,
	b bus.Bus,
	owned *world.OwnedLookupCursor,
) *BucketLookupCursorResource {
	return (&BucketLookupCursorResource{
		le:     le,
		b:      b,
		cursor: owned.Cursor(),
		owned:  owned,
	}).initMux()
}

func (r *BucketLookupCursorResource) initMux() *BucketLookupCursorResource {
	mux := srpc.NewMux()
	_ = s4wave_bucket_lookup.SRPCRegisterBucketLookupCursorResourceService(mux, r)
	r.mux = mux
	return r
}

// GetMux returns the rpc mux.
func (r *BucketLookupCursorResource) GetMux() srpc.Invoker {
	return r.mux
}

// GetRef returns the current object reference.
func (r *BucketLookupCursorResource) GetRef(ctx context.Context, req *s4wave_bucket_lookup.GetRefRequest) (*s4wave_bucket_lookup.GetRefResponse, error) {
	ref := r.cursor.GetRefWithOpArgs()
	return &s4wave_bucket_lookup.GetRefResponse{Ref: ref}, nil
}

// FollowRef follows an object reference and returns a new cursor.
func (r *BucketLookupCursorResource) FollowRef(ctx context.Context, req *s4wave_bucket_lookup.FollowRefRequest) (*s4wave_bucket_lookup.FollowRefResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	if r.owned != nil {
		owned, err := r.owned.FollowRef(ctx, req.GetRef())
		if err != nil {
			return nil, err
		}
		newResource := NewOwnedBucketLookupCursorResource(r.le, r.b, owned)
		id, err := resourceCtx.AddResource(newResource.GetMux(), owned.Release)
		if err != nil {
			owned.Release()
			return nil, err
		}
		return &s4wave_bucket_lookup.FollowRefResponse{ResourceId: id}, nil
	}

	newCursor, err := r.cursor.FollowRef(ctx, req.GetRef())
	if err != nil {
		return nil, err
	}
	newResource := NewBucketLookupCursorResource(r.le, r.b, newCursor)
	id, err := resourceCtx.AddResource(newResource.GetMux(), newCursor.Release)
	if err != nil {
		newCursor.Release()
		return nil, err
	}
	return &s4wave_bucket_lookup.FollowRefResponse{ResourceId: id}, nil
}

// GetBlock gets a block by reference.
func (r *BucketLookupCursorResource) GetBlock(ctx context.Context, req *s4wave_bucket_lookup.GetBlockRequest) (*s4wave_bucket_lookup.GetBlockResponse, error) {
	data, found, err := r.cursor.GetBlock(ctx, req.GetRef())
	if err != nil {
		return nil, err
	}
	block.RecordResourceGetBlock(ctx, req.GetRef(), found, len(data))
	return &s4wave_bucket_lookup.GetBlockResponse{
		Data:  data,
		Found: found,
	}, nil
}

// PutBlock puts a block.
func (r *BucketLookupCursorResource) PutBlock(ctx context.Context, req *s4wave_bucket_lookup.PutBlockRequest) (*s4wave_bucket_lookup.PutBlockResponse, error) {
	ref, existed, err := r.cursor.PutBlock(ctx, req.GetData(), req.GetOpts())
	if err != nil {
		return nil, err
	}
	return &s4wave_bucket_lookup.PutBlockResponse{
		Ref:     ref,
		Existed: existed,
	}, nil
}

// PutBlockBatch puts a batch of blocks.
func (r *BucketLookupCursorResource) PutBlockBatch(ctx context.Context, req *s4wave_bucket_lookup.PutBlockBatchRequest) (*s4wave_bucket_lookup.PutBlockBatchResponse, error) {
	entries := make([]*block.PutBatchEntry, len(req.GetEntries()))
	for i, entry := range req.GetEntries() {
		entries[i] = &block.PutBatchEntry{
			Ref:       entry.GetRef(),
			Data:      entry.GetData(),
			Refs:      entry.GetRefs(),
			Tombstone: entry.GetTombstone(),
		}
	}
	if err := r.cursor.PutBlockBatch(ctx, entries); err != nil {
		return nil, err
	}
	return &s4wave_bucket_lookup.PutBlockBatchResponse{}, nil
}

// GetBlockExistsBatch checks whether each block exists.
func (r *BucketLookupCursorResource) GetBlockExistsBatch(ctx context.Context, req *s4wave_bucket_lookup.GetBlockExistsBatchRequest) (*s4wave_bucket_lookup.GetBlockExistsBatchResponse, error) {
	found, err := r.cursor.GetBlockExistsBatch(ctx, req.GetRefs())
	if err != nil {
		return nil, err
	}
	return &s4wave_bucket_lookup.GetBlockExistsBatchResponse{Found: found}, nil
}

// BuildTransaction builds a transaction at the current position.
func (r *BucketLookupCursorResource) BuildTransaction(ctx context.Context, req *s4wave_bucket_lookup.BuildTransactionRequest) (*s4wave_bucket_lookup.BuildTransactionResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	txOwner, cursorOwner, err := r.cloneTransactionOwners()
	if err != nil {
		return nil, err
	}
	tx, rootCursor := r.cursor.BuildTransaction(req.GetPutOpts())

	txResource := resource_block_transaction.NewBlockTransactionResourceWithRetain(
		r.le,
		r.b,
		tx,
		rootCursor,
		retainOwnedCursor(txOwner),
	)
	txRelease := func() {}
	if txOwner != nil {
		txRelease = txOwner.Release
	}
	txID, err := resourceCtx.AddResource(txResource.GetMux(), txRelease)
	if err != nil {
		txRelease()
		if cursorOwner != nil {
			cursorOwner.Release()
		}
		return nil, err
	}

	cursorResource := resource_block_cursor.NewBlockCursorResourceWithRetain(
		r.le,
		r.b,
		tx,
		rootCursor,
		retainOwnedCursor(cursorOwner),
	)
	cursorRelease := func() {}
	if cursorOwner != nil {
		cursorRelease = cursorOwner.Release
	}
	cursorID, err := resourceCtx.AddResource(cursorResource.GetMux(), cursorRelease)
	if err != nil {
		cursorRelease()
		resourceCtx.ReleaseResource(txID)
		return nil, err
	}

	return &s4wave_bucket_lookup.BuildTransactionResponse{
		TransactionResourceId: txID,
		CursorResourceId:      cursorID,
	}, nil
}

// BuildTransactionAtRef builds a transaction at a specific block reference.
func (r *BucketLookupCursorResource) BuildTransactionAtRef(ctx context.Context, req *s4wave_bucket_lookup.BuildTransactionAtRefRequest) (*s4wave_bucket_lookup.BuildTransactionAtRefResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	txOwner, cursorOwner, err := r.cloneTransactionOwners()
	if err != nil {
		return nil, err
	}
	tx, rootCursor := r.cursor.BuildTransactionAtRef(req.GetPutOpts(), req.GetRef())

	txResource := resource_block_transaction.NewBlockTransactionResourceWithRetain(
		r.le,
		r.b,
		tx,
		rootCursor,
		retainOwnedCursor(txOwner),
	)
	txRelease := func() {}
	if txOwner != nil {
		txRelease = txOwner.Release
	}
	txID, err := resourceCtx.AddResource(txResource.GetMux(), txRelease)
	if err != nil {
		txRelease()
		if cursorOwner != nil {
			cursorOwner.Release()
		}
		return nil, err
	}

	cursorResource := resource_block_cursor.NewBlockCursorResourceWithRetain(
		r.le,
		r.b,
		tx,
		rootCursor,
		retainOwnedCursor(cursorOwner),
	)
	cursorRelease := func() {}
	if cursorOwner != nil {
		cursorRelease = cursorOwner.Release
	}
	cursorID, err := resourceCtx.AddResource(cursorResource.GetMux(), cursorRelease)
	if err != nil {
		cursorRelease()
		resourceCtx.ReleaseResource(txID)
		return nil, err
	}

	return &s4wave_bucket_lookup.BuildTransactionAtRefResponse{
		TransactionResourceId: txID,
		CursorResourceId:      cursorID,
	}, nil
}

// Clone clones the cursor.
func (r *BucketLookupCursorResource) Clone(ctx context.Context, req *s4wave_bucket_lookup.CloneRequest) (*s4wave_bucket_lookup.CloneResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	if r.owned != nil {
		owned, err := r.owned.Clone()
		if err != nil {
			return nil, err
		}
		clonedResource := NewOwnedBucketLookupCursorResource(r.le, r.b, owned)
		id, err := resourceCtx.AddResource(clonedResource.GetMux(), owned.Release)
		if err != nil {
			owned.Release()
			return nil, err
		}
		return &s4wave_bucket_lookup.CloneResponse{ResourceId: id}, nil
	}

	cloned := r.cursor.Clone()
	clonedResource := NewBucketLookupCursorResource(r.le, r.b, cloned)
	id, err := resourceCtx.AddResource(clonedResource.GetMux(), cloned.Release)
	if err != nil {
		cloned.Release()
		return nil, err
	}
	return &s4wave_bucket_lookup.CloneResponse{ResourceId: id}, nil
}

// Release releases the cursor resources.
func (r *BucketLookupCursorResource) Release(ctx context.Context, req *s4wave_bucket_lookup.ReleaseRequest) (*s4wave_bucket_lookup.ReleaseResponse, error) {
	if r.owned != nil {
		r.owned.Release()
	} else {
		r.cursor.Release()
	}
	return &s4wave_bucket_lookup.ReleaseResponse{}, nil
}

func retainOwnedCursor(owned *world.OwnedLookupCursor) resource_block_cursor.RetainAuthorityFunc {
	if owned == nil {
		return nil
	}
	return func() (func(), error) {
		child, err := owned.Clone()
		if err != nil {
			return nil, err
		}
		return child.Release, nil
	}
}

func (r *BucketLookupCursorResource) cloneTransactionOwners() (*world.OwnedLookupCursor, *world.OwnedLookupCursor, error) {
	if r.owned == nil {
		return nil, nil, nil
	}
	txOwner, err := r.owned.Clone()
	if err != nil {
		return nil, nil, err
	}
	cursorOwner, err := r.owned.Clone()
	if err != nil {
		txOwner.Release()
		return nil, nil, err
	}
	return txOwner, cursorOwner, nil
}

// Unmarshal fetches and unmarshals a block at the given reference.
func (r *BucketLookupCursorResource) Unmarshal(ctx context.Context, req *s4wave_bucket_lookup.UnmarshalRequest) (*s4wave_bucket_lookup.UnmarshalResponse, error) {
	data := req.GetData()
	ref := req.GetRef()
	blockTypeID := req.GetBlockType()

	if blockTypeID != "" && len(data) == 0 {
		bt, btRef, err := blocktype.ExLookupBlockType(ctx, r.b, blockTypeID)
		if err != nil {
			return nil, err
		}
		if bt == nil {
			return nil, errors.New("block type not found: " + blockTypeID)
		}
		if btRef != nil {
			defer btRef.Release()
		}

		cursor := r.cursor
		if ref != nil && !ref.GetEmpty() {
			followed, err := r.cursor.FollowRef(ctx, ref)
			if err != nil {
				return nil, err
			}
			defer followed.Release()
			cursor = followed
		}
		if cursor.GetRef().GetRootRef().GetEmpty() {
			return &s4wave_bucket_lookup.UnmarshalResponse{Found: false}, nil
		}
		blk, err := cursor.Unmarshal(ctx, bt.Constructor)
		if err != nil {
			return nil, err
		}
		if blk == nil {
			return &s4wave_bucket_lookup.UnmarshalResponse{Found: false}, nil
		}
		data, err := blk.MarshalBlock()
		if err != nil {
			return nil, err
		}
		return &s4wave_bucket_lookup.UnmarshalResponse{
			Data:  data,
			Found: true,
		}, nil
	}

	// If no data provided, fetch the block
	if len(data) == 0 {
		if ref == nil {
			ref = r.cursor.GetRef()
		}
		rootRef := ref.GetRootRef()
		if rootRef.GetEmpty() {
			return &s4wave_bucket_lookup.UnmarshalResponse{Found: false}, nil
		}
		var found bool
		var err error
		data, found, err = r.cursor.GetBlock(ctx, rootRef)
		if err != nil {
			return nil, err
		}
		if !found {
			return &s4wave_bucket_lookup.UnmarshalResponse{Found: false}, nil
		}
	}

	return &s4wave_bucket_lookup.UnmarshalResponse{
		Data:  data,
		Found: true,
	}, nil
}

// _ is a type assertion
var _ s4wave_bucket_lookup.SRPCBucketLookupCursorResourceServiceServer = ((*BucketLookupCursorResource)(nil))
