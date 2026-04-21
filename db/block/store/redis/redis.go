package block_store_redis

import (
	"github.com/s4wave/spacewave/db/block"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_redis "github.com/s4wave/spacewave/db/store/kvtx/redis"
	"github.com/s4wave/spacewave/net/hash"
)

// RedisBlock is a block store on top of a Redis database.
// Stores blocks at {objectPrefix}/{block ref}
type RedisBlock = block_store_kvtx.KVTxBlock

// NewRedisBlock builds a new block store on top of a redis db.
//
// forceHashType can be 0 to use the default hash type.
// hashGet hashes Get requests for integrity, use if the storage is unreliable or untrusted.
func NewRedisBlock(
	kvk *kvkey.KVKey,
	st *store_kvtx_redis.Store,
	forceHashType hash.HashType,
	hashGet bool,
) *RedisBlock {
	return block_store_kvtx.NewKVTxBlock(kvk, st, forceHashType, hashGet)
}

// _ is a type assertion
var _ block.StoreOps = ((*RedisBlock)(nil))
