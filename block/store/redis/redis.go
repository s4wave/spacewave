package block_store_redis

import (
	"context"

	"github.com/aperturerobotics/bifrost/hash"
	block_store "github.com/aperturerobotics/hydra/block/store"
	block_store_kvtx "github.com/aperturerobotics/hydra/block/store/kvtx"
	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx_redis "github.com/aperturerobotics/hydra/store/kvtx/redis"
)

// RedisBlock is a block store on top of a Redis database.
// Stores blocks at {objectPrefix}/{block ref}
type RedisBlock = block_store_kvtx.KVTxBlock

// NewRedisBlock builds a new block store on top of a redis db.
//
// forceHashType can be 0 to use the default hash type.
func NewRedisBlock(
	ctx context.Context,
	kvk *kvkey.KVKey,
	st *store_kvtx_redis.Store,
	forceHashType hash.HashType,
) *RedisBlock {
	return block_store_kvtx.NewKVTxBlock(ctx, kvk, st, forceHashType)
}

// _ is a type assertion
var _ block_store.Store = ((*RedisBlock)(nil))
