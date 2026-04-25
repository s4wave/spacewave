package sobject

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// MountSharedObjectBody is a directive to mount the body of a shared object.
// This typically signals to start a controller which validates + processes the sobject ops.
type MountSharedObjectBody interface {
	// Directive indicates MountSharedObjectBody is a directive.
	directive.Directive

	// MountSharedObjectBodyRef returns the shared object ref to mount.
	MountSharedObjectBodyRef() *SharedObjectRef
	// MountSharedObjectBodyType returns the shared object body type.
	MountSharedObjectBodyType() string
}

// MountSharedObjectBodyValue is the result type for MountSharedObjectBody.
//
// This is the interface exposed by the shared object body handler on the "client side."
type MountSharedObjectBodyValue[T comparable] interface {
	// GetSharedObjectRef returns the shared object handle.
	GetSharedObjectRef() *SharedObjectRef
	// GetSharedObjectBodyType returns the shared object handle.
	GetSharedObjectBodyType() string
	// GetSharedObject returns the shared object handle.
	GetSharedObject() SharedObject
	// GetSharedObjectBody returns the shared object body handle.
	GetSharedObjectBody() T
}

// mountSharedObjectBodyValue implements MountSharedObjectBodyValue
type mountSharedObjectBodyValue[T comparable] struct {
	ref      *SharedObjectRef
	bodyType string
	obj      SharedObject
	body     T
}

// NewMountSharedObjectBodyValue constructs a new MountSharedObjectBodyValue.
func NewMountSharedObjectBodyValue[T comparable](
	ref *SharedObjectRef,
	bodyType string,
	obj SharedObject,
	body T,
) MountSharedObjectBodyValue[T] {
	return &mountSharedObjectBodyValue[T]{
		ref:      ref,
		bodyType: bodyType,
		obj:      obj,
		body:     body,
	}
}

// GetSharedObjectRef returns the shared object handle.
func (v *mountSharedObjectBodyValue[T]) GetSharedObjectRef() *SharedObjectRef {
	return v.ref
}

// GetSharedObjectBodyType returns the shared object handle.
func (v *mountSharedObjectBodyValue[T]) GetSharedObjectBodyType() string {
	return v.bodyType
}

// GetSharedObject returns the shared object handle.
func (v *mountSharedObjectBodyValue[T]) GetSharedObject() SharedObject {
	return v.obj
}

// GetSharedObjectBody returns the shared object body handle.
func (v *mountSharedObjectBodyValue[T]) GetSharedObjectBody() T {
	return v.body
}

// _ is a type assertion
var _ MountSharedObjectBodyValue[any] = ((*mountSharedObjectBodyValue[any])(nil))

// ExMountSharedObjectBody executes a directive to mount the body of a shared object.
//
// If returnIfIdle is set, returns when the directive becomes idle.
func ExMountSharedObjectBody[T comparable](
	ctx context.Context,
	b bus.Bus,
	ref *SharedObjectRef,
	bodyType string,
	returnIfIdle bool,
	valDisposeCb func(),
) (MountSharedObjectBodyValue[T], directive.Reference, error) {
	av, _, avRef, err := bus.ExecOneOffTyped[MountSharedObjectBodyValue[T]](
		ctx,
		b,
		NewMountSharedObjectBody(ref, bodyType),
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

// mountSharedObjectBody implements MountSharedObjectBody
type mountSharedObjectBody struct {
	ref      *SharedObjectRef
	bodyType string
}

// NewMountSharedObjectBody constructs a new MountSharedObjectBody directive.
func NewMountSharedObjectBody(ref *SharedObjectRef, bodyType string) MountSharedObjectBody {
	return &mountSharedObjectBody{
		ref:      ref,
		bodyType: bodyType,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *mountSharedObjectBody) Validate() error {
	if err := d.ref.Validate(); err != nil {
		return err
	}
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *mountSharedObjectBody) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// MountSharedObjectBodyRef returns the shared object id to mount.
func (d *mountSharedObjectBody) MountSharedObjectBodyRef() *SharedObjectRef {
	return d.ref
}

// MountSharedObjectBodyType returns the shared object body type.
func (d *mountSharedObjectBody) MountSharedObjectBodyType() string {
	return d.bodyType
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *mountSharedObjectBody) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(MountSharedObjectBody)
	if !ok {
		return false
	}

	return d.ref.EqualVT(od.MountSharedObjectBodyRef()) && d.bodyType == od.MountSharedObjectBodyType()
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *mountSharedObjectBody) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *mountSharedObjectBody) GetName() string {
	return "MountSharedObjectBody"
}

// GetDebugVals returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *mountSharedObjectBody) GetDebugVals() directive.DebugValues {
	return directive.DebugValues{
		"sobject-id":  []string{d.ref.GetProviderResourceRef().GetId()},
		"provider-id": []string{d.ref.GetProviderResourceRef().GetProviderId()},
		"account-id":  []string{d.ref.GetProviderResourceRef().GetProviderAccountId()},
		"body-type":   []string{d.bodyType},
	}
}

// _ is a type assertion
var _ MountSharedObjectBody = ((*mountSharedObjectBody)(nil))
