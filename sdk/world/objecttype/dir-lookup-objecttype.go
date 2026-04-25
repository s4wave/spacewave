package objecttype

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupObjectType is a directive to look up an object type factory.
type LookupObjectType interface {
	// Directive indicates LookupObjectType is a directive.
	directive.Directive

	// LookupObjectTypeID returns the object type ID to lookup.
	LookupObjectTypeID() string
}

// LookupObjectTypeValue is the result type for LookupObjectType.
type LookupObjectTypeValue = ObjectType

// ExLookupObjectType executes a lookup for a single object type on the bus.
//
// typeID is the object type identifier to lookup.
// Returns the ObjectType or nil if not found.
func ExLookupObjectType(
	ctx context.Context,
	b bus.Bus,
	typeID string,
) (ObjectType, directive.Reference, error) {
	av, _, avRef, err := bus.ExecOneOffTyped[LookupObjectTypeValue](ctx, b, NewLookupObjectType(typeID), bus.ReturnWhenIdle(), nil)
	if err != nil {
		return nil, nil, err
	}
	if av == nil {
		if avRef != nil {
			avRef.Release()
		}
		return nil, nil, nil
	}
	return av.GetValue(), avRef, nil
}

// lookupObjectType implements LookupObjectType
type lookupObjectType struct {
	objectTypeID string
}

// NewLookupObjectType constructs a new LookupObjectType directive.
func NewLookupObjectType(objectTypeID string) LookupObjectType {
	return &lookupObjectType{
		objectTypeID: objectTypeID,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupObjectType) Validate() error {
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *lookupObjectType) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// LookupObjectTypeID returns the object type ID to lookup.
func (d *lookupObjectType) LookupObjectTypeID() string {
	return d.objectTypeID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupObjectType) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupObjectType)
	if !ok {
		return false
	}

	if d.LookupObjectTypeID() != od.LookupObjectTypeID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupObjectType) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupObjectType) GetName() string {
	return "LookupObjectType"
}

// GetDebugVals returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupObjectType) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if d.objectTypeID != "" {
		vals["object-type-id"] = []string{d.objectTypeID}
	}
	return vals
}

// _ is a type assertion
var _ LookupObjectType = ((*lookupObjectType)(nil))
