package world

import (
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupWorldEngine is a directive to lookup a running World Graph engine.
// Value type: world.EngineHandle.
type LookupWorldEngine interface {
	// Directive indicates LookupWorldEngine is a directive.
	directive.Directive

	// LookupWorldEngineID returns a specific world engine we are looking for.
	// Can be empty.
	LookupWorldEngineID() string
}

// LookupWorldEngineValue is the value type for LookupWorldEngine.
type LookupWorldEngineValue = EngineHandle

// lookupWorldEngine implements LookupWorldEngine
type lookupWorldEngine struct {
	id string
}

// NewLookupWorldEngine constructs a new LookupWorldEngine directive.
func NewLookupWorldEngine(id string) LookupWorldEngine {
	return &lookupWorldEngine{
		id: id,
	}
}

// LookupWorldEngineID returns a specific ID we are looking for.
// If empty, any engine is matched.
func (d *lookupWorldEngine) LookupWorldEngineID() string {
	return d.id
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupWorldEngine) Validate() error {
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *lookupWorldEngine) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupWorldEngine) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupWorldEngine)
	if !ok {
		return false
	}

	return d.LookupWorldEngineID() == od.LookupWorldEngineID()
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupWorldEngine) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupWorldEngine) GetName() string {
	return "LookupWorldEngine"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupWorldEngine) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if id := d.LookupWorldEngineID(); len(id) != 0 {
		vals["engine-id"] = []string{id}
	}
	return vals
}

// _ is a type assertion
var _ LookupWorldEngine = ((*lookupWorldEngine)(nil))
