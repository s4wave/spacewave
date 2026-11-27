package space_backup_test

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// copyOnlySource refuses the cache-filling read path while exposing the same
// fixture blocks through the existing CopySource contract.
type copyOnlySource struct {
	// StoreOps serves the real blocks when selected by GetCopySource.
	block.StoreOps
}

// GetCopySource exposes reads without the per-block source cache fills.
func (s copyOnlySource) GetCopySource() block.StoreOps { return s.StoreOps }

// GetBlock rejects a typing read through the cache-filling source handle.
func (s copyOnlySource) GetBlock(context.Context, *block.BlockRef) ([]byte, bool, error) {
	return nil, false, errors.New("backup used the cache-filling source")
}

// GetStoredBlock rejects a copy read through the cache-filling source handle.
func (s copyOnlySource) GetStoredBlock(context.Context, *block.BlockRef) (*block.StoredBlock, error) {
	return nil, errors.New("backup used the cache-filling source")
}

// _ is a type assertion.
var _ block.CopySource = copyOnlySource{}
