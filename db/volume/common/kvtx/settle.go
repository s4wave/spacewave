package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// hideUnmarked clears exists for each of refs the settle of the Volume's old
// generation unmarked, so a writer that checks before it puts puts the block
// again, which gives birth to it again, as block_gc.UnmarkedError describes.
// Once the Volume is Settled it reads nothing; before, one snapshot reads
// the state and, while Sweeping, the run entry of each block found.
func (v *Volume) hideUnmarked(ctx context.Context, refs []*block.BlockRef, exists []bool) error {
	// A Volume without liveness, or a Settled one, has none.
	if !v.atomic || v.settled.Load() {
		return nil
	}

	// Read the generation, then the run entry of each block found.
	return v.readRecords(ctx, func(rec *block_gc.Records) error {
		if _, err := rec.GetState(ctx); err != nil || rec.Settled() {
			return err
		}
		for i, ref := range refs {
			if !exists[i] {
				continue
			}
			unmarked, err := rec.Unmarked(ctx, block_gc.BlockKey(ref))
			if err != nil {
				return err
			}
			exists[i] = !unmarked
		}
		return nil
	})
}
