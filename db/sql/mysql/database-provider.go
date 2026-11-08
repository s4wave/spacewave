package mysql

import (
	"context"
	"fmt"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/s4wave/spacewave/db/tx"
)

// transactionContextKey selects storage for one command on a shared engine.
type transactionContextKey struct{}

// databaseProvider resolves the command's transaction or a fixed native catalog.
type databaseProvider struct {
	ctx context.Context
	tx  *Tx
}

// Database returns a database from this command's transaction.
func (p *databaseProvider) Database(ctx *sql.Context, name string) (sql.Database, error) {
	db, err := p.open(ctx, name, false)
	if err != nil {
		if ErrDatabaseNotFound.Is(err) {
			return nil, sql.ErrDatabaseNotFound.New(name)
		}
		return nil, err
	}
	return db, nil
}

// HasDatabase reports whether the selected transaction has the database.
func (p *databaseProvider) HasDatabase(ctx *sql.Context, name string) bool {
	_, err := p.open(ctx, name, false)
	return err == nil
}

// AllDatabases returns the databases in this transaction's catalog.
func (p *databaseProvider) AllDatabases(ctx *sql.Context) []sql.Database {
	// Resolve storage from this command, never a previous transaction.
	transaction, err := p.transaction(ctx)
	if err != nil {
		return nil
	}

	// Hold the transaction lock while reading its database catalog.
	transaction.rmtx.Lock()
	defer transaction.rmtx.Unlock()

	// Open each catalog database and collect the available handles.
	out := make([]sql.Database, 0, len(transaction.root.GetDatabases()))
	for _, rootDb := range transaction.root.GetDatabases() {
		db, err := transaction.openDatabaseLocked(p.context(ctx), rootDb.GetName(), false, !transaction.write)
		if err == nil && db != nil {
			out = append(out, db)
		}
	}
	return out
}

// CreateDatabase adds a database to the current writable transaction.
func (p *databaseProvider) CreateDatabase(ctx *sql.Context, name string) error {
	// Resolve storage from this command, never a previous transaction.
	transaction, err := p.transaction(ctx)
	if err != nil {
		return err
	}

	// Hold the transaction lock while creating the database.
	transaction.rmtx.Lock()
	defer transaction.rmtx.Unlock()

	// Require an unused database name and a writable transaction.
	dbs := transaction.root.GetRootDbSet(transaction.bcs)
	if _, _, ok := dbs.LookupByName(name); ok {
		return sql.ErrDatabaseExists.New(name)
	}
	if !transaction.write {
		return tx.ErrNotWrite
	}

	// Insert the database record and initialize its root block.
	_, rcs := transaction.root.InsertDatabase(name, nil, transaction.bcs)
	transaction.bcs.SetBlock(transaction.root, true)
	rcs = rcs.FollowRef(2, nil)
	rcs.SetBlock(NewDatabaseRootBlock(), true)

	// Open the created database and retain it in the transaction catalog.
	db, err := NewDatabase(p.context(ctx), name, false, rcs)
	if err != nil {
		return err
	}
	transaction.openDbs[name] = db
	return nil
}

// DropDatabase removes a database from the current writable transaction.
func (p *databaseProvider) DropDatabase(ctx *sql.Context, name string) error {
	// Resolve storage from this command, never a previous transaction.
	transaction, err := p.transaction(ctx)
	if err != nil {
		return err
	}

	// Hold the transaction lock while removing the database.
	transaction.rmtx.Lock()
	defer transaction.rmtx.Unlock()

	// Require a writable transaction before removing a database.
	if !transaction.write {
		return tx.ErrNotWrite
	}

	// Remove the named database from the stored catalog.
	dbs := transaction.root.GetRootDbSet(transaction.bcs)
	oldLen := len(transaction.root.GetDatabases())
	_, _, ok := dbs.DeleteByName(name)
	if !ok {
		return sql.ErrDatabaseNotFound.New(name)
	}

	// Clear the removed catalog reference and invalidate its cached database.
	if cursor := dbs.GetCursor(); cursor != nil && oldLen > len(transaction.root.GetDatabases()) {
		cursor.ClearRef(uint32(oldLen - 1)) //nolint:gosec
	}
	delete(transaction.openDbs, name)
	transaction.bcs.SetBlock(transaction.root, true)
	return nil
}

func (p *databaseProvider) open(ctx *sql.Context, name string, create bool) (*Database, error) {
	// Resolve storage from this command, never a previous transaction.
	transaction, err := p.transaction(ctx)
	if err != nil {
		return nil, err
	}

	// Hold the transaction catalog while opening the requested database.
	transaction.rmtx.Lock()
	defer transaction.rmtx.Unlock()
	return transaction.openDatabaseLocked(p.context(ctx), name, create, !transaction.write)
}

func (p *databaseProvider) context(ctx *sql.Context) context.Context {
	if ctx != nil && ctx.Context != nil {
		return ctx.Context
	}
	if p.ctx != nil {
		return p.ctx
	}
	return context.Background()
}

// transaction resolves the fixed catalog or the command-local storage handle.
func (p *databaseProvider) transaction(ctx *sql.Context) (*Tx, error) {
	if p.tx != nil {
		return p.tx, nil
	}
	if transaction, ok := ctx.Value(transactionContextKey{}).(*Tx); ok && transaction != nil {
		return transaction, nil
	}
	return nil, fmt.Errorf("SQL command has no storage transaction")
}

var _ sql.MutableDatabaseProvider = (*databaseProvider)(nil)
