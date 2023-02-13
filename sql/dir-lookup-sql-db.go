package sql

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupSqlDB is a directive to lookup a SQL Database.
type LookupSqlDB interface {
	// Directive indicates LookupSqlDB is a directive.
	directive.Directive

	// LookupSqlDBId returns the sql db id to lookup.
	LookupSqlDBId() string
}

// LookupSqlDBValue is the result type for LookupSqlDB.
// Multiple results may be pushed to the directive.
type LookupSqlDBValue = SqlDB

// lookupSqlDB implements LookupSqlDB
type lookupSqlDB struct {
	dbID string
}

// NewLookupSqlDB constructs a new LookupSqlDB directive.
func NewLookupSqlDB(dbID string) LookupSqlDB {
	return &lookupSqlDB{dbID: dbID}
}

// ExLookupSqlDBs executes the LookupSqlDB directive.
// If waitOne is set, waits for at least one value before returning.
func ExLookupSqlDBs(
	ctx context.Context,
	b bus.Bus,
	dbID string,
	waitOne bool,
) ([]LookupSqlDBValue, directive.Instance, directive.Reference, error) {
	return bus.ExecCollectValues[LookupSqlDBValue](
		ctx,
		b,
		NewLookupSqlDB(dbID),
		waitOne,
		nil,
	)
}

// ExLookupFirstSQLDb waits for the first HTTP handler to be returned.
// if returnIfIdle is set and the directive becomes idle, returns nil, nil, nil,
func ExLookupFirstSQLDb(
	ctx context.Context,
	b bus.Bus,
	dbID string,
	returnIfIdle bool,
	valDisposeCb func(),
) (LookupSqlDBValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[LookupSqlDBValue](
		ctx,
		b,
		NewLookupSqlDB(dbID),
		returnIfIdle,
		valDisposeCb,
		nil,
	)
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupSqlDB) Validate() error {
	if d.dbID == "" {
		return ErrEmptySqlDbId
	}
	return nil
}

// GetValueLookupSqlDBOptions returns options relating to value handling.
func (d *lookupSqlDB) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		UnrefDisposeDur: time.Second,
	}
}

// LookupSqlDBId returns the sql db id to lookup.
func (d *lookupSqlDB) LookupSqlDBId() string {
	return d.dbID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupSqlDB) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupSqlDB)
	if !ok {
		return false
	}

	if d.LookupSqlDBId() != od.LookupSqlDBId() {
		return false
	}
	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupSqlDB) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupSqlDB) GetName() string {
	return "LookupSqlDB"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupSqlDB) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["sql-db-id"] = []string{d.LookupSqlDBId()}
	return vals
}

// _ is a type assertion
var _ LookupSqlDB = ((*lookupSqlDB)(nil))
