package s4wave_sql_world

import (
	"context"
	"database/sql/driver"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_mysql "github.com/s4wave/spacewave/db/sql/mysql"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

type worldCursor = *bucket_lookup.Cursor

// WorldBackedSql commits SQL roots through world operations.
type WorldBackedSql struct {
	inner    *sql_mysql.Mysql
	ws       world.WorldState
	key      string
	root     worldCursor
	mtx      sync.Mutex
	writeMtx sync.Mutex
	tx       *worldBackedSqlTx
}

// NewWorldBackedSql opens a world-backed SQL database.
func NewWorldBackedSql(
	_ context.Context,
	root worldCursor,
	ws world.WorldState,
	objectKey string,
) (*WorldBackedSql, error) {
	if ws == nil {
		return nil, objecttype.ErrWorldStateRequired
	}
	if root == nil {
		return nil, errors.New("sql/db: root cursor is required")
	}
	if objectKey == "" {
		return nil, world.ErrEmptyObjectKey
	}
	st := &WorldBackedSql{
		ws:   ws,
		key:  objectKey,
		root: root,
	}
	st.inner = sql_mysql.NewMysql(root, st.captureCommittedRoot)
	return st, nil
}

// Close releases the backing world cursor.
func (s *WorldBackedSql) Close() {
	if s == nil || s.root == nil {
		return
	}
	s.root.Release()
	s.root = nil
}

// NewSqlTransaction opens a SQL transaction.
func (s *WorldBackedSql) NewSqlTransaction(
	ctx context.Context,
	write bool,
	dsn string,
) (hydra_sql.SqlTransaction, error) {
	if write {
		s.writeMtx.Lock()
	}
	var baseRoot *bucket.ObjectRef
	if write {
		baseRoot = s.inner.GetRootNodeRef()
	}
	tx, err := s.inner.NewSqlTransaction(ctx, write, dsn)
	if err != nil {
		if write {
			s.writeMtx.Unlock()
		}
		return nil, err
	}
	if !write {
		return tx, nil
	}
	wtx := &worldBackedSqlTx{
		store:    s,
		inner:    tx,
		dsn:      dsn,
		baseRoot: baseRoot,
	}
	s.mtx.Lock()
	s.tx = wtx
	s.mtx.Unlock()
	return wtx, nil
}

func (s *WorldBackedSql) captureCommittedRoot(root *bucket.ObjectRef) error {
	if root == nil || root.GetEmpty() {
		return errors.New("sql/db: committed root is empty")
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.tx == nil {
		return errors.New("sql/db: committed root captured without active transaction")
	}
	s.tx.committedRoot = root.Clone()
	return nil
}

func (s *WorldBackedSql) clearActiveTx(tx *worldBackedSqlTx) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.tx == tx {
		s.tx = nil
	}
}

func (s *WorldBackedSql) refreshInnerRoot(ctx context.Context) error {
	obj, err := world.MustGetObject(ctx, s.ws, s.key)
	if err != nil {
		return err
	}
	root, _, err := obj.GetRootRef(ctx)
	if err != nil {
		return err
	}
	s.inner.SetRootNodeRef(root)
	return nil
}

type worldBackedSqlTx struct {
	store         *WorldBackedSql
	inner         hydra_sql.SqlTransaction
	dsn           string
	baseRoot      *bucket.ObjectRef
	committedRoot *bucket.ObjectRef
	statements    []*SqlStatement
	releaseOnce   sync.Once
}

// Commit commits the SQL transaction and advances the world object root.
func (t *worldBackedSqlTx) Commit(ctx context.Context) error {
	defer t.releaseWrite()
	t.committedRoot = nil
	if err := t.inner.Commit(ctx); err != nil {
		return err
	}
	root := t.committedRoot
	if root == nil {
		return &CommitPersistedError{Err: errors.New("sql/db: committed root was not captured")}
	}
	_, _, err := t.store.ws.ApplyWorldOp(ctx, NewSqlSetRootOp(t.store.key, t.baseRoot, root, t.statements), peer.ID(""))
	if err != nil {
		return &CommitPersistedError{Err: err}
	}
	if err := t.store.refreshInnerRoot(ctx); err != nil {
		return &CommitPersistedError{Err: err}
	}
	return nil
}

// Discard discards the SQL transaction.
func (t *worldBackedSqlTx) Discard() {
	t.releaseWrite()
	t.inner.Discard()
}

func (t *worldBackedSqlTx) releaseWrite() {
	t.releaseOnce.Do(func() {
		t.store.clearActiveTx(t)
		t.store.writeMtx.Unlock()
	})
}

// GetReadOnly returns if the transaction is read-only.
func (t *worldBackedSqlTx) GetReadOnly() bool {
	return t.inner.GetReadOnly()
}

// GetSqlOps returns the SQL operations interface.
func (t *worldBackedSqlTx) GetSqlOps(ctx context.Context) (hydra_sql.SqlOps, error) {
	ops, err := t.inner.GetSqlOps(ctx)
	if err != nil || t.inner.GetReadOnly() {
		return ops, err
	}
	return &recordingSqlOps{tx: t, inner: ops}, nil
}

type recordingSqlOps struct {
	tx    *worldBackedSqlTx
	inner hydra_sql.SqlOps
}

func (o *recordingSqlOps) Exec(query string, args []driver.Value) (driver.Result, error) {
	return o.ExecContext(context.Background(), query, hydra_sql.ConvertToNamedValues(args))
}

func (o *recordingSqlOps) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	statement, err := buildSqlStatement(SqlStatementKind_SQL_STATEMENT_KIND_EXEC, o.tx.dsn, query, args)
	if err != nil {
		return nil, err
	}
	res, err := o.inner.ExecContext(ctx, query, args)
	if err != nil {
		return nil, err
	}
	o.tx.statements = append(o.tx.statements, statement)
	return res, nil
}

func (o *recordingSqlOps) Query(query string, args []driver.Value) (driver.Rows, error) {
	return o.QueryContext(context.Background(), query, hydra_sql.ConvertToNamedValues(args))
}

func (o *recordingSqlOps) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	statement, err := buildSqlStatement(SqlStatementKind_SQL_STATEMENT_KIND_QUERY, o.tx.dsn, query, args)
	if err != nil {
		return nil, err
	}
	rows, err := o.inner.QueryContext(ctx, query, args)
	if err != nil {
		return nil, err
	}
	o.tx.statements = append(o.tx.statements, statement)
	return rows, nil
}

// _ are type assertions
var (
	_ hydra_sql.SqlStore       = ((*WorldBackedSql)(nil))
	_ hydra_sql.SqlTransaction = ((*worldBackedSqlTx)(nil))
	_ hydra_sql.SqlOps         = ((*recordingSqlOps)(nil))
)
