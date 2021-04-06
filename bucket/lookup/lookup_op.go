package bucket_lookup

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
)

// StartBucketLookupOperation acquires a bucket lookup handle following the
// bucket operation arguments, and returns the handle.
func StartBucketLookupOperation(
	ctx context.Context,
	b bus.Bus,
	args *bucket.BucketOpArgs,
) (Handle, directive.Reference, error) {
	if err := args.Validate(); err != nil {
		return nil, nil, err
	}
	dv, diRef, err := bus.ExecOneOff(
		ctx,
		b,
		NewBuildBucketLookup(args.GetBucketId()),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	bv, ok := dv.GetValue().(BuildBucketLookupValue)
	if !ok {
		diRef.Release()
		return nil, nil, errors.New("build bucket lookup returned invalid type")
	}

	return bv, diRef, nil
}
