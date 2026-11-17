package packsweep

import (
	"bytes"
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Verify defaults.
const (
	// DefaultVerifyBatch is the frontier keys one verify transaction takes.
	DefaultVerifyBatch = 512
	// DefaultVerifyReads is the blocks a verify reads at once.
	DefaultVerifyReads = 16
	// DefaultVerifyMaxBad is the bad blocks a verify result lists.
	DefaultVerifyMaxBad = 20
)

// verifySet is the set of blocks a verify has read, with its frontier. It
// lives in the verify's own scratch store, apart from any cycle.
var verifySet = walkSet{
	visited:  verifyVisitedPrefix,
	frontier: verifyFrontierPrefix,
}

// VerifyOptions configures Verify.
type VerifyOptions struct {
	// Store is an empty scratch store holding the blocks read and the
	// frontier, so memory stays bounded however many blocks the roots reach.
	Store kvtx.Store
	// Source serves the blocks, such as a pack store that reads the bucket
	// alone.
	Source packcopy.Source
	// Roots are the named roots, walked in order. A block an earlier root
	// reached counts for that root alone.
	Roots []block.NamedRoot
	// Batch is the frontier keys per transaction; zero uses
	// DefaultVerifyBatch.
	Batch int
	// Reads is the blocks read at once; zero uses DefaultVerifyReads.
	Reads int
	// MaxBad is the bad blocks the result lists; zero uses
	// DefaultVerifyMaxBad. Every bad block is counted.
	MaxBad int
	// Progress observes the verify after each batch, nil for none.
	Progress func(*VerifyResult)
}

// Verify reads every block the roots reach from the source, checking each
// against its ref's hash, and counts the blocks and bytes each root adds and
// the blocks that are missing, unreadable, corrupt or without their refs. It
// keeps the set of blocks read and the frontier in the scratch store and only
// counts and the first few bad blocks in memory. An error is a failed scratch
// store or a stop; a bad block is a result.
func Verify(ctx context.Context, opts *VerifyOptions) (*VerifyResult, error) {
	// Apply the defaults.
	batch := opts.Batch
	if batch <= 0 {
		batch = DefaultVerifyBatch
	}
	reads := opts.Reads
	if reads <= 0 {
		reads = DefaultVerifyReads
	}
	maxBad := opts.MaxBad
	if maxBad <= 0 {
		maxBad = DefaultVerifyMaxBad
	}

	// Walk each root in turn, sharing the set of blocks read.
	res := &VerifyResult{BadReasons: make(map[string]uint64)}
	v := &verifier{store: opts.Store, src: opts.Source, reads: reads, maxBad: maxBad, res: res}
	for _, root := range opts.Roots {
		if root.Ref.GetEmpty() {
			continue
		}
		rr := &VerifyRootResult{Name: root.Name, Ref: root.Ref}
		res.Roots = append(res.Roots, rr)
		if err := v.seed(ctx, root.Ref); err != nil {
			return nil, err
		}
		for {
			done, err := v.batch(ctx, rr, batch)
			if err != nil {
				return nil, err
			}
			if done {
				break
			}
			if opts.Progress != nil {
				opts.Progress(res)
			}
		}
	}
	return res, nil
}

// verifier walks the verify set a batch of its frontier at a time.
type verifier struct {
	// store is the scratch store.
	store kvtx.Store
	// src serves the blocks.
	src packcopy.Source
	// reads is the blocks read at once.
	reads int
	// maxBad is the bad blocks the result lists.
	maxBad int
	// res is the result so far.
	res *VerifyResult
}

// verifyRead is one frontier block and what reading it found.
type verifyRead struct {
	// tail is its frontier key's tail.
	tail []byte
	// ref is the block.
	ref *block.BlockRef
	// stored is the block when reason is empty.
	stored *block.StoredBlock
	// reason is why the block could not be used, empty when it could.
	reason string
}

// seed adds the root to the set and the frontier unless an earlier root
// reached it.
func (v *verifier) seed(ctx context.Context, ref *block.BlockRef) error {
	// Visit the root in one transaction.
	tx, err := v.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if _, err := visit(ctx, tx, verifySet, ref); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// batch reads up to n frontier blocks, then replaces them with their children
// in one transaction, counting them for rr. It reports done when the
// frontier was empty.
func (v *verifier) batch(ctx context.Context, rr *VerifyRootResult, n int) (bool, error) {
	// Take the batch and read its blocks.
	keys, err := v.readFrontier(ctx, n)
	if err != nil || len(keys) == 0 {
		return len(keys) == 0, err
	}
	if err := v.readBlocks(ctx, keys); err != nil {
		return false, err
	}

	// Replace each block with its children, or count it bad.
	tx, err := v.store.NewTransaction(ctx, true)
	if err != nil {
		return false, err
	}
	defer tx.Discard()
	for _, k := range keys {
		if err := v.take(ctx, tx, rr, k); err != nil {
			return false, err
		}
	}
	return false, tx.Commit(ctx)
}

// take replaces a read block on the frontier with its children, counting it
// for rr, or counts it bad.
func (v *verifier) take(ctx context.Context, tx kvtx.TxOps, rr *VerifyRootResult, k *verifyRead) error {
	// Take it off the frontier.
	if err := tx.Delete(ctx, prefixed(verifySet.frontier, k.tail)); err != nil {
		return err
	}

	// Count a block that is unusable, or lacks the refs to walk under it.
	if k.reason == "" && !k.stored.RefsKnown {
		k.reason = packcopy.ReasonRefsUnknown
	}
	if k.reason != "" {
		v.bad(rr, k)
		return nil
	}

	// Count it, and add its children.
	size := uint64(len(k.stored.Data))
	rr.Blocks, rr.Bytes = rr.Blocks+1, rr.Bytes+size
	v.res.Blocks, v.res.Bytes = v.res.Blocks+1, v.res.Bytes+size
	for _, child := range k.stored.Refs {
		if child.GetEmpty() {
			continue
		}
		if _, err := visit(ctx, tx, verifySet, child); err != nil {
			return err
		}
	}
	return nil
}

// bad counts a bad block for rr, and lists it while the list has room.
func (v *verifier) bad(rr *VerifyRootResult, k *verifyRead) {
	rr.Bad++
	v.res.Bad++
	v.res.BadReasons[k.reason]++
	if len(v.res.BadBlocks) < v.maxBad {
		v.res.BadBlocks = append(v.res.BadBlocks, VerifyBad{Root: rr.Name, Ref: k.ref, Reason: k.reason})
	}
}

// readFrontier reads up to n frontier keys.
func (v *verifier) readFrontier(ctx context.Context, n int) ([]*verifyRead, error) {
	// Open a read.
	tx, err := v.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Take the first n keys.
	var keys []*verifyRead
	errFull := errors.New("full")
	prefix := []byte(verifySet.frontier)
	err = tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		// Decode the key's ref, stopping at the n-th.
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(value); err != nil {
			return errors.Wrap(err, "frontier row")
		}
		keys = append(keys, &verifyRead{tail: bytes.Clone(key[len(prefix):]), ref: ref})
		if len(keys) == n {
			return errFull
		}
		return nil
	})
	if err != nil && err != errFull {
		return nil, err
	}
	return keys, nil
}

// readBlocks reads the keys' blocks, v.reads at a time, checking each
// against its hash.
func (v *verifier) readBlocks(ctx context.Context, keys []*verifyRead) error {
	// Start the readers, keeping the first error.
	next := make(chan *verifyRead)
	var wg sync.WaitGroup
	var errOnce sync.Once
	var firstErr error
	for range min(v.reads, len(keys)) {
		wg.Go(func() {
			for k := range next {
				stored, reason, err := packcopy.ReadVerified(ctx, v.src, k.ref)
				if err != nil {
					errOnce.Do(func() { firstErr = err })
					continue
				}
				k.stored, k.reason = stored, reason
			}
		})
	}

	// Hand them the keys, and wait for the reads.
	for _, k := range keys {
		next <- k
	}
	close(next)
	wg.Wait()
	return firstErr
}
