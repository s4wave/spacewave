package s4db

import (
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
)

// Tx is a transaction of the database.
type Tx struct {
	kvtx.Tx
}

// ScanPrefixLens calls cb with each key under prefix and its value length.
func (t *Tx) ScanPrefixLens(ctx context.Context, prefix []byte, cb func(key []byte, n int) error) error {
	return ErrUnimplemented
}
