package world_block

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/sirupsen/logrus"
)

// OpenSnapshot opens a World root using only its block store and inline transform
// configuration. It requires no provider account or local volume. Callers own
// the store and must Close the returned engine before closing the store.
// The engine has no durable head publisher; recovery callers use read transactions.
func OpenSnapshot(ctx context.Context, le *logrus.Entry, store block.StoreOps, ref *bucket.ObjectRef) (*Engine, error) {
	// A missing root is not an empty backup and must not restore as one.
	if ref.GetRootRef().GetEmpty() {
		return nil, errors.New("snapshot root is empty")
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if !ref.GetTransformConfRef().GetEmpty() {
		return nil, errors.New("snapshot requires an inline transform configuration")
	}

	// Reuse the normal transform and World readers on the supplied immutable root.
	factories := transform_all.BuildFactorySet()
	transformer, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, factories, ref.GetTransformConf())
	if err != nil {
		return nil, err
	}
	cursor := bucket_lookup.NewCursor(ctx, nil, le, factories, store, transformer, ref.Clone(), &bucket.BucketOpArgs{BucketId: ref.GetBucketId()}, ref.GetTransformConf())
	return NewEngine(ctx, le, cursor, nil, nil, false)
}
