package bucket_lookup

import (
	"context"
	"errors"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// BuildBucketLookup is a directive to build a bucket lookup handle.
type BuildBucketLookup interface {
	// Directive indicates BuildBucketLookup is a directive.
	directive.Directive

	// BuildBucketLookupBucketID returns the bucket ID constraint.
	// Cannot be empty.
	BuildBucketLookupBucketID() string
}

// BuildBucketLookupValue is the result type for BuildBucketLookup.
// The value is removed and replaced when any values change.
type BuildBucketLookupValue = Handle

// buildBucketLookup implements BuildBucketLookup
type buildBucketLookup struct {
	bucketID, volumeID string
}

// NewBuildBucketLookup constructs a new BuildBucketLookup directive.
func NewBuildBucketLookup(bucketID string) BuildBucketLookup {
	return &buildBucketLookup{bucketID: bucketID}
}

// ExBuildBucketLookup executes the BuildBucketLookup directive.
func ExBuildBucketLookup(
	ctx context.Context,
	b bus.Bus,
	returnIfIdle bool,
	bucketID string,
	disposeCb func(),
) (BuildBucketLookupValue, directive.Instance, directive.Reference, error) {
	bv, di, bvRef, err := bus.ExecWaitValue[BuildBucketLookupValue](
		ctx,
		b,
		NewBuildBucketLookup(bucketID),
		bus.ReturnIfIdle(returnIfIdle),
		disposeCb,
		nil,
	)
	if err != nil {
		if bvRef != nil {
			bvRef.Release()
		}
		return nil, nil, nil, err
	}
	return bv, di, bvRef, nil
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *buildBucketLookup) Validate() error {
	if d.bucketID == "" {
		return errors.New("bucket id cannot be empty")
	}

	return nil
}

// GetValueBuildBucketLookupOptions returns options relating to value handling.
func (d *buildBucketLookup) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur: time.Second * 3,
	}
}

// BuildBucketLookupBucketIDRe returns the bucket ID constraint.
func (d *buildBucketLookup) BuildBucketLookupBucketID() string {
	return d.bucketID
}

// BuildBucketLookupVolumeID returns the volume ID constraint.
func (d *buildBucketLookup) BuildBucketLookupVolumeID() string {
	return d.volumeID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *buildBucketLookup) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(BuildBucketLookup)
	if !ok {
		return false
	}

	if d.BuildBucketLookupBucketID() != od.BuildBucketLookupBucketID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *buildBucketLookup) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *buildBucketLookup) GetName() string {
	return "BuildBucketLookup"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *buildBucketLookup) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["bucket-id"] = []string{d.BuildBucketLookupBucketID()}
	return vals
}

// _ is a type assertion
var _ BuildBucketLookup = ((*buildBucketLookup)(nil))
