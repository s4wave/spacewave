package packmove

import (
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// SwitchConfig configures Switch.
type SwitchConfig struct {
	// Source is the volume store to copy. No other process may write it.
	Source kvtx.Store
	// Dest is the empty store that receives the copy.
	Dest kvtx.Store
	// Keys names the key layout of both stores.
	Keys *store_kvkey.KVKey
	// LogPaths are the pack logs Pack wrote, one per key range, in key order.
	LogPaths []string
	// WorldStoreID is the bucket ID of the World's block store, as Pack was
	// given it.
	WorldStoreID string
	// WritebackPrefix is the key prefix of the World store's write-back object
	// store: the block store's GetObjectStorePrefixByID of its write-back
	// object store ID. The copy leaves out every key under it, which holds the
	// source's upload and eviction records, and writes fresh upload markers
	// under it.
	WritebackPrefix []byte
	// OmitGraph leaves out the reference graph, so the destination's
	// collector starts with an empty graph. The packed blocks the graph
	// described are in packs, which no collector deletes.
	OmitGraph bool
	// SkipBlocks seeks past the whole block range without checking each block
	// against the logs. Use it only when the logs cover the range end to end,
	// as a finished sharded Pack run's do, and nothing wrote the source since:
	// a block the logs miss is then left out of both the copy and the packs.
	SkipBlocks bool
	// Progress receives the copy's counts after each commit; nil reports nothing.
	Progress func(volume_kvtx.CopyProgress)
}
