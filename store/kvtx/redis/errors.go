package store_kvtx_redis

import "errors"

// ErrRedisUrlEmpty is returned if the redis url was empty.
var ErrRedisUrlEmpty = errors.New("redis url cannot be empty")
