package bucket_lookup

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
)

// StartBucketLookupOperation acquires a bucket lookup handle following the
// bucket operation arguments, and returns the handle.
func StartBucketLookupOperation(
	ctx context.Context,
	b bus.Bus,
	returnIfIdle bool,
	args *bucket.BucketOpArgs,
) (Handle, directive.Instance, directive.Reference, error) {
	if err := args.Validate(); err != nil {
		return nil, nil, nil, err
	}
	return bus.ExecWaitValue[Handle](
		ctx,
		b,
		NewBuildBucketLookup(args.GetBucketId()),
		bus.ReturnIfIdle(returnIfIdle),
		nil,
		nil,
	)
}
