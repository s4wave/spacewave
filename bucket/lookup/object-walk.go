package bucket_lookup

import (
	"context"
	"sort"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/util/conc"
	"github.com/pkg/errors"
)

// WalkObjectBlocksCb is the callback called by WalkObjectBlocks.
type WalkObjectBlocksCb func(entry *WalkObjectBlocksEntry) (cntu bool, err error)

// WalkObjectBlocksValue is a value passed to the callback for WalkObjectBlocks.
type WalkObjectBlocksEntry struct {
	// Depth is the number of refs we traversed to get to this entry.
	// Depth starts at 0 for the root reference.
	// Sub-blocks are counted in the depth.
	Depth int
	// RefID is the reference ID of this block or sub-block.
	// Zero if there is no parent.
	RefID uint32
	// Ref is the block ref for this entry.
	// If IsSubBlock this is the ref of the parent block.
	// If GetBlockRefs returns an error, ref=nil and err=error.
	Ref *block.BlockRef
	// Ctor is the constructor for the block at this entry.
	// May be nil if constructor was not set or IsSubBlock.
	Ctor block.Ctor
	// Blk is the block or sub-block at this entry.
	// May be nil if ctor was nil.
	Blk interface{}
	// IsSubBlock indicates this is a sub-block.
	// If set, found is also true and err is nil.
	IsSubBlock bool
	// Err is any error fetching the block.
	// If err is set we will not traverse blk.
	Err error
	// Found indicates if the block was found in storage or not.
	Found bool
	// Data is the data at this entry.
	// May be empty if Err != nil or !Found or IsSubBlock
	// May be empty if depth == 0 and blk != nil
	Data []byte
}

// NewWalkObjectBlocksWithRef constructs a new walk tree entry with a root ref.
func NewWalkObjectBlocksWithRef(ref *block.BlockRef, ctor block.Ctor) *WalkObjectBlocksEntry {
	return &WalkObjectBlocksEntry{Ref: ref, Ctor: ctor}
}

// NewWalkObjectBlocksWithSubBlock constructs a new walk tree entry with a sub-block.
func NewWalkObjectBlocksWithSubBlock(subBlock block.SubBlock) *WalkObjectBlocksEntry {
	return &WalkObjectBlocksEntry{
		Blk:        subBlock,
		Found:      true,
		IsSubBlock: true,
	}
}

// NewWalkObjectBlocksWithBlock constructs a new walk tree entry with a root block.
func NewWalkObjectBlocksWithBlock(blk block.Block) *WalkObjectBlocksEntry {
	return &WalkObjectBlocksEntry{
		Blk:        blk,
		Found:      true,
		IsSubBlock: true,
	}
}

// NewWalkObjectBlocksWithError constructs a new walk tree entry with an error.
func NewWalkObjectBlocksWithError(err error) *WalkObjectBlocksEntry {
	return &WalkObjectBlocksEntry{
		Err: err,
	}
}

// NewWalkObjectBlocksWithData constructs a new walk tree entry with an unparsed block.
func NewWalkObjectBlocksWithData(data []byte, ctor block.Ctor) *WalkObjectBlocksEntry {
	return &WalkObjectBlocksEntry{
		Data:  data,
		Found: len(data) != 0,
		Ctor:  ctor,
	}
}

// WalkObjectBlocks concurrently walks the tree of object refs calling a callback.
//
// The concurrency limit controls how many concurrent callbacks can be called.
// If maxConcurrency <= 0, has no limit on concurrent callbacks.
//
// If the context is canceled, returns context.Canceled.
// Any error fetching a block other than context canceled will be passed to the cb.
// If a block was not found, the callback is called with ErrNotFound.
//
// If alwaysDecode is false, skips decoding blocks if they have no refs.
// This can save time significantly if you don't need to check the block contents.
//
// The callback can modify the block or change the block / ref in the Entry
// before returning to adjust the next sub-graph that will be traversed.
//
// The callback will be called concurrently and must be concurrency-safe.
// The callback will be called with a nil block if the block had no ctor.
// If the callback returns an error, stops execution and returns that error immediately.
// If the callback returns false for continue, skips that sub-graph of objects.
// If the callback is nil, skips calling the callback.
func WalkObjectBlocks(
	ctx context.Context,
	root *WalkObjectBlocksEntry,
	cb WalkObjectBlocksCb,
	readBkt bucket.Bucket,
	readXfrm block.Transformer,
	maxConcurrency int,
	alwaysDecode bool,
) error {
	if root == nil {
		return nil
	}
	errCh := make(chan error, 1)
	handleErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}
	queue := root.BuildConcurrentQueue(
		ctx,
		handleErr,
		cb,
		readBkt, readXfrm,
		maxConcurrency,
		alwaysDecode,
	)
	return queue.WaitIdle(ctx, errCh)
}

// walkState contains state for walking a tree of blocks.
type walkState struct {
	readBkt      bucket.Bucket
	readXfrm     block.Transformer
	enqueue      func(...func())
	handleErr    func(err error)
	cb           WalkObjectBlocksCb
	alwaysDecode bool
}

// BuildConcurrentQueue builds and starts a concurrent queue to walk the tree.
func (e *WalkObjectBlocksEntry) BuildConcurrentQueue(
	ctx context.Context,
	handleErr func(err error),
	cb WalkObjectBlocksCb,
	readBkt bucket.Bucket,
	readXfrm block.Transformer,
	maxConcurrency int,
	alwaysDecode bool,
) *conc.ConcurrentQueue {
	queue := conc.NewConcurrentQueue(maxConcurrency)
	enqueue := func(f ...func()) {
		_, _ = queue.Enqueue(f...)
	}
	state := &walkState{
		readBkt:      readBkt,
		readXfrm:     readXfrm,
		enqueue:      enqueue,
		handleErr:    handleErr,
		cb:           cb,
		alwaysDecode: alwaysDecode,
	}
	enqueue(
		e.buildVisitFn(
			ctx,
			state,
		),
	)
	return queue
}

// buildVisitFn builds the function called by the concurrent queue in WalkObjectBlocks.
func (e *WalkObjectBlocksEntry) buildVisitFn(ctx context.Context, st *walkState) func() {
	return func() {
		if ctx.Err() != nil {
			return
		}

		if !e.Found && e.Err == nil && !e.IsSubBlock && !e.Ref.GetEmpty() {
			// returns nil, false, nil if reference was empty.
			// returns nil, false, ErrNotFound if reference was not found.
			e.Data, e.Found, e.Err = st.readBkt.GetBlock(ctx, e.Ref)
		}

		if e.Found && e.Ctor != nil && e.Err == nil && !e.IsSubBlock {
			err := e.decodeBlock(st.alwaysDecode, st.readXfrm)
			if err != nil && e.Err == nil {
				e.Err = err
			}
		}

		var cntu bool
		var err error
		if st.cb != nil {
			cntu, err = st.cb(e)
		} else {
			cntu, err = true, e.Err
		}
		if err != nil {
			st.handleErr(err)
			return
		}
		if !cntu || e.Blk == nil || e.Err != nil {
			return
		}

		// Defer enqueuing next entries until we return.
		var toEnqueue []*WalkObjectBlocksEntry
		enqueueEntry := func(ent *WalkObjectBlocksEntry) {
			ent.Depth = e.Depth + 1
			toEnqueue = append(toEnqueue, ent)
		}

		// enqueue any sub-blocks
		if withSubBlocks, ok := e.Blk.(block.BlockWithSubBlocks); ok {
			for refID, subBlk := range withSubBlocks.GetSubBlocks() {
				if subBlk != nil && !subBlk.IsNil() {
					enqueueEntry(&WalkObjectBlocksEntry{
						Depth:      e.Depth + 1,
						Ref:        e.Ref,
						RefID:      refID,
						Blk:        subBlk,
						IsSubBlock: true,
						Found:      true,
					})
				}
			}
		}

		// enqueue any block refs
		withRefs, ok := e.Blk.(block.BlockWithRefs)
		if ok {
			blkRefs, err := withRefs.GetBlockRefs()
			if err != nil {
				blkRefs = nil
				enqueueEntry(&WalkObjectBlocksEntry{
					Blk:        e.Blk,
					Err:        err,
					IsSubBlock: e.IsSubBlock,
					Found:      true,
					Data:       e.Data,
				})
			}
			for refID, ref := range blkRefs {
				enqueueEntry(&WalkObjectBlocksEntry{
					RefID: refID,
					Ref:   ref,
					Ctor:  withRefs.GetBlockRefCtor(refID),
				})
			}
		}

		// enqueue the toEnqueue set after sorting
		if len(toEnqueue) == 0 {
			return
		}

		sort.SliceStable(toEnqueue, func(i, j int) bool {
			return toEnqueue[i].RefID < toEnqueue[j].RefID
		})

		enqFns := make([]func(), len(toEnqueue))
		for i, enq := range toEnqueue {
			enqFns[i] = enq.buildVisitFn(ctx, st)
		}
		st.enqueue(enqFns...)
	}
}

// decodeBlock conditionally decodes the block.
func (e *WalkObjectBlocksEntry) decodeBlock(alwaysDecode bool, readXfrm block.Transformer) error {
	if !e.Found || e.Ctor == nil || e.Err != nil {
		return nil
	}
	decodeBlk := e.Ctor()

	// skip decoding / processing block if it has no sub-blocks or refs
	if !alwaysDecode {
		switch decodeBlk.(type) {
		case block.BlockWithRefs:
		case block.BlockWithSubBlocks:
		default:
			return nil
		}
	}

	// transform data
	dat, err := readXfrm.DecodeBlock(e.Data)
	if err != nil {
		if err != context.Canceled {
			err = errors.Wrapf(err, "decode block: %s", e.Ref.MarshalString())
		}
		return err
	}

	// unmarshal the block
	if err := decodeBlk.UnmarshalBlock(dat); err != nil {
		if err != context.Canceled {
			err = errors.Wrapf(err, "unmarshal block: %s", e.Ref.MarshalString())
		}
		return err
	}
	e.Blk = decodeBlk
	return nil
}
