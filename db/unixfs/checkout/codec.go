package unixfs_checkout

import (
	"context"
	"io/fs"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// Codec supplies the tree's transform and archive policy. The checkout engine
// owns hash verification, resume, leases, staging and publication.
type Codec struct {
	// Read opens a decoded tree until its callback returns.
	Read func(context.Context, *logrus.Entry, block.StoreOps, *block.BlockRef, func(fs.FS) error) error
	// Prefetch walks the content DAG with bounded concurrent block reads.
	Prefetch func(context.Context, *logrus.Entry, block.StoreOps, *block.BlockRef, int, func(uint64, uint64)) error
	// Seed imports prior files with the same chunking and transform as publication.
	Seed func(context.Context, *logrus.Entry, fs.FS, hash.HashType) (block.StoreOps, error)
}
