package block_store_s3

import (
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
)

// PackStoreOptions configures a PackStore.
type PackStoreOptions struct {
	// Pricing is the price list of the service holding the bucket. nil uses
	// PricingForEndpoint for a *Client and the Amazon S3 prices otherwise.
	Pricing *Pricing
	// KeepPacks stops compaction and reclaim, so the store only ever adds
	// packfiles.
	KeepPacks bool
	// RefreshInterval is the time between two listings of the packfile
	// entries, which find the packfiles other writers added. Zero uses
	// DefaultRefreshInterval.
	RefreshInterval time.Duration
	// Index is a local KV store for the entry cache and the block location
	// index. nil keeps neither. The store owns every key in it, so pass a
	// prefixed view.
	//
	// With it, a start reads the index's pack table and the entries of its
	// unindexed packs, and lists the bucket in the background, unless
	// SoleWriter is set, reading only the entries the table does not list.
	// A lookup reads the entry of the pack the index places its block in
	// from the index, so a start never reads every entry. A background
	// routine indexes the listed packs: one read of the end of each pack
	// and a few short write transactions, with a pause between them. It
	// takes no lock a read waits on. Until a pack is indexed, lookups test
	// its bloom filter as without the index.
	Index kvtx.Store
	// ExactReads makes a block miss fetch only the block's bytes from its
	// pack. False uses the pack reader's adaptive window, which suits HTTP
	// range requests.
	ExactReads bool
	// Linger is how long a short upload batch of settled blocks waits for more
	// blocks before it is written anyway. Zero uses packLinger.
	Linger time.Duration
	// MergeTailBytes merges a batch, when not zero, into the newest packfile
	// smaller than this many bytes while the merged packfile fits one upload
	// batch, and deletes the packfile it replaces, even with KeepPacks set.
	MergeTailBytes int64
	// MaxPackBytes bounds one packfile a batch writes. Zero uses
	// writer.DefaultMaxPackBytes. A batch's block bytes are bounded by half
	// of it and its block count in proportion to it, as for the default
	// limit. Larger packs mean fewer objects and index rows, and each
	// upload holds about three copies of its blocks in memory. Readers take
	// a pack of any size.
	MaxPackBytes int64
	// ReadCache serves block extents before the bucket. Nil reads the bucket.
	ReadCache RangeCache
	// SoleWriter says no other writer adds packfiles under the prefix, so the
	// store lists the bucket only for its first load, and not even then when
	// the index's table holds a pack: the store's own writes keep its entries
	// and index current. RefreshInterval is then unused.
	SoleWriter bool
	// IndexOnly makes the index the whole record of the store's packs: the
	// store never lists the bucket, not at a start with an empty table nor
	// after a read that finds a pack gone, so a block the index does not place
	// costs no request. Packs under the prefix that another writer left stay
	// invisible. The one exception is an index RequestPackListing marked,
	// such as one restored from a backup: the first load lists the bucket
	// once and adds the packs the table lacks. It implies SoleWriter and needs
	// Index; without Index it is ignored.
	IndexOnly bool
	// ReadOnlyIndex makes the store write nothing to Index: a listing
	// publishes the packs it finds in memory only, a removed pack leaves its
	// rows, an index tail a read fetched is not kept, and no routine indexes
	// the listed packs. Lookups still read the index and test the bloom filter
	// of every pack it does not place. It is for a store that only serves
	// reads, beside a disk that must not grow.
	ReadOnlyIndex bool
}
