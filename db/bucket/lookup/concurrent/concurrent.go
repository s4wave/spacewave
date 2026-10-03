package lookup_concurrent

import (
	"context"
	"runtime"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/conc"
	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/bucket"
	lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/dex"
	"github.com/sirupsen/logrus"
)

// ControllerID is the id for the concurrent lookup controller.
const ControllerID = "hydra/lookup/concurrent"

// Version is the version of the concurrent implementation.
var Version = controller.MustParseVersion("0.0.1")

// LookupController implements the concurrent lookup controller.
type LookupController struct {
	// le is the logger
	le *logrus.Entry
	// b is the bus
	b bus.Bus
	// conf is the config
	conf *Config

	// bucketHandleSetCtr contains the bucket handle set
	bucketHandleSetCtr *ccontainer.CContainer[*[]bucket.BucketHandle]
	// fallbackBlockStoreRc is a refcount for the fallback block store.
	fallbackBlockStoreRc *refcount.RefCount[block_store.Store]
}

// NewLookupController is the lookup controller constructor.
func NewLookupController(
	le *logrus.Entry,
	b bus.Bus,
	conf *Config,
) lookup.Controller {
	lc := &LookupController{
		le:                 le.WithField("bucket-id", conf.GetBucketConf().GetId()),
		b:                  b,
		conf:               conf,
		bucketHandleSetCtr: ccontainer.NewCContainer[*[]bucket.BucketHandle](nil),
	}
	if fallbackBlockStoreID := lc.conf.GetFallbackBlockStoreId(); fallbackBlockStoreID != "" {
		lc.fallbackBlockStoreRc = refcount.NewRefCount(
			nil,
			true,
			nil,
			nil,
			block_store.NewAccessBlockStoreViaBusFunc(b, fallbackBlockStoreID, false),
		)
	}
	return lc
}

// Execute executes the reconciler controller.
func (c *LookupController) Execute(ctx context.Context) error {
	if c.fallbackBlockStoreRc != nil {
		c.fallbackBlockStoreRc.SetContext(ctx)
	}
	return nil
}

// LookupBlock searches for a block using the bucket lookup controller.
// If lookup is disabled, will return an error.
func (c *LookupController) LookupBlock(
	rctx context.Context,
	ref *block.BlockRef,
	optf ...lookup.LookupBlockOption,
) ([]byte, bool, error) {
	res, err := c.lookupBlock(rctx, ref, false, optf...)
	if err != nil || res == nil {
		return nil, false, err
	}
	return res.Data, true, nil
}

// LookupStoredBlock searches for a block and its outgoing refs using the
// bucket lookup controller. RefsKnown is unset when the source that held the
// block could not supply its refs. Returns nil if not found.
func (c *LookupController) LookupStoredBlock(
	rctx context.Context,
	ref *block.BlockRef,
	optf ...lookup.LookupBlockOption,
) (*block.StoredBlock, error) {
	return c.lookupBlock(rctx, ref, true, optf...)
}

// lookupBlock reads a block from the bucket handles, then the fallback store
// and the network. Handle reads include refs when withRefs is set. Misses
// always ask for refs, so the write-back can record the block's edges.
// Returns nil when the block is not found.
func (c *LookupController) lookupBlock(
	rctx context.Context,
	ref *block.BlockRef,
	withRefs bool,
	optf ...lookup.LookupBlockOption,
) (retRes *block.StoredBlock, retErr error) {
	// Validate the block reference before searching the bucket handles.
	opts := lookup.NewLookupBlockOpts(optf...)
	if ref.GetEmpty() {
		return nil, block.ErrEmptyBlockRef
	}

	// apply lookup timeout
	timeoutDur := opts.Timeout
	if timeoutDur == 0 {
		timeoutDur, _ = c.conf.ParseLookupTimeoutDur()
	}
	var reqCtx context.Context
	var reqCtxCancel context.CancelFunc
	if timeoutDur > 0 {
		reqCtx, reqCtxCancel = context.WithTimeout(rctx, timeoutDur)
	} else {
		reqCtx, reqCtxCancel = context.WithCancel(rctx)
	}
	defer reqCtxCancel()

	// if timeout not found is set, transform DeadlineExceeded to not found.
	if opts.TimeoutNotFound {
		defer func() {
			if retErr == context.DeadlineExceeded {
				retRes, retErr = nil, nil
			}
		}()
	}

	// acquire handles
	bh, err := c.getBucketHandles(reqCtx)
	if err != nil {
		return nil, err
	}
	if len(bh) == 0 {
		return nil, errors.Wrap(bucket.ErrBucketNotFound, c.conf.GetBucketConf().GetId())
	}

	// Attach the block reference to lookup diagnostics.
	le := func() *logrus.Entry {
		return c.le.WithField("ref", ref.MarshalString())
	}

	// fast path: one bucket handle
	if len(bh) == 1 {
		res, err := block.ReadStoredBlock(reqCtx, bh[0].GetBucket(), ref, withRefs)
		if err != nil {
			if err != context.Canceled {
				le().WithError(err).Warn("unable to lookup ref")
			}
			return nil, err
		}
		if res == nil {
			return c.lookupMissing(reqCtx, bh, ref, opts)
		}
		return res, nil
	}

	// perform concurrent lookup
	var bcast broadcast.Broadcast
	var rres *block.StoredBlock
	var rerr error
	var waitCh <-chan struct{}
	bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		// Queue a block read for every existing bucket handle.
		var running int
		queue := make([]func(), 0, len(bh))
		for _, h := range bh {
			// Exclude handles whose buckets no longer exist.
			if !h.GetExists() {
				continue
			}

			// Count the bucket read and queue its completion callback.
			running++
			queue = append(queue, func() {
				// Read the bucket block before publishing its result.
				res, err := block.ReadStoredBlock(reqCtx, h.GetBucket(), ref, withRefs)

				// Publish the bucket result and notify waiting readers under the lock.
				bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
					// Retain the first block and prioritize substantive lookup errors.
					if err != nil {
						// prioritize non context canceled errors
						if rerr == nil || err != context.Canceled {
							rerr = err
						}
					} else if res != nil && rres == nil {
						rres = res
					}

					// Wake the lookup when a result arrives or all bucket reads finish.
					running--
					if running == 0 || rerr != nil || rres != nil {
						broadcast()
					}
				})
			})
		}

		// Start the queued bucket reads concurrently.
		for _, fn := range queue {
			go fn()
		}

		// Subscribe to bucket read completion before releasing the lock.
		waitCh = getWaitCh()
	})

	// wait for running == 0
	select {
	case <-reqCtx.Done():
		return nil, context.Canceled
	case <-waitCh:
	}

	// Return the bucket result or search the remaining block sources.
	if rerr != nil {
		return nil, rerr
	}
	if rres == nil {
		return c.lookupMissing(reqCtx, bh, ref, opts)
	}
	return rres, nil
}

// lookupMissing reads a block that no bucket handle holds from the fallback
// store, then the network, and writes a hit back to the bucket handles.
func (c *LookupController) lookupMissing(
	reqCtx context.Context,
	bh []bucket.BucketHandle,
	ref *block.BlockRef,
	opts *lookup.LookupBlockOpts,
) (*block.StoredBlock, error) {
	// Report the bucket miss when verbose diagnostics are enabled.
	if c.conf.GetVerbose() {
		c.le.WithField("ref", ref.MarshalString()).
			Debugf("ref not found against %d handles", len(bh))
	}

	// Search the fallback block store after the bucket handles miss.
	var res *block.StoredBlock
	if c.fallbackBlockStoreRc != nil {
		// Acquire the fallback block store for the missing reference.
		fallbackBlockStore, fallbackBlockStoreRef, err := c.fallbackBlockStoreRc.Wait(reqCtx)
		if err != nil {
			return nil, err
		}

		// Read the fallback block and release the store reference.
		res, err = fallbackBlockStore.GetStoredBlock(reqCtx, ref)
		fallbackBlockStoreRef.Release()
		if err != nil {
			return nil, err
		}
	}

	// Search the network when local sources miss and network lookup is allowed.
	if res == nil && !opts.LocalOnly {
		notFoundBehavior := c.conf.GetNotFoundBehavior()
		wait := notFoundBehavior == NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE_WAIT
		if wait || notFoundBehavior == NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE {
			var err error
			res, err = c.lookupWithDirective(reqCtx, ref, wait)
			if err != nil {
				return nil, err
			}
		}
	}

	// Write the recovered block and its edges back to the bucket handles.
	if res != nil {
		if err := c.writeback(reqCtx, bh, ref, res); err != nil {
			c.le.WithField("ref", ref.MarshalString()).
				WithError(err).
				Warn("unable to write-back block")
		}
	}
	return res, nil
}

// writeback writes a block found outside the bucket handles back to them
// with its refs. A block without known refs is not written back, so a bucket
// never holds a block whose edges were lost.
func (c *LookupController) writeback(
	reqCtx context.Context,
	bh []bucket.BucketHandle,
	ref *block.BlockRef,
	res *block.StoredBlock,
) error {
	// Require enabled writeback and known edges before storing the block.
	if c.conf.GetWritebackBehavior() != WritebackBehavior_WritebackBehavior_ALL || !res.RefsKnown {
		return nil
	}

	// Preserve the block reference and edges in every bucket write.
	putOpts := &block.PutOpts{
		HashType:      ref.GetHash().GetHashType(),
		ForceBlockRef: ref,
		Refs:          res.Refs,
	}

	// Queue writes to existing bucket handles and collect their outcomes.
	doFns := make([]func(), 0, len(bh))
	var lastErr atomic.Pointer[error]
	var nw atomic.Uint32
	for _, h := range bh {
		// Exclude bucket handles that cannot receive the block.
		if !h.GetExists() || h.GetBucket() == nil {
			continue
		}

		// Record whether this bucket write failed or added the block.
		doFns = append(doFns, func() {
			_, existed, werr := h.GetBucket().PutBlock(reqCtx, res.Data, putOpts)
			if werr != nil {
				lastErr.Store(&werr)
			} else if !existed {
				nw.Add(1)
			}
		})
	}

	// Finish without starting workers when no bucket can receive the block.
	if len(doFns) == 0 {
		return nil
	}

	// Complete the queued bucket writes before reporting their errors.
	q := conc.NewConcurrentQueue(runtime.NumCPU(), doFns...)
	if err := q.WaitIdle(reqCtx, nil); err != nil {
		return err
	}

	// Propagate a bucket write failure after all workers finish.
	if errp := lastErr.Load(); errp != nil {
		return *errp
	}

	// Report how many bucket handles gained the block.
	if c.conf.GetVerbose() {
		if written := nw.Load(); written != 0 {
			c.le.WithField("ref", ref.MarshalString()).
				Debugf("wrote-back block to %d handles", written)
		}
	}
	return nil
}

// LookupBlockExistsBatch checks whether each block exists using local block
// stores when LocalOnly is set.
func (c *LookupController) LookupBlockExistsBatch(
	rctx context.Context,
	refs []*block.BlockRef,
	optf ...lookup.LookupBlockOption,
) (retFound []bool, retErr error) {
	// Validate the requested references and allocate their existence results.
	opts := lookup.NewLookupBlockOpts(optf...)
	retFound = make([]bool, len(refs))
	for _, ref := range refs {
		if ref.GetEmpty() {
			return nil, block.ErrEmptyBlockRef
		}
	}
	if len(refs) == 0 {
		return retFound, nil
	}

	// Use individual lookups when the batch may search network sources.
	if !opts.LocalOnly {
		for i, ref := range refs {
			// Resolve the block and preserve its existence at the requested position.
			_, found, err := c.LookupBlock(rctx, ref, optf...)
			if err != nil {
				return nil, err
			}
			retFound[i] = found
		}
		return retFound, nil
	}

	// Bound local batch reads by the configured lookup deadline.
	var reqCtx context.Context
	var reqCtxCancel context.CancelFunc
	timeoutDur := opts.Timeout
	if timeoutDur == 0 {
		timeoutDur, _ = c.conf.ParseLookupTimeoutDur()
	}
	if timeoutDur > 0 {
		reqCtx, reqCtxCancel = context.WithTimeout(rctx, timeoutDur)
	} else {
		reqCtx, reqCtxCancel = context.WithCancel(rctx)
	}
	defer reqCtxCancel()

	// Convert an expired batch lookup into missing-reference results if requested.
	if opts.TimeoutNotFound {
		defer func() {
			if retErr == context.DeadlineExceeded {
				retErr = nil
				retFound = make([]bool, len(refs))
			}
		}()
	}

	// Acquire the bucket handles that serve the requested references.
	bh, err := c.getBucketHandles(reqCtx)
	if err != nil {
		return nil, err
	}
	if len(bh) == 0 {
		return nil, errors.Wrap(bucket.ErrBucketNotFound, c.conf.GetBucketConf().GetId())
	}

	// Merge existence results from every available bucket.
	for _, h := range bh {
		// Exclude bucket handles that cannot serve local reads.
		if !h.GetExists() || h.GetBucket() == nil {
			continue
		}

		// Read and validate the bucket batch before merging its results.
		found, err := h.GetBucket().GetBlockExistsBatch(reqCtx, refs)
		if err != nil {
			return nil, err
		}
		if len(found) != len(refs) {
			return nil, errors.Errorf("bucket exists batch returned %d results for %d refs", len(found), len(refs))
		}

		// Preserve a hit from any bucket for each requested reference.
		for i, ok := range found {
			retFound[i] = retFound[i] || ok
		}
	}

	// Check the fallback store for references absent from every bucket.
	if c.fallbackBlockStoreRc != nil {
		// Collect missing references with their positions in the requested batch.
		var missingRefs []*block.BlockRef
		var missingIdx []int
		for i, found := range retFound {
			if !found {
				missingRefs = append(missingRefs, refs[i])
				missingIdx = append(missingIdx, i)
			}
		}

		// Read missing-reference existence from the fallback store.
		if len(missingRefs) != 0 {
			// Acquire the fallback store for the missing-reference batch.
			fallbackBlockStore, fallbackBlockStoreRef, err := c.fallbackBlockStoreRc.Wait(reqCtx)
			if err != nil {
				return nil, err
			}

			// Read and validate the fallback batch before mapping its results.
			found, err := fallbackBlockStore.GetBlockExistsBatch(reqCtx, missingRefs)
			fallbackBlockStoreRef.Release()
			if err != nil {
				return nil, err
			}
			if len(found) != len(missingRefs) {
				return nil, errors.Errorf("fallback exists batch returned %d results for %d refs", len(found), len(missingRefs))
			}

			// Map fallback hits back to the original reference positions.
			for i, ok := range found {
				retFound[missingIdx[i]] = ok
			}
		}
	}

	return retFound, nil
}

// PutBlock writes a block using the bucket lookup controller.
// The behavior of the write-back is configured in the lookup controller.
// If lookup is disabled, will return an error.
func (c *LookupController) PutBlock(
	reqCtx context.Context,
	data []byte, opts *block.PutOpts,
) ([]*bucket.ObjectRef, bool, error) {
	switch c.conf.GetPutBlockBehavior() {
	case PutBlockBehavior_PutBlockBehavior_ALL:
		return c.putBlockAllVolumes(reqCtx, data, opts)
	default:
		return nil, false, block_store.ErrReadOnly
	}
}

// putBlockAllVolumes implements PutBlockBehavior_PutBlockBehavior_ALL
func (c *LookupController) putBlockAllVolumes(
	rctx context.Context,
	data []byte,
	opts *block.PutOpts,
) ([]*bucket.ObjectRef, bool, error) {
	// Keep bucket writes within a context canceled when this request returns.
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// Acquire the bucket handles and a channel for their write results.
	bucketHandles, err := c.getBucketHandles(ctx)
	if err != nil {
		return nil, false, err
	}
	type res struct {
		err error
		ex  bool
		e   *block.BlockRef
		b   string
	}
	resCh := make(chan *res)

	// Write the block only to handles whose buckets still exist.
	putBlockFn := func(h bucket.BucketHandle) (bres *block.BlockRef, existed bool, berr error) {
		if !h.GetExists() {
			return nil, false, nil
		}
		return h.GetBucket().PutBlock(ctx, data, opts)
	}

	// Start one block write for every existing bucket handle.
	var br int
	for _, h := range bucketHandles {
		// Exclude bucket handles that no longer exist.
		if !h.GetExists() {
			continue
		}

		// Count this bucket write and deliver its result unless canceled.
		br++
		go func(h bucket.BucketHandle) {
			// Write the bucket block before forwarding its result to the request.
			bres, existed, berr := putBlockFn(h)
			select {
			case <-ctx.Done():
				return
			case resCh <- &res{
				err: berr,
				e:   bres,
				ex:  existed,
				b:   h.GetID(),
			}:
			}
		}(h)
	}

	// Gather bucket references and retain the first write failure.
	var rerr error
	refs := make([]*bucket.ObjectRef, 0, br)
	allExisted := true
	for i := 0; i < br; i++ {
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case res := <-resCh:
			if res.err != nil {
				if rerr == nil {
					rerr = res.err
				}
			} else if res.e != nil && !res.e.GetEmpty() {
				refs = append(refs, &bucket.ObjectRef{
					RootRef:  res.e,
					BucketId: res.b,
				})
				if !res.ex {
					allExisted = false
				}
			}
		}
	}
	return refs, allExisted, rerr
}

// lookupWithDirective uses the dex directive to lookup a block.
func (c *LookupController) lookupWithDirective(reqCtx context.Context, ref *block.BlockRef, wait bool) (*block.StoredBlock, error) {
	// Build a network lookup directive for the configured bucket.
	bucketID := c.conf.GetBucketConf().GetId()
	dir := dex.NewLookupBlockFromNetwork(bucketID, ref)

	// End idle network demand once a missing block has been reported.
	var notFoundSeen atomic.Bool
	var idle atomic.Bool
	var idleCb bus.ExecIdleCallback = func(isIdle bool, errs []error) (cwait bool, err error) {
		// Keep network demand active while resolvers still have work.
		if !isIdle {
			return true, nil
		}

		// Combine resolver idleness with the requested wait and observed misses.
		idle.Store(true)
		cwait, err = bus.ReturnIfIdle(!wait)(isIdle, errs)
		if cwait && err == nil && notFoundSeen.Load() {
			// don't wait if we saw not-found or an error
			cwait = false
		}
		return cwait, err
	}

	// Wait for a network block or a terminal lookup result.
	lval, _, aref, err := bus.ExecWaitValue(
		reqCtx,
		c.b,
		dir,
		idleCb,
		nil,
		func(val dex.LookupBlockFromNetworkValue) (bool, error) {
			// if IgnoreNotFound is set in the lookup controller: prevents
			// resolving the directive with a not-found result.
			if err := val.GetError(); err != nil && err != block.ErrNotFound {
				return true, err
			}

			// Record network misses so idle demand can complete without a block.
			if len(val.GetData()) == 0 {
				notFoundSeen.Store(true)

				// if we already saw idle=true and not-found is returned,
				// return the not-found result immediately.
				return idle.Load(), nil
			}
			return true, nil
		},
	)
	if aref != nil {
		aref.Release()
	}

	// Reject network lookup failures and missing block data.
	if err != nil || lval == nil {
		return nil, err
	}
	if err := lval.GetError(); err != nil {
		return nil, err
	}
	if len(lval.GetData()) == 0 {
		return nil, nil
	}
	return &block.StoredBlock{
		Data:      lval.GetData(),
		Refs:      lval.GetRefs(),
		RefsKnown: lval.GetRefsKnown(),
	}, nil
}

// getBucketHandles waits for the bucket handle set.
func (c *LookupController) getBucketHandles(ctx context.Context) ([]bucket.BucketHandle, error) {
	valptr, err := c.bucketHandleSetCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return *valptr, nil
}

// PushBucketHandles pushes the bucket handle list that the controller may use
// to service requests. The controller should wait for this to be called before
// beginning to service requests. The bucket handles pushed should always have
// GetExists() == true.
func (c *LookupController) PushBucketHandles(ctx context.Context, handles []bucket.BucketHandle) {
	c.bucketHandleSetCtr.SetValue(&handles)
}

// GetControllerInfo returns controller information.
func (c *LookupController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"bucket lookup "+c.conf.GetBucketConf().GetId(),
	)
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
// The context passed is canceled when the directive instance expires.
func (c *LookupController) HandleDirective(
	ctx context.Context,
	i directive.Instance,
) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
func (c *LookupController) Close() error {
	return nil
}

// _ is a type assertion
var _ lookup.Controller = (*LookupController)(nil)
