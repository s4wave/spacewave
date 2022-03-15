package mysql

import (
	"context"
	"sync"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/dolthub/go-mysql-server/sql"
)

// Tx contains a transaction against the mysql data store.
type Tx struct {
	commitOnce sync.Once
	t          *Mysql
	write      bool

	root    *Root
	tx      *block.Transaction
	bcs     *block.Cursor
	rmtx    sync.RWMutex
	openDbs map[string]*Database
}

// GetBlockTransaction returns the underlying block transaction.
func (t *Tx) GetBlockTransaction() *block.Transaction {
	return t.tx
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (t *Tx) Commit(ctx context.Context) (cerr error) {
	t.commitOnce.Do(func() {
		if t.write {
			res, _, err := t.tx.Write(true)
			if err != nil {
				cerr = err
			} else {
				nc := *t.t.rootCursor
				nc.SetRootRef(res)
				t.t.rootCursor = &nc
			}
			t.t.rmtx.Unlock()
		} else {
			t.t.rmtx.RUnlock()
		}
	})
	return
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	t.commitOnce.Do(func() {
		if t.write {
			t.t.rmtx.Unlock()
		} else {
			t.t.rmtx.RUnlock()
		}
	})
}

// DatabaseCount returns the number of databases in the tree.
func (t *Tx) DatabaseCount() int {
	return len(t.root.GetDatabases())
}

// OpenDatabase opens a database with the given name.
//
// If not exist, create is set, and tx is a write tx, it will be created.
func (t *Tx) OpenDatabase(name string, create bool) (*Database, error) {
	if name == "" {
		return nil, ErrEmptyDatabaseName
	}
	t.rmtx.Lock()
	defer t.rmtx.Unlock()
	readOnly := !t.write
	return t.openDatabaseLocked(name, create, readOnly)
}

// BuildDatabaseProvider builds the database catalog from the available dbs.
func (t *Tx) BuildDatabaseProvider() (sql.DatabaseProvider, error) {
	t.rmtx.Lock()
	defer t.rmtx.Unlock()

	rootDbs := t.root.GetDatabases()
	dbs := make([]sql.Database, len(rootDbs))
	for i, v := range rootDbs {
		db, err := t.openDatabaseLocked(v.GetName(), false, !t.write)
		if err != nil {
			if ErrDatabaseNotFound.Is(err) {
				continue
			}
			return nil, err
		}
		dbs[i] = db
	}
	// dbs = append(dbs, information_schema.NewInformationSchemaDatabase(cl))

	// TODO: return a MutableDatabaseProvider
	return sql.NewDatabaseProvider(dbs...), nil
}

// openDatabaseLocked implements OpenDatabase when rmtx is locked by caller.
func (t *Tx) openDatabaseLocked(name string, create, readOnly bool) (*Database, error) {
	if d, ok := t.openDbs[name]; ok {
		// note: d may be nil here.
		return d, nil
	}
	dbs := t.root.GetRootDbSet(t.bcs)
	nsb, rcs, ok := dbs.LookupByName(name)
	var dsb *RootDb
	if !ok {
		if !create {
			return nil, ErrDatabaseNotFound.New(name)
		}
		if !t.write {
			return nil, tx.ErrNotWrite
		}
		_, rcs = t.root.InsertDatabase(name, nil, t.bcs)
		rcs = rcs.FollowRef(2, nil)                // follow ref field
		rcs.SetBlock(NewDatabaseRootBlock(), true) // init empty db root
	} else {
		dsb, ok = nsb.(*RootDb)
		if !ok {
			return nil, ErrUnexpectedType
		}
		rcs = rcs.FollowRef(2, dsb.GetRef())
	}
	ndb, err := NewDatabase(name, readOnly, rcs)
	if err != nil {
		return nil, err
	}
	t.openDbs[name] = ndb
	return ndb, nil
}
