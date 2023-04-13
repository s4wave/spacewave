package bucket_lookup

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/volume"
)

// StartBucketRWOperation acquires a bucket lookup handle following the bucket
// operation arguments. If the volume ID is set, acquires a write handle to the
// bucket within the volume, and uses the lookup for reads, and the volume
// bucket handle for writes.
// Note: ignores WaitBucket field.
func StartBucketRWOperation(
	ctx context.Context,
	b bus.Bus,
	args *bucket.BucketOpArgs,
) (bucket.Bucket, func(), error) {
	if err := args.Validate(); err != nil {
		return nil, nil, err
	}

	// 1. acquire the lookup handle
	blv, _, blvRel, err := ExBuildBucketLookup(ctx, b, false, args.GetBucketId(), nil)
	if err != nil {
		return nil, nil, err
	}
	readHandle := NewBucketFromHandle(ctx, blv)

	// 2. acquire the write handle
	var writeHandle bucket.Bucket
	rels := []func(){blvRel.Release}
	rel := func() {
		for _, r := range rels {
			r()
		}
	}
	if volID := args.GetVolumeId(); volID != "" {
		bhv, _, bhvRef, err := volume.ExBuildBucketAPI(ctx, b, false, args.GetBucketId(), volID, nil)
		if err != nil {
			return nil, nil, err
		}
		if !bhv.GetExists() {
			bhvRef.Release()
			return nil, nil, volume.ErrBucketNotInVolume
		}
		rels = append(rels, bhvRef.Release)
		writeHandle = bhv.GetBucket()
	}

	return bucket.NewBucketRW(readHandle, writeHandle), rel, nil
}
