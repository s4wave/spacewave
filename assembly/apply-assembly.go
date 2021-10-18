package assembly

import (
	"github.com/aperturerobotics/controllerbus/directive"
)

// ApplyAssembly is a directive to apply a Assembly.
// Value type: ApplyAssemblyValue
type ApplyAssembly interface {
	// Directive indicates ApplyAssembly is a directive.
	directive.Directive

	// GetApplyAssembly returns the Assembly to apply.
	GetApplyAssembly() Assembly
}

// ApplyAssemblyValue is the result type for ApplyAssembly.
type ApplyAssemblyValue = Reference

// applyAssembly implements ApplyAssembly
type applyAssembly struct {
	conf Assembly
}

// NewApplyAssembly constructs a new ApplyAssembly directive.
func NewApplyAssembly(conf Assembly) ApplyAssembly {
	return &applyAssembly{
		conf: conf,
	}
}

// GetApplyAssembly returns the configset to apply.
func (d *applyAssembly) GetApplyAssembly() Assembly {
	return d.conf
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *applyAssembly) Validate() error {
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *applyAssembly) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent checks if the other directive is equivalent.
func (d *applyAssembly) IsEquivalent(other directive.Directive) bool {
	return false
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *applyAssembly) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *applyAssembly) GetName() string {
	return "ApplyAssembly"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *applyAssembly) GetDebugVals() directive.DebugValues {
	return nil
}

// _ is a type assertion
var _ ApplyAssembly = ((*applyAssembly)(nil))
