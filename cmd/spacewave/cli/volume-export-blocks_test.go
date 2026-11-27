//go:build !js

package spacewave_cli

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
)

// volumeExportBlocks supplies raw source bytes and receiver presence from memory.
type volumeExportBlocks map[string][]byte

// GetStoredBlock returns fixture bytes or nil when the source lacks the ref.
func (s volumeExportBlocks) GetStoredBlock(_ context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	data, ok := s[ref.MarshalString()]
	if !ok {
		return nil, nil
	}
	return &block.StoredBlock{Data: data}, nil
}

// HasBlock checks receiver membership without reading fixture bytes.
func (s volumeExportBlocks) HasBlock(_ context.Context, ref *block.BlockRef) (bool, error) {
	_, ok := s[ref.MarshalString()]
	return ok, nil
}

// _ is a type assertion.
var (
	_ packcopy.Source     = volumeExportBlocks(nil)
	_ volumeBlockPresence = volumeExportBlocks(nil)
)
