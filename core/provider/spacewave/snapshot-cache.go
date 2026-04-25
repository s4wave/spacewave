package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/util/refcount"
)

// getOrCreateSnapshotRefCount returns a keyed snapshot refcount, creating it on demand.
func getOrCreateSnapshotRefCount[K comparable, V comparable](
	cache *map[K]*refcount.RefCount[V],
	key K,
	resolve func(ctx context.Context, key K, released func()) (V, func(), error),
) *refcount.RefCount[V] {
	if *cache == nil {
		*cache = make(map[K]*refcount.RefCount[V])
	}
	rc := (*cache)[key]
	if rc == nil {
		rc = refcount.NewRefCountWithOptions(
			context.Background(),
			true,
			nil,
			nil,
			func(ctx context.Context, released func()) (V, func(), error) {
				return resolve(ctx, key, released)
			},
			snapshotRefCountOptions,
		)
		(*cache)[key] = rc
	}
	return rc
}

// getOrCreateSingletonSnapshotRefCount returns a singleton snapshot refcount.
func getOrCreateSingletonSnapshotRefCount[V comparable](
	rc **refcount.RefCount[V],
	resolve func(ctx context.Context, released func()) (V, func(), error),
) *refcount.RefCount[V] {
	if *rc == nil {
		*rc = refcount.NewRefCountWithOptions(
			context.Background(),
			true,
			nil,
			nil,
			resolve,
			snapshotRefCountOptions,
		)
	}
	return *rc
}
