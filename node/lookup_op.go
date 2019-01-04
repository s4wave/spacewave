package node

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/volume"
)

// bucketLookupOperation is an ongoing bucket lookup operation
type bucketLookupOperation struct {
	bucket_lookup.Handle
	// diRef is the directive instance ref
	diRef directive.Reference
}

// Close releases the operation.
func (op *bucketLookupOperation) Close() {
	if op.diRef != nil {
		op.diRef.Release()
	}
}

// StartBucketLookupOperation acquires a bucket lookup handle following the
// bucket operation arguments, and returns the handle.
func StartBucketLookupOperation(
	ctx context.Context,
	b bus.Bus,
	args *volume.BucketOpArgs,
) (*bucketLookupOperation, error) {
	if err := args.Validate(); err != nil {
		return nil, err
	}

	valCh := make(chan BuildBucketLookupValue, 1)
	pushVal := func(val directive.Value) {
		v, ok := val.(BuildBucketLookupValue)
		if !ok {
			return
		}
		select {
		case valCh <- v:
		default:
		}
	}
	_, diRef, err := b.AddDirective(
		NewBuildBucketLookup(
			args.GetBucketId(),
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
			if v.GetBucketConfig() == nil {
				if !args.GetWaitBucket() {
					diRef.Release()
					return nil, errors.New("bucket not found")
				}
				// wait for the bucket to be found.
			} else {
				bop := &bucketLookupOperation{diRef: diRef, Handle: v}
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
