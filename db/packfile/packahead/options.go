package packahead

import (
	"os"
	"time"

	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/s4wave/spacewave/db/packfile/writer"
)

// Options configures a Log.
type Options struct {
	// MaxPackBytes bounds a finalized pack, its index included. Zero uses
	// writer.DefaultMaxPackBytes, the bound every pack reader accepts.
	MaxPackBytes int64
	// MaxPackBlocks bounds the values in a pack. Zero uses
	// writer.DefaultMaxBlocksPerPack.
	MaxPackBlocks int
	// Linger rotates the open pack once its first batch is this old, so a
	// trickle of writes still reaches the upload. Zero rotates only when full.
	Linger time.Duration
	// PackID names a finalized pack from its contents. Nil names each pack
	// with identity.NewPackID, as a store that deletes packs needs.
	PackID func(result *writer.PackResult) (string, error)
	// ReserveBytes is the free disk an Append must leave: one whose blocks
	// would leave less returns ErrReserve and writes nothing, so the log never
	// takes the space another store on the disk needs. Zero keeps none.
	ReserveBytes int64
	// FreeBytes returns the disk space free on the file system holding the
	// Log's directory. Nil measures it with statfs.
	FreeBytes func(dir string) (uint64, error)
	// SyncData makes the batches written to a pack file durable, the sync
	// writers share. Nil uses fdatasync, F_FULLFSYNC on darwin. A test sets
	// it to order writers around a sync.
	SyncData func(f *os.File) error
}

// withDefaults returns the options with each zero field at its default.
func (o Options) withDefaults() Options {
	// Default each zero field.
	if o.MaxPackBytes <= 0 {
		o.MaxPackBytes = writer.DefaultMaxPackBytes
	}
	if o.MaxPackBlocks <= 0 {
		o.MaxPackBlocks = int(writer.DefaultMaxBlocksPerPack)
	}
	if o.FreeBytes == nil {
		o.FreeBytes = freeDiskBytes
	}
	if o.SyncData == nil {
		o.SyncData = syncData
	}
	if o.PackID == nil {
		o.PackID = func(*writer.PackResult) (string, error) { return identity.NewPackID(), nil }
	}
	return o
}
