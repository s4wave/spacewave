package world

import (
	"github.com/aperturerobotics/controllerbus/directive"
)

// ApplyWorldOp is a directive to apply a hydra world operation.
type ApplyWorldOp interface {
	// Directive indicates ApplyWorldOp is a directive.
	directive.Directive

	// ApplyWorldOpOperationTypeID returns the operation type ID.
	// Cannot be empty.
	ApplyWorldOpOperationTypeID() string
	// ApplyWorldOpEngineID returns the world engine ID.
	// Can be empty.
	ApplyWorldOpEngineID() string
}

// ApplyWorldOpValue is the result type for ApplyWorldOp.
type ApplyWorldOpValue = ApplyWorldOpFunc

// applyWorldOp is an in-memory ApplyWorldOp directive
type applyWorldOp struct {
	operationTypeID string
	engineID        string
}

// NewApplyWorldOp constructs an ApplyWorldOp.
func NewApplyWorldOp(operationTypeID, engineID string) ApplyWorldOp {
	return &applyWorldOp{operationTypeID: operationTypeID, engineID: engineID}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *applyWorldOp) Validate() error {
	if d.operationTypeID == "" {
		return ErrEmptyOp
	}
	return nil
}

// GetValueApplyWorldOpOptions returns options relating to value handling.
func (d *applyWorldOp) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// ApplyWorldOpOperationTypeID returns the bucket config.
func (d *applyWorldOp) ApplyWorldOpOperationTypeID() string {
	return d.operationTypeID
}

// ApplyWorldOpOperationTypeID returns the world engine ID.
func (d *applyWorldOp) ApplyWorldOpEngineID() string {
	return d.engineID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *applyWorldOp) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(ApplyWorldOp)
	if !ok {
		return false
	}

	if od.ApplyWorldOpEngineID() != d.ApplyWorldOpEngineID() {
		return false
	}
	if od.ApplyWorldOpOperationTypeID() != d.ApplyWorldOpOperationTypeID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *applyWorldOp) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *applyWorldOp) GetName() string {
	return "ApplyWorldOp"
}

// GetDebugString returns the directive arguments stringified.
func (d *applyWorldOp) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["operation-type-id"] = []string{d.ApplyWorldOpOperationTypeID()}
	if engineID := d.ApplyWorldOpEngineID(); engineID != "" {
		vals["engine-id"] = []string{engineID}
	}
	return vals
}

// _ is a type assertion
var _ ApplyWorldOp = ((*applyWorldOp)(nil))
