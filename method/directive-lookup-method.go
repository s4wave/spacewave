package auth_method

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// AuthLookupMethod is a directive to search for a auth method by ID.
// At least one of the search fields should be set.
type AuthLookupMethod interface {
	// Directive indicates AuthLookupMethod is a directive.
	directive.Directive

	// AuthLookupMethodID is the auth method identifier.
	// Cannot be empty.
	AuthLookupMethodID() string
}

// AuthLookupMethodValue is the result of the AuthLookupMethod directive.
type AuthLookupMethodValue = Method

// ExAuthLookupMethod looks up a single instance of the auth method.
// Waits for the auth method to exist or ctx to be canceled.
func ExAuthLookupMethod(
	ctx context.Context,
	b bus.Bus,
	methodID string,
) (AuthLookupMethodValue, error) {
	val, valRef, err := bus.ExecOneOff(ctx, b, NewAuthLookupMethod(methodID), false, nil)
	if err != nil {
		return nil, err
	}
	valRef.Release()
	v, vOk := val.(AuthLookupMethodValue)
	if !vOk {
		return nil, errors.New("lookup auth method returned invalid type")
	}
	return v, nil
}

// ExAuthLookupMethods executes the LookupMethod directive.
// Returns all available implementations of method id.
// If method id is empty, returns all.
func ExAuthLookupMethods(
	ctx context.Context,
	b bus.Bus,
	methodID string,
) ([]AuthLookupMethodValue, directive.Reference, error) {
	vals, dirRef, err := bus.ExecCollectValues(ctx, b, NewAuthLookupMethod(methodID), nil)
	if err != nil {
		return nil, nil, err
	}
	res := make([]AuthLookupMethodValue, 0, len(vals))
	for _, v := range vals {
		dv, dvOk := v.(AuthLookupMethodValue)
		if dvOk {
			res = append(res, dv)
		}
	}
	return res, dirRef, nil
}

// lookupMethod implements AuthLookupMethod with a global id constraint.
type lookupMethod struct {
	id string
}

// NewAuthLookupMethod constructs a new lookupMethod directive.
func NewAuthLookupMethod(
	id string,
) AuthLookupMethod {
	return &lookupMethod{
		id: id,
	}
}

// AuthLookupMethodID is the method ID.
// Cannot be empty.
func (d *lookupMethod) AuthLookupMethodID() string {
	return d.id
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupMethod) Validate() error {
	if d.id == "" {
		return errors.New("id must be set")
	}

	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *lookupMethod) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		MaxValueCount:   1,
		MaxValueHardCap: true,
	}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupMethod) IsEquivalent(other directive.Directive) bool {
	ot, ok := other.(*lookupMethod)
	if !ok {
		return false
	}

	return ot.id == d.id
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupMethod) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupMethod) GetName() string {
	return "AuthLookupMethod"
}

// GetDebugVals returns the directive arguments as key/value pairs.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupMethod) GetDebugVals() directive.DebugValues {
	vals := directive.NewDebugValues()
	vals["id"] = []string{d.AuthLookupMethodID()}
	return vals
}

// _ is a type constraint
var _ AuthLookupMethod = ((*lookupMethod)(nil))
