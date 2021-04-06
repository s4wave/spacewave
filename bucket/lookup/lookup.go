package bucket_lookup

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/volume"
)

// LookupBlockOpts contains additional options for the lookup block call.
type LookupBlockOpts struct {
	// LocalOnly indicates we should not use the network for the call.
	LocalOnly bool
}

// LookupBlockOption is a option for the LookupBlock call.
type LookupBlockOption func(opts *LookupBlockOpts)

// NewLookupBlockOpts builds opts from the options set.
func NewLookupBlockOpts(opts ...LookupBlockOption) *LookupBlockOpts {
	o := &LookupBlockOpts{}
	for _, op := range opts {
		op(o)
	}
	return o
}

// WithLocalOnly indicates we should only use the local volumes for the call.
func WithLocalOnly() LookupBlockOption {
	return func(opts *LookupBlockOpts) {
		opts.LocalOnly = true
	}
}

// Lookup are the lookup operations.
type Lookup interface {
	// LookupBlock searches for a block using the bucket lookup controller.
	// If lookup is disabled, will return an error.
	LookupBlock(
		reqCtx context.Context,
		ref *block.BlockRef,
		opts ...LookupBlockOption,
	) ([]byte, bool, error)
	// PutBlock writes a block using the bucket lookup controller.
	// The behavior of the write-back is configured in the lookup controller.
	// If lookup is disabled, will return an error.
	// Optionally returns true for second return value if all existed already.
	PutBlock(
		reqCtx context.Context,
		data []byte, opts *block.PutOpts,
	) ([]*bucket.ObjectRef, bool, error)
}

// Handle looks up data from a bucket independent of volume.
// Calls are bounded by the handle and request contexts.
// Will be terminated when bucket config value changes.
type Handle interface {
	// GetContext returns the context of the lookup handle.
	GetContext() context.Context
	// GetBucketConfig returns the current in-use bucket config.
	// Will be nil if the bucket is not known.
	GetBucketConfig() *bucket.Config
	// GetLookup returns the lookup handle.
	// Will return nil if the bucket config is not yet known.
	GetLookup(ctx context.Context) (Lookup, error)
}

// Controller manages calls against a bucket across multiple buckets.
type Controller interface {
	// Controller indicates the lookup controller is a controller.
	controller.Controller
	// Lookup indicates the controller implements the lookup methods.
	Lookup

	// PushBucketHandles pushes the bucket handle list that the controller may
	// use to service requests. The controller should wait for this to be called
	// before beginning to service requests. The bucket handles pushed will
	// always have GetExists() == true.
	PushBucketHandles(ctx context.Context, handles []volume.BucketHandle)
}

// Config is the minimum requirement for a lookup config object.
type Config interface {
	// Config indicates the config is a config object.
	config.Config

	// GetBucketConf returns the bucket config.
	GetBucketConf() *bucket.Config
	// SetBucketConf sets the bucket config.
	SetBucketConf(c *bucket.Config)
}
