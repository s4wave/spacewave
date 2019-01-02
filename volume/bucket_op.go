package volume

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// bucketOperation is an ongoing bucket operation, implements BucketHandle.
type bucketOperation struct {
	BucketHandle
	// diRef is the directive instance ref
	diRef directive.Reference
}

// Close releases the operation.
func (op *bucketOperation) Close() {
	if op.BucketHandle != nil {
		op.BucketHandle.Close()
	}
	if op.diRef != nil {
		op.diRef.Release()
	}
}

// StartBucketOperation acquires a bucket handle following the bucket operation
// arguments, and returns the handle.
func StartBucketOperation(
	ctx context.Context,
	b bus.Bus,
	args *BucketOpArgs,
) (BucketHandle, error) {
	if err := args.Validate(); err != nil {
		return nil, err
	}

	valCh := make(chan BuildBucketAPIValue, 3)
	pushVal := func(val directive.Value) {
		v, ok := val.(BuildBucketAPIValue)
		if !ok {
			return
		}
		select {
		case valCh <- v:
		default:
		}
	}
	_, diRef, err := b.AddDirective(
		NewBuildBucketAPI(
			args.GetBucketId(),
			args.GetVolumeId(),
		),
		bus.NewCallbackHandler(func(av directive.AttachedValue) {
			pushVal(av.GetValue())
		}, nil, nil),
	)
	if err != nil {
		return nil, err
	}
	for {
		select {
		case <-ctx.Done():
			diRef.Release()
			return nil, ctx.Err()
		case v := <-valCh:
			if !v.GetExists() {
				if !args.GetWaitBucket() {
					diRef.Release()
					return nil, errors.New("bucket not found")
				}
				// wait for the bucket to be found.
			} else {
				bop := &bucketOperation{diRef: diRef, BucketHandle: v}
				go func() {
					select {
					case <-ctx.Done():
					case <-v.GetContext().Done():
					}
					bop.Close()
				}()
				return bop, nil
			}
		}
	}
}
