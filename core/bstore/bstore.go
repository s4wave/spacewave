package bstore

import (
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/sirupsen/logrus"
)

// BlockStore is the block store handle interface.
type BlockStore interface {
	// Store is the block store interface.
	block_store.Store
}

// Validate validates the block store ref.
func (r *BlockStoreRef) Validate() error {
	if err := r.GetProviderResourceRef().Validate(); err != nil {
		return err
	}
	return nil
}

// GetLogger adds debug values to the logger.
func (r *BlockStoreRef) GetLogger(le *logrus.Entry) *logrus.Entry {
	return r.GetProviderResourceRef().GetLogger(le)
}
