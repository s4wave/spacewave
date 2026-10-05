package bucket_lookup

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/conc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

type copyWorkerTraceContextKey struct{}

func withCopyWorkerTrace(ctx context.Context) context.Context {
	return context.WithValue(ctx, copyWorkerTraceContextKey{}, true)
}

func copyWorkerTraceEnabled(ctx context.Context) bool {
	enabled, _ := ctx.Value(copyWorkerTraceContextKey{}).(bool)
	return enabled
}

// WalkObjectBlocksCb is the callback called by WalkObjectBlocks.
type WalkObjectBlocksCb func(entry *WalkObjectBlocksEntry) (cntu bool, err error)

// WalkObjectBlocksEntry is a value passed to the callback for WalkObjectBlocks.
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
	Blk any
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
	// This is the data pre-transformation (at rest).
	Data []byte
	// XfrmData is the transformed data at this entry if any.
	// Empty if there is no read transformer or if Blk is nil (not decoded).
	XfrmData []byte
}

// BlockRefs returns the outgoing refs of the entry's block. A decoded block
// yields the refs it holds. A block walked without a constructor, such as an
// object root reached through an ObjectRef, yields the refs store holds for
// it: its children exist even when the walk cannot decode them.
func (e *WalkObjectBlocksEntry) BlockRefs(ctx context.Context, store block.StoreOps) ([]*block.BlockRef, error) {
	// Prefer the refs of the decoded block.
	if e.Blk != nil {
		return block.ExtractBlockRefs(e.Blk)
	}

	// Fall back to the refs stored with the undecoded block.
	stored, err := store.GetStoredBlock(ctx, e.Ref)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, errors.Wrap(block.ErrNotFound, e.Ref.MarshalString())
	}
	if !stored.RefsKnown {
		return nil, errors.Wrap(block.ErrRefsUnknown, e.Ref.MarshalString())
	}
	return stored.Refs, nil
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
// If alwaysDecode is false, skips decoding blocks if they have no refs within.
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
	readBkt bucket.BucketOps,
	readXfrm block.Transformer,
	maxConcurrency int,
	alwaysDecode bool,
) error {
	// Skip the object walk when no root entry was supplied.
	if root == nil {
		return nil
	}

	// Create a cancelable object walk with a shared worker error channel.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 1)
	handleErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	// Start the root traversal and wait for idle or a worker failure.
	queue := root.BuildConcurrentQueue(
		ctx,
		handleErr,
		cb,
		readBkt, readXfrm,
		maxConcurrency,
		alwaysDecode,
	)
	err := queue.WaitIdle(ctx, errCh)
	if err == nil {
		err = ctx.Err()
	}
	cancel()

	// Join callbacks before releasing the caller's stores or callback state.
	// Idle and the last worker's error can arrive together; inspect that
	// buffered error after joining even when WaitIdle observed idle first.
	_ = queue.WaitIdle(context.WithoutCancel(ctx), nil)
	if err != nil {
		return err
	}
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

// walkState contains state for walking a tree of blocks.
type walkState struct {
	readBkt      bucket.BucketOps
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
	readBkt bucket.BucketOps,
	readXfrm block.Transformer,
	maxConcurrency int,
	alwaysDecode bool,
) *conc.ConcurrentQueue {
	// Construct the concurrent traversal queue and its shared walk state.
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

	// Schedule the root block visit on the traversal queue.
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
		// Stop the block visit when the object walk is canceled.
		if ctx.Err() != nil {
			return
		}

		// Trace the block visit for the lifetime of its worker request.
		workerCtx := ctx
		var workerTask *trace.Task
		if copyWorkerTraceEnabled(ctx) {
			workerCtx, workerTask = trace.NewTask(ctx, "db/bucket/lookup/copy-worker-request")
			trace.Log(workerCtx, "copy-worker-phase", "request")
			defer workerTask.End()
		}

		// Fetch the referenced block when the entry has no payload yet.
		if !e.Found && e.Err == nil && !e.IsSubBlock && !e.Ref.GetEmpty() {
			// returns nil, false, nil if reference was empty.
			e.Data, e.Found, e.Err = st.readBkt.GetBlock(workerCtx, e.Ref)
		}

		// Decode the fetched block for child traversal.
		if e.Found && e.Ctor != nil && e.Err == nil && !e.IsSubBlock {
			err := e.decodeBlock(st.alwaysDecode, st.readXfrm)
			if err != nil && e.Err == nil {
				e.Err = err
			}
		}

		// Ask the block callback whether traversal should continue.
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

		// Order child entries by reference ID before scheduling their visits.
		slices.SortStableFunc(toEnqueue, func(a, b *WalkObjectBlocksEntry) int {
			if a.RefID < b.RefID {
				return -1
			}
			if a.RefID > b.RefID {
				return 1
			}
			return 0
		})

		// Schedule each child entry on the shared traversal queue.
		enqFns := make([]func(), len(toEnqueue))
		for i, enq := range toEnqueue {
			enqFns[i] = enq.buildVisitFn(ctx, st)
		}
		st.enqueue(enqFns...)
	}
}

// decodeBlock conditionally decodes the block.
func (e *WalkObjectBlocksEntry) decodeBlock(alwaysDecode bool, readXfrm block.Transformer) error {
	// Construct a decoder only for an available block with a constructor.
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

	// Decode the stored block bytes with the source transformer.
	dat, err := e.Data, error(nil)
	if readXfrm != nil {
		dat, err = readXfrm.DecodeBlock(e.Data)
	}
	if err != nil {
		if err != context.Canceled {
			err = errors.Wrapf(err, "decode block: %s", e.Ref.MarshalString())
		}
		return err
	}
	e.XfrmData = dat

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
