package mysql

import (
	"context"

	"github.com/aperturerobotics/util/csync"
	sqle "github.com/dolthub/go-mysql-server"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// RootCursor is the minimal cursor surface Mysql needs to read and update the
// root object reference.
type RootCursor interface {
	// BuildTransaction opens a block transaction at the committed root.
	BuildTransaction(*block.PutOpts) (*block.Transaction, *block.Cursor)
	// GetRef returns the committed root object reference.
	GetRef() *bucket.ObjectRef
	// SetRootRef advances the committed root to ref.
	SetRootRef(*block.BlockRef)
}

// Mysql is the root of a mysql server data structure, containing named databases.
type Mysql struct {
	// engine keeps planning and variable state for this store across transactions.
	engine     *sqle.Engine
	rmtx       csync.RWMutex
	rootCursor RootCursor
	commitFn   CommitFn
}

// CommitFn is a function to call with the updated root before confirming it.
// Should be used to write the updated state back to storage.
// Note: engine rmtx is locked while cb is called, do not block or call engine funcs!
// If an error is returned the change will be rolled back.
// Do not change the nrootBcs during this call.
type CommitFn func(nref *bucket.ObjectRef) error

// NewMysql creates a handle with an optional root object cursor pointing to the
// tree. The cursor ref can be empty to indicate a new tree.
func NewMysql(rootCursor RootCursor, commitFn CommitFn) *Mysql {
	return &Mysql{rootCursor: rootCursor, commitFn: commitFn, engine: sqle.NewDefault(&databaseProvider{})}
}

// Variables returns the engine variable bundle shared by this store's sessions.
func (t *Mysql) Variables() *sql.EngineVariables { return t.engine.Variables() }

// Close releases engine resources after every store transaction has finished.
func (t *Mysql) Close() error { return t.engine.Close() }

// GetRootNodeRef returns the reference to the root node.
func (t *Mysql) GetRootNodeRef(ctx context.Context) (*bucket.ObjectRef, error) {
	// Read the root only while this caller retains the shared storage gate.
	release, err := t.rmtx.Lock(ctx, false)
	if err != nil {
		return nil, err
	}
	defer release()
	return t.rootCursor.GetRef().Clone(), nil
}

// SetRootNodeRef switches the root cursor to an externally advanced root.
func (t *Mysql) SetRootNodeRef(ctx context.Context, ref *bucket.ObjectRef) error {
	// Serialize root replacement with transactions without trapping canceled callers.
	release, err := t.rmtx.Lock(ctx, true)
	if err != nil {
		return err
	}
	defer release()
	t.rootCursor.SetRootRef(ref.GetRootRef().Clone())
	return nil
}

// NewMysqlTransaction returns a transaction against the db.
func (t *Mysql) NewMysqlTransaction(ctx context.Context, write bool) (*Tx, error) {
	// Hold the database root lock for the requested transaction mode.
	release, err := t.rmtx.Lock(ctx, write)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		release()
		return nil, err
	}

	// Load the root and construct the transaction with its database catalog.
	rn, btx, bcs, err := t.fetchRoot(ctx, write)
	atx := &Tx{
		t:       t,
		write:   write,
		root:    rn,
		tx:      btx,
		bcs:     bcs,
		openDbs: make(map[string]*Database),
		unlock:  release,
	}
	if err != nil {
		atx.Discard()
		return nil, err
	}
	return atx, nil
}

// fetchRoot fetches the root block. A read transaction's block transaction is
// read-only, so it shares decoded blocks with other readers instead of cloning
// them.
func (t *Mysql) fetchRoot(ctx context.Context, write bool) (
	rn *Root,
	btx *block.Transaction,
	bcs *block.Cursor,
	err error,
) {
	btx, bcs = t.rootCursor.BuildTransaction(nil)
	if !write {
		btx.SetReadOnly()
	}
	rn, err = block.UnmarshalBlock[Root](ctx, bcs)
	return
}
