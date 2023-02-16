package mysql

import (
	"errors"

	"github.com/dolthub/go-mysql-server/sql"
)

// TableCount returns the count of tables in the database.
func (d *Database) TableCount() int {
	return len(d.root.GetTables())
}

// Creates the table with the given name and schema. If a table with that name
// already exists, must return sql.ErrTableAlreadyExists.
func (d *Database) CreateTable(ctx *sql.Context, name string, schema sql.PrimaryKeySchema, collation sql.CollationID) error {
	if _, _, nsbOk := d.nsbs.LookupByName(name); nsbOk {
		return sql.ErrTableAlreadyExists.New(name)
	}
	ics, ok := d.root.InsertTable(name, nil, d.bcs)
	if !ok {
		return sql.ErrTableAlreadyExists.New(name)
	}
	ics = ics.FollowRef(2, nil)
	_, _, err := BuildTable(ctx, ics, name, schema, 1, collation)
	return err
}

// DropTable deletes a table, if it exists.
func (d *Database) DropTable(ctx *sql.Context, name string) error {
	_, _, ok := d.nsbs.DeleteByName(name)
	if !ok {
		return sql.ErrTableNotFound.New(name)
	}
	return nil
}

// Renames a table from oldName to newName as given. If a table with newName
// already exists, must return sql.ErrTableAlreadyExists.
func (d *Database) RenameTable(ctx *sql.Context, oldName, newName string) error {
	_, _, ok := d.nsbs.LookupByName(newName)
	if ok {
		return sql.ErrTableAlreadyExists.New(newName)
	}
	// TODO
	return errors.New("TODO rename table")
}

// _ is a type assertion
var (
	_ sql.TableCreator = ((*Database)(nil))
	_ sql.TableDropper = ((*Database)(nil))
	_ sql.TableRenamer = ((*Database)(nil))
)
