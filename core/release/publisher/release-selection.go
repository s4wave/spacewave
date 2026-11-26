package publisher

import (
	"context"

	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/delta"
	"github.com/s4wave/spacewave/db/packfile"
)

// releaseSelection borrows a walk-ordered subset of verified closure metadata.
// It can emit that subset, but cannot supply a complete reachability proof.
type releaseSelection struct {
	// blocks borrows selected entries from the immutable verified closure.
	blocks []*packedBlock
}

// count reports the number of selected blocks for publication progress.
func (s *releaseSelection) count() int {
	return len(s.blocks)
}

// emit reads the selection in walk order into content-addressed packs.
func (s *releaseSelection) emit(
	ctx context.Context,
	read loadBlock,
	spaceID string,
	emit delta.ChunkEmitter,
) ([]*packfile.PackfileEntry, uint64, error) {
	index := 0
	return emitBlocks(ctx, read, spaceID, func() *packedBlock {
		if index == len(s.blocks) {
			return nil
		}
		entry := s.blocks[index]
		index++
		return entry
	}, emit)
}
