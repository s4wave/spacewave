package store_kvtx_redis

import (
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
	iterator "github.com/s4wave/spacewave/db/kvtx/iterator"
)

// NewIterator constructs a new iterator.
func NewIterator(
	ctx context.Context,
	ops iterator.Ops,
	prefix []byte,
	sort, reverse bool,
) kvtx.Iterator {
	// TODO: implement a faster sorted iteration backed by a Redis sorted set
	// of keys; this buffers all keys in memory via ScanPrefixKeys.
	return iterator.NewIterator(ctx, ops, prefix, sort, reverse)
}
