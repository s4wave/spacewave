package space_backup_test

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
)

// hiddenRoot hides the World block from the copy but leaves typing reads intact.
type hiddenRoot struct {
	// StoreOps supplies the other source operations.
	block.StoreOps
	// root is deliberately absent from this source's copy reads.
	root *block.BlockRef
}

// GetStoredBlock injects a reachable hole, which must prevent index publication.
func (s hiddenRoot) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if ref.EqualsRef(s.root) {
		return nil, nil
	}
	return s.StoreOps.GetStoredBlock(ctx, ref)
}

// _ is a type assertion.
var _ packcopy.Source = hiddenRoot{}
