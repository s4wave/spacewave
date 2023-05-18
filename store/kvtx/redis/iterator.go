package store_kvtx_redis

import (
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	iterator "github.com/aperturerobotics/hydra/kvtx/iterator"
)

// NewIterator constructs a new iterator.
func NewIterator(
	ctx context.Context,
	ops iterator.Ops,
	prefix []byte,
	sort, reverse bool,
) kvtx.Iterator {
	// buffers all keys in memory (uses ScanPrefixKeys)
	return iterator.NewIterator(ctx, ops, prefix, sort, reverse)

	/* TODO: Redis: implement a faster sorted iteration (sorted set of keys)
	return &Iterator{
		conn:    conn,
		prefix:  prefix,
		sort:    sort,
		reverse: reverse,
	}
	*/
}

// _ is a type assertion
// var _ kvtx.Iterator = ((*Iterator)(nil))
