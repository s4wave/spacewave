package mysql

import (
	"context"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/s4wave/spacewave/db/tx"
)

type databaseProvider struct {
	ctx context.Context
	tx  *Tx
}

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

func (p *databaseProvider) HasDatabase(ctx *sql.Context, name string) bool {
	_, err := p.open(ctx, name, false)
	return err == nil
}

func (p *databaseProvider) AllDatabases(ctx *sql.Context) []sql.Database {
	p.tx.rmtx.Lock()
	defer p.tx.rmtx.Unlock()

	out := make([]sql.Database, 0, len(p.tx.root.GetDatabases()))
	for _, rootDb := range p.tx.root.GetDatabases() {
		db, err := p.tx.openDatabaseLocked(p.context(ctx), rootDb.GetName(), false, !p.tx.write)
		if err == nil && db != nil {
			out = append(out, db)
		}
	}
	return out
}

func (p *databaseProvider) CreateDatabase(ctx *sql.Context, name string) error {
	p.tx.rmtx.Lock()
	defer p.tx.rmtx.Unlock()

	dbs := p.tx.root.GetRootDbSet(p.tx.bcs)
	if _, _, ok := dbs.LookupByName(name); ok {
		return sql.ErrDatabaseExists.New(name)
	}
	if !p.tx.write {
		return tx.ErrNotWrite
	}
	_, rcs := p.tx.root.InsertDatabase(name, nil, p.tx.bcs)
	p.tx.bcs.SetBlock(p.tx.root, true)
	rcs = rcs.FollowRef(2, nil)
	rcs.SetBlock(NewDatabaseRootBlock(), true)
	db, err := NewDatabase(p.context(ctx), name, false, rcs)
	if err != nil {
		return err
	}
	p.tx.openDbs[name] = db
	return nil
}

func (p *databaseProvider) DropDatabase(ctx *sql.Context, name string) error {
	p.tx.rmtx.Lock()
	defer p.tx.rmtx.Unlock()

	if !p.tx.write {
		return tx.ErrNotWrite
	}
	dbs := p.tx.root.GetRootDbSet(p.tx.bcs)
	oldLen := len(p.tx.root.GetDatabases())
	_, _, ok := dbs.DeleteByName(name)
	if !ok {
		return sql.ErrDatabaseNotFound.New(name)
	}
	if cursor := dbs.GetCursor(); cursor != nil && oldLen > len(p.tx.root.GetDatabases()) {
		cursor.ClearRef(uint32(oldLen - 1)) //nolint:gosec
	}
	delete(p.tx.openDbs, name)
	p.tx.bcs.SetBlock(p.tx.root, true)
	return nil
}

func (p *databaseProvider) open(ctx *sql.Context, name string, create bool) (*Database, error) {
	p.tx.rmtx.Lock()
	defer p.tx.rmtx.Unlock()
	return p.tx.openDatabaseLocked(p.context(ctx), name, create, !p.tx.write)
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

var _ sql.MutableDatabaseProvider = ((*databaseProvider)(nil))
