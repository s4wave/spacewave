package world

import (
	"github.com/aperturerobotics/controllerbus/directive"
)

// ApplyObjectOp is a directive to apply a hydra world object operation.
type ApplyObjectOp interface {
	// Directive indicates ApplyObjectOp is a directive.
	directive.Directive

	// ApplyObjectOpOperationTypeID returns the operation type ID.
	// Cannot be empty.
	ApplyObjectOpOperationTypeID() string
	// ApplyObjectOpObjectID returns the operation object key.
	// Cannot be empty.
	ApplyObjectOpObjectID() string
	// ApplyObjectOpEngineID returns the world engine ID.
	// Can be empty.
	ApplyObjectOpEngineID() string
}

// ApplyObjectOpValue is the result type for ApplyObjectOp.
type ApplyObjectOpValue = ApplyObjectOpFunc

// applyObjectOp is an in-memory ApplyObjectOp directive
type applyObjectOp struct {
	operationTypeID string
	objectID        string
	engineID        string
}

// NewApplyObjectOp constructs an ApplyObjectOp.
func NewApplyObjectOp(operationTypeID, objectID, engineID string) ApplyObjectOp {
	return &applyObjectOp{
		operationTypeID: operationTypeID,
		objectID:        objectID,
		engineID:        engineID,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *applyObjectOp) Validate() error {
	if d.operationTypeID == "" {
		return ErrEmptyOp
	}
	return nil
}

// GetValueApplyObjectOpOptions returns options relating to value handling.
func (d *applyObjectOp) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// ApplyObjectOpOperationTypeID returns the bucket config.
func (d *applyObjectOp) ApplyObjectOpOperationTypeID() string {
	return d.operationTypeID
}

// ApplyObjectOpOperationTypeID returns the world engine ID.
func (d *applyObjectOp) ApplyObjectOpEngineID() string {
	return d.engineID
}

// ApplyObjectOpOperationTypeID returns the world engine object ID.
func (d *applyObjectOp) ApplyObjectOpObjectID() string {
	return d.objectID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *applyObjectOp) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(ApplyObjectOp)
	if !ok {
		return false
	}

	if od.ApplyObjectOpEngineID() != d.ApplyObjectOpEngineID() {
		return false
	}
	if od.ApplyObjectOpObjectID() != d.ApplyObjectOpObjectID() {
		return false
	}
	if od.ApplyObjectOpOperationTypeID() != d.ApplyObjectOpOperationTypeID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *applyObjectOp) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *applyObjectOp) GetName() string {
	return "ApplyObjectOp"
}

// GetDebugString returns the directive arguments stringified.
func (d *applyObjectOp) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["operation-type-id"] = []string{d.ApplyObjectOpOperationTypeID()}
	vals["object-id"] = []string{d.ApplyObjectOpObjectID()}
	if engineID := d.ApplyObjectOpEngineID(); engineID != "" {
		vals["engine-id"] = []string{engineID}
	}
	return vals
}

// _ is a type assertion
var _ ApplyObjectOp = ((*applyObjectOp)(nil))
