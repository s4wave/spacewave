package world

import (
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupWorldOp is a directive to lookup a world operation handler.
type LookupWorldOp interface {
	// Directive indicates LookupWorldOp is a directive.
	directive.Directive

	// LookupWorldOpOperationTypeID returns the operation type ID.
	// Cannot be empty.
	LookupWorldOpOperationTypeID() string
	// LookupWorldOpEngineID returns the world engine ID.
	// Can be empty.
	LookupWorldOpEngineID() string
}

// LookupWorldOpValue is the result type for LookupWorldOp.
type LookupWorldOpValue = LookupOp

// lookupWorldOp is an in-memory LookupWorldOp directive
type lookupWorldOp struct {
	operationTypeID string
	engineID        string
}

// NewLookupWorldOp constructs an LookupWorldOp.
// objectKey can be empty to indicate a world operation.
func NewLookupWorldOp(operationTypeID, engineID string) LookupWorldOp {
	return &lookupWorldOp{
		operationTypeID: operationTypeID,
		engineID:        engineID,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupWorldOp) Validate() error {
	if d.operationTypeID == "" {
		return ErrEmptyOp
	}
	return nil
}

// GetValueLookupWorldOpOptions returns options relating to value handling.
func (d *lookupWorldOp) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		UnrefDisposeDur:            time.Second,
		UnrefDisposeEmptyImmediate: true,
	}
}

// LookupWorldOpOperationTypeID returns the bucket config.
func (d *lookupWorldOp) LookupWorldOpOperationTypeID() string {
	return d.operationTypeID
}

// LookupWorldOpOperationTypeID returns the world engine ID.
func (d *lookupWorldOp) LookupWorldOpEngineID() string {
	return d.engineID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupWorldOp) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupWorldOp)
	if !ok {
		return false
	}

	if od.LookupWorldOpEngineID() != d.LookupWorldOpEngineID() {
		return false
	}
	if od.LookupWorldOpOperationTypeID() != d.LookupWorldOpOperationTypeID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupWorldOp) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupWorldOp) GetName() string {
	return "LookupWorldOp"
}

// GetDebugString returns the directive arguments stringified.
func (d *lookupWorldOp) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["operation-type-id"] = []string{d.LookupWorldOpOperationTypeID()}
	if engineID := d.LookupWorldOpEngineID(); engineID != "" {
		vals["engine-id"] = []string{engineID}
	}
	return vals
}

// _ is a type assertion
var _ LookupWorldOp = ((*lookupWorldOp)(nil))
