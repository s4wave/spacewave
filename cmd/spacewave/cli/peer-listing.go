//go:build !js

package spacewave_cli

import (
	"bufio"
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
)

// peerListing streams each verified peer ref once, in the order first served.
// The repair walk reads a batch of blocks on several goroutines, so mtx
// guards the listing.
type peerListing struct {
	// StoreOps supplies verified peer blocks and the read-only store operations.
	block.StoreOps
	// mtx guards writer and seen.
	mtx sync.Mutex
	// writer buffers refs until the repair publishes its listing.
	writer *bufio.Writer
	// seen holds only the refs this peer has served successfully.
	seen map[string]struct{}
}

// GetStoredBlock lists a verified peer hit before handing it to the walk.
func (p *peerListing) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Leave misses and failed reads out of the listing.
	stored, err := p.StoreOps.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return stored, err
	}

	// Record each ref only after its first successful listing write.
	key := ref.MarshalString()
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if _, exists := p.seen[key]; !exists {
		if _, err := p.writer.WriteString(key + "\n"); err != nil {
			return nil, err
		}
		p.seen[key] = struct{}{}
	}
	return stored, nil
}

// GetBlock reads bytes through the same listing as stored-block reads.
func (p *peerListing) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	stored, err := p.GetStoredBlock(ctx, ref)
	return stored.GetData(), stored != nil, err
}

// GetStoredBlockBatch lists each block as the sequential peer serves it.
func (p *peerListing) GetStoredBlockBatch(ctx context.Context, refs []*block.BlockRef, fn block.StoredBlockFunc) error {
	return block.GetStoredBlockEach(ctx, p, refs, fn)
}

// GetBlockExists lists peer hits even when a typed leaf needs only a presence check.
func (p *peerListing) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	stored, err := p.GetStoredBlock(ctx, ref)
	return stored != nil, err
}

// GetBlockExistsBatch checks each ref through the same listing as single reads.
func (p *peerListing) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Preserve input order while stopping at the first read or listing failure.
	found := make([]bool, len(refs))
	for i, ref := range refs {
		var err error
		found[i], err = p.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
	}
	return found, nil
}

// BeginReadOperation keeps scoped reads attached to the listing.
func (p *peerListing) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return p, func() {}, nil
}

// _ checks the peer listing's source and store contracts.
var (
	_ packcopy.Source          = (*peerListing)(nil)
	_ block.StoreOps           = (*peerListing)(nil)
	_ block.StoredBlockBatcher = (*peerListing)(nil)
)
