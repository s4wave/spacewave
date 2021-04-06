package mysql

import (
	"sync"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/lookup"
)

// Mysql is the root of a mysql server data structure, containing named databases.
type Mysql struct {
	rmtx       sync.RWMutex
	rootCursor *bucket_lookup.Cursor
}

// NewMysql creates a handle with an optional root object cursor pointing to the
// tree. The cursor ref can be empty to indicate a new tree.
func NewMysql(rootCursor *bucket_lookup.Cursor) *Mysql {
	return &Mysql{rootCursor: rootCursor}
}

// GetRootNodeRef returns the reference to the root node.
func (t *Mysql) GetRootNodeRef() *bucket.ObjectRef {
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()
	return t.rootCursor.GetRef()
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
		bcs.SetBlock(rn)
	}
	return
}
