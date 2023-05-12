package volume

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
)

// BuildBucketAPI is a directive to get an API handle for a storage bucket.
type BuildBucketAPI interface {
	// Directive indicates BuildBucketAPI is a directive.
	directive.Directive

	// BuildBucketAPIBucketID returns the bucket ID constraint.
	// Cannot be empty.
	BuildBucketAPIBucketID() string
	// BuildBucketAPIVolumeID returns the volume ID constraint.
	// Cannot be empty.
	BuildBucketAPIVolumeID() string
}

// BuildBucketAPIValue is the result type for BuildBucketAPI.
// The value is removed and replaced when any values change.
type BuildBucketAPIValue = BucketHandle

// buildBucketAPI implements BuildBucketAPI
type buildBucketAPI struct {
	bucketID, volumeID string
}

// NewBuildBucketAPI constructs a new BuildBucketAPI directive.
func NewBuildBucketAPI(bucketID, volumeID string) BuildBucketAPI {
	return &buildBucketAPI{bucketID: bucketID, volumeID: volumeID}
}

// ExBuildBucketAPI executes the BuildBucketAPI directive.
func ExBuildBucketAPI(
	ctx context.Context,
	b bus.Bus,
	returnIfIdle bool,
	bucketID, volumeID string,
	valDisposeCb func(),
) (BuildBucketAPIValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[BuildBucketAPIValue](
		ctx,
		b,
		NewBuildBucketAPI(bucketID, volumeID),
		bus.ReturnIfIdle(returnIfIdle),
		valDisposeCb,
		nil,
	)
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *buildBucketAPI) Validate() error {
	if d.bucketID == "" {
		return bucket.ErrBucketIDEmpty
	}
	if d.volumeID == "" {
		return ErrVolumeIDEmpty
	}

	return nil
}

// GetValueBuildBucketAPIOptions returns options relating to value handling.
func (d *buildBucketAPI) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur: time.Second * 3,
	}
}

// BuildBucketAPIBucketIDRe returns the bucket ID constraint.
func (d *buildBucketAPI) BuildBucketAPIBucketID() string {
	return d.bucketID
}

// BuildBucketAPIVolumeID returns the volume ID constraint.
func (d *buildBucketAPI) BuildBucketAPIVolumeID() string {
	return d.volumeID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *buildBucketAPI) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(BuildBucketAPI)
	if !ok {
		return false
	}

	if d.BuildBucketAPIVolumeID() != od.BuildBucketAPIVolumeID() {
		return false
	}

	if d.BuildBucketAPIBucketID() != od.BuildBucketAPIBucketID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *buildBucketAPI) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *buildBucketAPI) GetName() string {
	return "BuildBucketAPI"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *buildBucketAPI) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["bucket-id"] = []string{d.BuildBucketAPIBucketID()}
	vals["volume-id"] = []string{d.BuildBucketAPIVolumeID()}
	return vals
}

// _ is a type assertion
var _ BuildBucketAPI = ((*buildBucketAPI)(nil))
