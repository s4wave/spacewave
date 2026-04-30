package mysql

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// RootCursor is the minimal cursor surface Mysql needs to read and update the
// root object reference.
type RootCursor interface {
	BuildTransaction(*block.PutOpts) (*block.Transaction, *block.Cursor)
	GetRef() *bucket.ObjectRef
	SetRootRef(*block.BlockRef)
}

// Mysql is the root of a mysql server data structure, containing named databases.
type Mysql struct {
	rmtx       sync.RWMutex
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
	return &Mysql{rootCursor: rootCursor, commitFn: commitFn}
}

// GetRootNodeRef returns the reference to the root node.
func (t *Mysql) GetRootNodeRef() *bucket.ObjectRef {
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()
	return t.rootCursor.GetRef().Clone()
}

// NewMysqlTransaction returns a transaction against the db.
func (t *Mysql) NewMysqlTransaction(ctx context.Context, write bool) (*Tx, error) {
	if write {
		t.rmtx.Lock()
	} else {
		t.rmtx.RLock()
	}

	rn, btx, bcs, err := t.fetchRoot(ctx)
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
func (t *Mysql) fetchRoot(ctx context.Context) (
	rn *Root,
	btx *block.Transaction,
	bcs *block.Cursor,
	err error,
) {
	btx, bcs = t.rootCursor.BuildTransaction(nil)
	rn, err = block.UnmarshalBlock[*Root](ctx, bcs, NewRootBlock)
	return
}
