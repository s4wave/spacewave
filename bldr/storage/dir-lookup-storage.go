package storage

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupStorage is a directive to lookup a volume factory (Storage).
type LookupStorage interface {
	// Directive indicates LookupStorage is a directive.
	directive.Directive

	// LookupStorageId is the storage id to lookup.
	// If empty, returns all storage on the bus.
	LookupStorageId() string
}

// LookupStorageValue is the result type for LookupStorage.
// Multiple results may be pushed to the directive.
type LookupStorageValue = Storage

// lookupStorage implements LookupStorage
type lookupStorage struct {
	storageId string
}

// NewLookupStorage constructs a new LookupStorage directive.
func NewLookupStorage(storageId string) LookupStorage {
	return &lookupStorage{storageId: storageId}
}

// ExLookupStorage executes the LookupStorage directive.
// If waitOne is set, waits for at least one value before returning.
func ExLookupStorage(
	ctx context.Context,
	b bus.Bus,
	storageId string,
	waitOne bool,
) ([]LookupStorageValue, directive.Instance, directive.Reference, error) {
	return bus.ExecCollectValues[LookupStorageValue](
		ctx,
		b,
		NewLookupStorage(storageId),
		waitOne,
		nil,
	)
}

// Validate validates the directive.
func (d *lookupStorage) Validate() error {
	return nil
}

// GetValueLookupStorageOptions returns options relating to value handling.
func (d *lookupStorage) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// LookupStorageId returns the block store id.
func (d *lookupStorage) LookupStorageId() string {
	return d.storageId
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupStorage) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupStorage)
	if !ok {
		return false
	}

	if d.LookupStorageId() != od.LookupStorageId() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupStorage) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupStorage) GetName() string {
	return "LookupStorage"
}

// GetDebugString returns the directive arguments stringified.
func (d *lookupStorage) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if id := d.LookupStorageId(); id != "" {
		vals["storage-id"] = []string{d.LookupStorageId()}
	}
	return vals
}

// _ is a type assertion
var _ LookupStorage = ((*lookupStorage)(nil))
