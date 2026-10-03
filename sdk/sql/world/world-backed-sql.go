package s4wave_sql_world

import (
	"context"
	"database/sql/driver"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_mysql "github.com/s4wave/spacewave/db/sql/mysql"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// WorldBackedSql commits SQL roots through world operations.
type WorldBackedSql struct {
	// inner is the SQL database at the object root.
	inner *sql_mysql.Mysql
	// ws publishes committed roots.
	ws world.WorldState
	// key is the world object key.
	key string
	// release releases the database's cursor and the stage that owns its
	// writes.
	release func()
	// mtx guards tx.
	mtx sync.Mutex
	// writeMtx serializes write transactions.
	writeMtx sync.Mutex
	// tx is the open write transaction, if any.
	tx *worldBackedSqlTx
}

// NewWorldBackedSql opens a world-backed SQL database at the object's current
// root. The database stages its writes until it publishes the root that
// references them; Close releases the stage.
func NewWorldBackedSql(
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
) (*WorldBackedSql, error) {
	// Validate the World, storage cursor and SQL object key before opening the store.
	if ws == nil {
		return nil, objecttype.ErrWorldStateRequired
	}
	if objectKey == "" {
		return nil, world.ErrEmptyObjectKey
	}

	// Open a staged cursor at the object's current root.
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, err
	}
	rootRef, _, err := obj.GetRootRef(ctx)
	if err != nil {
		return nil, err
	}
	root, release, err := world.OpenStagedCursor(ctx, ws, rootRef)
	if err != nil {
		return nil, err
	}

	// Open the database on the staged root.
	st := &WorldBackedSql{
		ws:      ws,
		key:     objectKey,
		release: release,
	}
	st.inner = sql_mysql.NewMysql(root, st.captureCommittedRoot)
	return st, nil
}

// Close releases the database's cursor and stage.
func (s *WorldBackedSql) Close() {
	if s == nil || s.release == nil {
		return
	}
	s.release()
	s.release = nil
}

// NewSqlTransaction opens a SQL transaction.
func (s *WorldBackedSql) NewSqlTransaction(
	ctx context.Context,
	write bool,
	dsn string,
) (hydra_sql.SqlTransaction, error) {
	// Serialize SQL writers and capture the root they start from.
	if write {
		s.writeMtx.Lock()
	}
	var baseRoot *bucket.ObjectRef
	if write {
		baseRoot = s.inner.GetRootNodeRef()
	}

	// Open the SQL transaction and release the writer lock if opening fails.
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

	// Track the writable SQL transaction until it releases its writer lock.
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
	// Capture the committed SQL root on the active transaction.
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
	// Reload the SQL root from the World object.
	obj, err := world.MustGetObject(ctx, s.ws, s.key)
	defer world.ReleaseObjectState(obj)
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
	// Commit the SQL transaction and release its writer lock afterward.
	defer t.releaseWrite()
	t.committedRoot = nil
	if err := t.inner.Commit(ctx); err != nil {
		return err
	}
	root := t.committedRoot
	if root == nil {
		return &CommitPersistedError{Err: errors.New("sql/db: committed root was not captured")}
	}

	// Advance the World object root with the committed SQL statements.
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
	// Execute the SQL statement and record it for World root replay.
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
	// Query the SQL transaction and record the statement for World root replay.
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

var (
	_ hydra_sql.SqlStore       = (*WorldBackedSql)(nil)
	_ hydra_sql.SqlTransaction = (*worldBackedSqlTx)(nil)
	_ hydra_sql.SqlOps         = (*recordingSqlOps)(nil)
)
