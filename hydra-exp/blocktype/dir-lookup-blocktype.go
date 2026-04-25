package blocktype

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupBlockType is a directive to look up a block type.
type LookupBlockType interface {
	// Directive indicates LookupBlockType is a directive.
	directive.Directive

	// LookupBlockTypeID returns the block type ID to lookup.
	LookupBlockTypeID() string
}

// LookupBlockTypeValue is the result type for LookupBlockType.
type LookupBlockTypeValue = BlockType

// ExLookupBlockType executes a lookup for a single block type on the bus.
//
// blockTypeID is the block type identifier to lookup.
// Returns the BlockType or an error if not found.
func ExLookupBlockType(
	ctx context.Context,
	b bus.Bus,
	blockTypeID string,
) (BlockType, directive.Reference, error) {
	av, _, avRef, err := bus.ExecOneOffTyped[LookupBlockTypeValue](ctx, b, NewLookupBlockType(blockTypeID), nil, nil)
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

// lookupBlockType implements LookupBlockType
type lookupBlockType struct {
	blockTypeID string
}

// NewLookupBlockType constructs a new LookupBlockType directive.
func NewLookupBlockType(blockTypeID string) LookupBlockType {
	return &lookupBlockType{
		blockTypeID: blockTypeID,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupBlockType) Validate() error {
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *lookupBlockType) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// LookupBlockTypeID returns the block type ID to lookup.
func (d *lookupBlockType) LookupBlockTypeID() string {
	return d.blockTypeID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupBlockType) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupBlockType)
	if !ok {
		return false
	}

	if d.LookupBlockTypeID() != od.LookupBlockTypeID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupBlockType) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupBlockType) GetName() string {
	return "LookupBlockType"
}

// GetDebugVals returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupBlockType) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if d.blockTypeID != "" {
		vals["block-type-id"] = []string{d.blockTypeID}
	}
	return vals
}

// _ is a type assertion
var _ LookupBlockType = ((*lookupBlockType)(nil))
