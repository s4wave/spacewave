package packahead

import (
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
}

// withDefaults returns the options with each zero field at its default.
func (o Options) withDefaults() Options {
	if o.MaxPackBytes <= 0 {
		o.MaxPackBytes = writer.DefaultMaxPackBytes
	}
	if o.MaxPackBlocks <= 0 {
		o.MaxPackBlocks = int(writer.DefaultMaxBlocksPerPack)
	}
	if o.PackID == nil {
		o.PackID = func(*writer.PackResult) (string, error) { return identity.NewPackID(), nil }
	}
	return o
}
