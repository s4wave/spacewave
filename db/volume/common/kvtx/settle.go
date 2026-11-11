package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// hideAbsent clears exists for each of refs that reads absent until a put
// stores it again, as block_gc.Records.Hidden describes, so a writer that
// checks before it puts puts the block again, which gives birth to it again.
// Once the Volume is Settled it reads nothing; before, one snapshot reads the
// state and, per block found, what Hidden reads.
func (v *Volume) hideAbsent(ctx context.Context, refs []*block.BlockRef, exists []bool) error {
	// A Volume without liveness, or a Settled one, has none.
	if !v.atomic || v.settled.Load() {
		return nil
	}

	// Read the generation, then whether each block found is hidden.
	return v.readRecords(ctx, func(rec *block_gc.Records) error {
		if _, err := rec.GetState(ctx); err != nil || rec.Settled() {
			return err
		}
		for i, ref := range refs {
			if !exists[i] {
				continue
			}
			hidden, err := rec.Hidden(ctx, ref)
			if err != nil {
				return err
			}
			exists[i] = !hidden
		}
		return nil
	})
}
