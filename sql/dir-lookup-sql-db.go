package sql

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupSqlStore is a directive to lookup a SQL store.
type LookupSqlStore interface {
	// Directive indicates LookupSqlStore is a directive.
	directive.Directive

	// LookupSqlStoreId returns the sql db id to lookup.
	LookupSqlStoreId() string
}

// LookupSqlStoreValue is the result type for LookupSqlStore.
// Multiple results may be pushed to the directive.
type LookupSqlStoreValue = SqlStore

// lookupSqlStore implements LookupSqlStore
type lookupSqlStore struct {
	dbID string
}

// NewLookupSqlStore constructs a new LookupSqlStore directive.
func NewLookupSqlStore(dbID string) LookupSqlStore {
	return &lookupSqlStore{dbID: dbID}
}

// ExLookupSqlStore waits for the sql db to be resolved.
// if returnIfIdle is set and the directive becomes idle, returns nil, nil, nil,
func ExLookupSqlStore(
	ctx context.Context,
	b bus.Bus,
	dbID string,
	returnIfIdle bool,
	valDisposeCb func(),
) (LookupSqlStoreValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[LookupSqlStoreValue](
		ctx,
		b,
		NewLookupSqlStore(dbID),
		bus.ReturnIfIdle(returnIfIdle),
		valDisposeCb,
		nil,
	)
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupSqlStore) Validate() error {
	if d.dbID == "" {
		return ErrEmptySqlDbId
	}
	return nil
}

// GetValueLookupSqlStoreOptions returns options relating to value handling.
func (d *lookupSqlStore) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		UnrefDisposeDur: time.Second,
	}
}

// LookupSqlStoreId returns the sql db id to lookup.
func (d *lookupSqlStore) LookupSqlStoreId() string {
	return d.dbID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupSqlStore) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupSqlStore)
	if !ok {
		return false
	}

	if d.LookupSqlStoreId() != od.LookupSqlStoreId() {
		return false
	}
	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupSqlStore) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupSqlStore) GetName() string {
	return "LookupSqlStore"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupSqlStore) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["sql-db-id"] = []string{d.LookupSqlStoreId()}
	return vals
}

// _ is a type assertion
var _ LookupSqlStore = ((*lookupSqlStore)(nil))
