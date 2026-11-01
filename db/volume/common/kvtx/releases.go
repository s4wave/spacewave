package kvtx

import (
	"context"
	"errors"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// releaseCandidates bounds the freed blocks a release check keeps by name, to
// log those the mark reached.
const releaseCandidates = 256

// releaseCheck holds the blocks the release resolver freed before a block
// pass's mark, for checkReleases.
type releaseCheck struct {
	// freed holds every block the resolver freed.
	freed *block_gc.NodeSet
	// candidates are the first freed blocks by name.
	candidates []string
}

// resolveReleases resolves the shared counts' pending releases in budgeted
// transactions, as RefGraph.ResolveReleases describes, each yielding the
// writer after its budget or to a waiting writer. It runs before a block
// pass's mark, which checkReleases then compares with the blocks it freed. It
// resolves nothing while the sharing walk builds the counts, or on a Volume
// that keeps none, and returns a nil check then or on an error.
//
// The caller runs it apart from checkDueDeadlists: a release must not take a
// count between a record's read of the counts and its write of their changes.
func (v *Volume) resolveReleases(ctx context.Context) (*releaseCheck, error) {
	// A Volume that keeps no counts has no releases.
	if !v.holdCounts {
		return nil, nil
	}

	// Resolve slices until no release is left, noting each freed block.
	check := &releaseCheck{freed: block_gc.NewNodeSet()}
	visit := func(node string) error {
		if check.freed.Add(node) && len(check.candidates) < releaseCandidates {
			check.candidates = append(check.candidates, node)
		}
		return nil
	}
	for {
		var res block_gc.ReleaseResult
		err := v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) (err error) {
			held := time.Now()
			yield := func() bool { return time.Since(held) >= sweepBudget || v.writersWaiting() }
			res, err = rg.ResolveReleases(ctx, yield, visit)
			return err
		})
		if errors.Is(err, block_gc.ErrSharingWalk) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		v.releasesResolved.Add(res.Resolved)
		v.releaseFreedBlocks.Add(res.Freed)
		v.releaseUnderflows.Add(res.Underflows)
		if res.Done {
			return check, nil
		}
	}
}

// checkReleases counts the blocks check freed that mark reached, which are
// errors: an uncounted holder, or a write naming the block after its free,
// and logs a sample of them. A nil check counts nothing.
func (v *Volume) checkReleases(check *releaseCheck, mark *block_gc.BlockMark) {
	// Count the freed blocks the mark reached.
	if check == nil || check.freed.Len() == 0 {
		return
	}
	reached := check.freed.Overlap(mark.Live)
	v.releaseLiveBlocks.Add(uint64(reached)) // #nosec G115 -- a count is not negative.
	if reached == 0 || v.le == nil {
		return
	}

	// Log the named ones.
	var sample []string
	for _, node := range check.candidates {
		if mark.Live.Has(node) && len(sample) < deadlistSampleSize {
			sample = append(sample, node)
		}
	}
	v.le.WithField("live-blocks", reached).
		WithField("freed-blocks", check.freed.Len()).
		WithField("sample", sample).
		Error("released blocks the mark reached")
}
