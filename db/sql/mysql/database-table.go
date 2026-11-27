package mysql

import (
	"errors"

	"github.com/dolthub/go-mysql-server/sql"
)

// TableCount returns the count of tables in the database.
func (d *Database) TableCount() int {
	return len(d.root.GetTables())
}

// CreateTable creates the table with the given name and schema. If a table
// with that name already exists, returns sql.ErrTableAlreadyExists.
func (d *Database) CreateTable(ctx *sql.Context, name string, schema sql.PrimaryKeySchema, collation sql.CollationID, comment string) error {
	// Require an unused table name before inserting its database record.
	if _, _, nsbOk := d.nsbs.LookupByName(name); nsbOk {
		return sql.ErrTableAlreadyExists.New(name)
	}

	// Insert the named table record into the database root.
	ics, ok := d.root.InsertTable(name, nil, d.bcs)
	if !ok {
		return sql.ErrTableAlreadyExists.New(name)
	}

	// Build the table schema at the inserted table root reference.
	d.bcs.SetBlock(d.root, true)
	ics = ics.FollowRef(2, nil)
	_, _, err := BuildTable(ctx, ics, name, schema, collation, comment)
	return err
}

// DropTable deletes a table, if it exists.
func (d *Database) DropTable(ctx *sql.Context, name string) error {
	// Remove the named table record from the database root.
	if !d.nsbs.RemoveByName(name) {
		return sql.ErrTableNotFound.New(name)
	}

	// Invalidate the removed table and mark the database dirty.
	delete(d.tbls, name)
	d.MarkDirty()
	return nil
}

// RenameTable renames a table from oldName to newName as given. It returns
// sql.ErrTableAlreadyExists when a table named newName exists.
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
	_ sql.TableCreator = (*Database)(nil)
	_ sql.TableDropper = (*Database)(nil)
	_ sql.TableRenamer = (*Database)(nil)
)
