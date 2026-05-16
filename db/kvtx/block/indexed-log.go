package kvtx_block

import (
	"context"
	"encoding/binary"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// IndexedLogKey encodes a monotonic uint64 log index as a big-endian key.
func IndexedLogKey(index uint64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, index)
	return key
}

// NextIndexedLogIndex returns the next append index for an indexed log tree.
func NextIndexedLogIndex(ctx context.Context, tree kvtx.BlockTx) (uint64, error) {
	it := tree.BlockIterate(ctx, nil, true, true)
	defer it.Close()
	if !it.Next() {
		return 0, it.Err()
	}
	key := it.Key()
	if len(key) != 8 {
		return 0, errors.Errorf("invalid indexed log key length %d", len(key))
	}
	return binary.BigEndian.Uint64(key) + 1, it.Err()
}
