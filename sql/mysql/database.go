package mysql

import (
	"strings"

	"github.com/aperturerobotics/hydra/block"
	namedsbset "github.com/aperturerobotics/hydra/block/sbset"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/pkg/errors"
)

// Database is the block-graph backed SQL db cursor.
// NOTE: calls are not concurrency-safe.
type Database struct {
	name     string
	readOnly bool
	bcs      *block.Cursor
	root     *DatabaseRoot
	nsbs     *namedsbset.NamedSubBlockSet

	// tbls contains table instances in memory
	tbls map[string]*Table
}

// NewDatabase constructs a new database handle.
func NewDatabase(name string, readOnly bool, bcs *block.Cursor) (*Database, error) {
	dbr, err := block.UnmarshalBlock[*DatabaseRoot](bcs, NewDatabaseRootBlock)
	if err != nil {
		return nil, err
	}
	return &Database{
		name:     name,
		readOnly: readOnly,
		bcs:      bcs,
		root:     dbr,
		nsbs:     dbr.GetRootTableSet(bcs),
		tbls:     make(map[string]*Table),
	}, nil
}

// Name returns the name.
func (d *Database) Name() string {
	return d.name
}

// IsReadOnly returns whether this database is read-only.
func (d *Database) IsReadOnly() bool {
	return d.readOnly
}

// GetTableInsensitive retrieves a table by its case insensitive name.
// Implementations should look for exact (case-sensitive matches) first. If no
// exact matches are found then any table matching the name case insensitively
// should be returned. If there is more than one table that matches a case
// insensitive comparison the resolution strategy is not defined.
func (d *Database) GetTableInsensitive(ctx *sql.Context, tblName string) (sql.Table, bool, error) {
	cctx := GetDbContext(ctx)
	set := d.root.GetRootTableSet(d.bcs)
	// search exact match
	tbl, ok := d.tbls[tblName]
	if ok {
		return tbl, tbl != nil, nil
	}
	nsb, bcs, found := set.LookupByName(tblName)
	if !found {
		// search case insensitive
		nsb, bcs, found = set.LookupByNameCaseInsensitive(tblName)
		if !found {
			return nil, false, nil
		}
	}

	tble, ok := nsb.(*DatabaseRootTable)
	if !ok {
		return nil, false, ErrUnexpectedType
	}

	tbln := tble.GetName()
	if !strings.EqualFold(tbln, tblName) {
		return nil, false, errors.Errorf("unexpected table name: %s", tbln)
	}
	ttbl, err := LoadTable(cctx, tbln, bcs.FollowRef(2, tble.GetRef()))
	if err != nil {
		return nil, false, err
	}
	d.tbls[tbln] = ttbl
	return ttbl, true, nil
}

// GetTableNames returns the table names of every table in the database
func (d *Database) GetTableNames(ctx *sql.Context) ([]string, error) {
	tables := d.root.GetTables()
	names := make([]string, len(tables))
	for i, t := range tables {
		names[i] = t.GetName()
	}
	return names, nil
}

// _ is a type assertion
var (
	_ sql.Database         = ((*Database)(nil))
	_ sql.ReadOnlyDatabase = ((*Database)(nil))

	// _ sql.StoredProcedureDatabase = ((*Database)(nil))
	// _ sql.TableCopierDatabase    = ((*Database)(nil))
	// _ sql.TemporaryTableDatabase = ((*Database)(nil))
	// _ sql.TransactionDatabase    = ((*Database)(nil))
	// _ sql.TriggerDatabase        = ((*Database)(nil))
	// _ sql.VersionedDatabase      = ((*Database)(nil))
	// _ sql.ViewDatabase = ((*Database)(nil))
)
