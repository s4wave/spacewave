package volume

import (
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/object"
)

// BuildObjectStoreAPI is a directive to get API handles to object store.
type BuildObjectStoreAPI interface {
	// Directive indicates BuildObjectStoreAPI is a directive.
	directive.Directive

	// BuildObjectStoreAPIStoreID returns the object store ID.
	// Cannot be empty.
	BuildObjectStoreAPIStoreID() string
	// BuildObjectStoreAPIVolumeID returns the volume ID constraint.
	// Can be empty to select any volume.
	BuildObjectStoreAPIVolumeID() string
}

// BuildObjectStoreAPIValue is the result type for BuildObjectStoreAPI.
// The value is removed and replaced when any values change.
type BuildObjectStoreAPIValue = ObjectStoreHandle

// buildObjectStoreAPI implements BuildObjectStoreAPI
type buildObjectStoreAPI struct {
	objectStoreID, volumeID string
}

// NewBuildObjectStoreAPI constructs a new BuildObjectStoreAPI directive.
func NewBuildObjectStoreAPI(objectStoreID, volumeID string) BuildObjectStoreAPI {
	return &buildObjectStoreAPI{
		objectStoreID: objectStoreID,
		volumeID:      volumeID,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *buildObjectStoreAPI) Validate() error {
	if d.objectStoreID == "" {
		return object.ErrEmptyObjectStoreId
	}

	return nil
}

// GetValueBuildObjectStoreAPIOptions returns options relating to value handling.
func (d *buildObjectStoreAPI) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur:            time.Millisecond * 500,
		UnrefDisposeEmptyImmediate: true,
	}
}

// BuildObjectStoreAPIStoreID returns the object store ID constraint.
func (d *buildObjectStoreAPI) BuildObjectStoreAPIStoreID() string {
	return d.objectStoreID
}

// BuildObjectStoreAPIVolumeID returns the volume ID constraint.
func (d *buildObjectStoreAPI) BuildObjectStoreAPIVolumeID() string {
	return d.volumeID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *buildObjectStoreAPI) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(BuildObjectStoreAPI)
	if !ok {
		return false
	}

	if d.BuildObjectStoreAPIVolumeID() != od.BuildObjectStoreAPIVolumeID() {
		return false
	}

	if d.BuildObjectStoreAPIStoreID() != od.BuildObjectStoreAPIStoreID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *buildObjectStoreAPI) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *buildObjectStoreAPI) GetName() string {
	return "BuildObjectStoreAPI"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *buildObjectStoreAPI) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["store-id"] = []string{d.BuildObjectStoreAPIStoreID()}
	vals["volume-id"] = []string{d.BuildObjectStoreAPIVolumeID()}
	return vals
}

// _ is a type assertion
var _ BuildObjectStoreAPI = ((*buildObjectStoreAPI)(nil))
