package store_kvtx_redis

import "errors"

var (
	// ErrRedisUrlEmpty is returned if the redis url was empty.
	ErrRedisUrlEmpty = errors.New("redis url cannot be empty")
)
