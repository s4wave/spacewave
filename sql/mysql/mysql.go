package mysql

import (
	"context"
	"sync"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/sql"
)

// Mysql is the root of a mysql server data structure, containing named databases.
type Mysql struct {
	rmtx       sync.RWMutex
	rootCursor *bucket_lookup.Cursor
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
func NewMysql(rootCursor *bucket_lookup.Cursor, commitFn CommitFn) *Mysql {
	return &Mysql{rootCursor: rootCursor, commitFn: commitFn}
}

// GetRootNodeRef returns the reference to the root node.
func (t *Mysql) GetRootNodeRef() *bucket.ObjectRef {
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()
	return t.rootCursor.GetRef().Clone()
}

// NewTransaction returns a new SqlDB transaction.
func (t *Mysql) NewSqlTransaction(ctx context.Context, write bool, dsn string) (sql.SqlTransaction, error) {
	mtx, err := t.NewMysqlTransaction(write)
	if err != nil {
		return nil, err
	}
	stx, err := NewSqlTx(ctx, mtx, dsn)
	if err != nil {
		mtx.Discard()
		return nil, err
	}
	return stx, nil
}

// NewMysqlTransaction returns a transaction against the db.
func (t *Mysql) NewMysqlTransaction(write bool) (*Tx, error) {
	if write {
		t.rmtx.Lock()
	} else {
		t.rmtx.RLock()
	}

	rn, btx, bcs, err := t.fetchRoot()
	atx := &Tx{
		t:       t,
		write:   write,
		root:    rn,
		tx:      btx,
		bcs:     bcs,
		openDbs: make(map[string]*Database),
	}
	if err != nil {
		atx.Discard()
		return nil, err
	}
	return atx, nil
}

// fetchRoot fetches the root block.
func (t *Mysql) fetchRoot() (
	rn *Root,
	btx *block.Transaction,
	bcs *block.Cursor,
	err error,
) {
	btx, bcs = t.rootCursor.BuildTransaction(nil)
	if !t.rootCursor.GetRef().GetRootRef().GetEmpty() {
		bi, biErr := bcs.Unmarshal(NewRootBlock)
		if biErr != nil {
			return nil, nil, nil, biErr
		}
		rn, _ = bi.(*Root)
	} else {
		rn = &Root{}
		bcs.SetBlock(rn, true)
	}
	return
}

// _ is a type assertion
var _ sql.SqlStore = ((*Mysql)(nil))
