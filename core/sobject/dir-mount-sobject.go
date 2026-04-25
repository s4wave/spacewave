package sobject

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// MountSharedObject is a directive to mount a shared object with a provider account.
type MountSharedObject interface {
	// Directive indicates MountSharedObject is a directive.
	directive.Directive

	// MountSharedObjectRef returns the shared object ref to mount.
	MountSharedObjectRef() *SharedObjectRef
}

// MountSharedObjectValue is the result type for MountSharedObject.
type MountSharedObjectValue = SharedObject

// ExMountSharedObject executes a lookup for a single provider on the bus.
//
// If returnIfIdle is set, returns when the directive becomes idle.
func ExMountSharedObject(
	ctx context.Context,
	b bus.Bus,
	ref *SharedObjectRef,
	returnIfIdle bool,
	valDisposeCb func(),
) (SharedObject, directive.Reference, error) {
	av, _, avRef, err := bus.ExecOneOffTyped[MountSharedObjectValue](
		ctx,
		b,
		NewMountSharedObject(ref),
		bus.ReturnIfIdle(returnIfIdle),
		valDisposeCb,
	)
	if err != nil {
		return nil, nil, err
	}
	if av == nil {
		avRef.Release()
		return nil, nil, nil
	}
	return av.GetValue(), avRef, nil
}

// mountSharedObject implements MountSharedObject
type mountSharedObject struct {
	ref *SharedObjectRef
}

// NewMountSharedObject constructs a new MountSharedObject directive.
func NewMountSharedObject(ref *SharedObjectRef) MountSharedObject {
	return &mountSharedObject{
		ref: ref,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *mountSharedObject) Validate() error {
	if err := d.ref.Validate(); err != nil {
		return err
	}
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *mountSharedObject) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// MountSharedObjectRef returns the shared object id to mount.
func (d *mountSharedObject) MountSharedObjectRef() *SharedObjectRef {
	return d.ref
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *mountSharedObject) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(MountSharedObject)
	if !ok {
		return false
	}

	return d.ref.EqualVT(od.MountSharedObjectRef())
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *mountSharedObject) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *mountSharedObject) GetName() string {
	return "MountSharedObject"
}

// GetDebugVals returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *mountSharedObject) GetDebugVals() directive.DebugValues {
	return directive.DebugValues{
		"sobject-id":  []string{d.ref.GetProviderResourceRef().GetId()},
		"provider-id": []string{d.ref.GetProviderResourceRef().GetProviderId()},
		"account-id":  []string{d.ref.GetProviderResourceRef().GetProviderAccountId()},
	}
}

// _ is a type assertion
var _ MountSharedObject = ((*mountSharedObject)(nil))
