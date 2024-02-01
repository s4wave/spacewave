package block_store

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupBlockStore is a directive to lookup a block store.
type LookupBlockStore interface {
	// Directive indicates LookupBlockStore is a directive.
	directive.Directive

	// LookupBlockStoreId is the block store id to lookup.
	// Cannot be empty.
	LookupBlockStoreId() string
}

// LookupBlockStoreValue is the result type for LookupBlockStore.
// Multiple results may be pushed to the directive.
type LookupBlockStoreValue = Store

// lookupBlockStore implements LookupBlockStore
type lookupBlockStore struct {
	blockStoreId string
}

// NewLookupBlockStore constructs a new LookupBlockStore directive.
func NewLookupBlockStore(blockStoreId string) LookupBlockStore {
	return &lookupBlockStore{blockStoreId: blockStoreId}
}

// ExLookupBlockStores executes the LookupBlockStore directive.
// If waitOne is set, waits for at least one value before returning.
func ExLookupBlockStores(
	ctx context.Context,
	b bus.Bus,
	blockStoreId string,
	waitOne bool,
) ([]LookupBlockStoreValue, directive.Instance, directive.Reference, error) {
	return bus.ExecCollectValues[LookupBlockStoreValue](
		ctx,
		b,
		NewLookupBlockStore(blockStoreId),
		waitOne,
		nil,
	)
}

// ExLookupFirstBlockStore waits for the first block store to be returned.
// if returnIfIdle is set and the directive becomes idle, returns nil, nil, nil,
func ExLookupFirstBlockStore(
	ctx context.Context,
	b bus.Bus,
	blockStoreId string,
	returnIfIdle bool,
	valDisposeCb func(),
) (LookupBlockStoreValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[LookupBlockStoreValue](
		ctx,
		b,
		NewLookupBlockStore(blockStoreId),
		bus.ReturnIfIdle(returnIfIdle),
		valDisposeCb,
		nil,
	)
}

// Validate validates the directive.
func (d *lookupBlockStore) Validate() error {
	if d.blockStoreId == "" {
		return ErrBlockStoreIDEmpty
	}
	return nil
}

// GetValueLookupBlockStoreOptions returns options relating to value handling.
func (d *lookupBlockStore) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		UnrefDisposeDur:            time.Millisecond * 500,
		UnrefDisposeEmptyImmediate: true,
	}
}

// LookupBlockStoreId returns the block store id.
func (d *lookupBlockStore) LookupBlockStoreId() string {
	return d.blockStoreId
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupBlockStore) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupBlockStore)
	if !ok {
		return false
	}

	if d.LookupBlockStoreId() != od.LookupBlockStoreId() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupBlockStore) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupBlockStore) GetName() string {
	return "LookupBlockStore"
}

// GetDebugString returns the directive arguments stringified.
func (d *lookupBlockStore) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["block-store-id"] = []string{d.LookupBlockStoreId()}
	return vals
}

// _ is a type assertion
var _ LookupBlockStore = ((*lookupBlockStore)(nil))
