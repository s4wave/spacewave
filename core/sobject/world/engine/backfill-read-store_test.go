//go:build !js

package sobject_world_engine_test

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// backfillReadStore gates the real graph-copy source without replacing its data.
type backfillReadStore struct {
	block.StoreOps
	// wait holds dependency delivery until release or lifetime cancellation.
	wait func(context.Context) error
}

// GetStoredBlock keeps a copy in flight until the gate opens or its context ends.
func (s *backfillReadStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Controller retirement must cancel this read before closing its snapshot.
	if err := s.wait(ctx); err != nil {
		return nil, err
	}
	return s.StoreOps.GetStoredBlock(ctx, ref)
}
