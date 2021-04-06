package bucket_lookup

import (
	"context"
	"errors"

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
	av1, diRef1, err := bus.ExecOneOff(
		ctx,
		b,
		NewBuildBucketLookup(args.GetBucketId()),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	blv, ok := av1.GetValue().(BuildBucketLookupValue)
	if !ok {
		diRef1.Release()
		return nil, nil, errors.New("build bucket lookup returned invalid value")
	}
	readHandle := NewBucketFromHandle(ctx, blv)

	// 2. acquire the write handle
	var writeHandle bucket.Bucket
	rels := []func(){diRef1.Release}
	rel := func() {
		for _, r := range rels {
			r()
		}
	}
	if volID := args.GetVolumeId(); volID != "" {
		av2, diRef2, err := bus.ExecOneOff(
			ctx,
			b,
			volume.NewBuildBucketAPI(args.GetBucketId(), volID),
			nil,
		)
		if err != nil {
			rel()
			return nil, nil, err
		}
		rels = append(rels, diRef2.Release)
		bhv, ok := av2.GetValue().(volume.BuildBucketAPIValue)
		if !ok {
			rel()
			return nil, nil, errors.New("build bucket api returned invalid value")
		}
		if !bhv.GetExists() {
			rel()
			return nil, nil, errors.New("bucket does not exist in volume")
		}
		writeHandle = bhv.GetBucket()
	}

	return bucket.NewBucketRW(readHandle, writeHandle), rel, nil
}
