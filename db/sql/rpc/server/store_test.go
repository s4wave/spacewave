package sql_rpc_server

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	sql_rpc_client "github.com/s4wave/spacewave/db/sql/rpc/client"
	"github.com/s4wave/spacewave/db/tx"
)

func TestTxHandleCloseOpsWaitsForActiveStreams(t *testing.T) {
	h := &txHandle{
		active: make(map[uint64]func()),
		idle:   make(chan struct{}),
	}

	released := make(chan struct{})
	_, release, err := h.acquire(func() {
		close(released)
	})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	closed := make(chan struct{})
	go func() {
		h.closeOps()
		close(closed)
	}()

	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("closeOps did not release active stream")
	}

	select {
	case <-closed:
		t.Fatal("closeOps returned before active stream released")
	default:
	}

	release()

	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("closeOps did not return after active stream released")
	}

	_, _, err = h.acquire(nil)
	if !errors.Is(err, tx.ErrDiscarded) {
		t.Fatalf("acquire after close error = %v, want %v", err, tx.ErrDiscarded)
	}
}

func TestStoreClientConcurrentTransactions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fakeStore := &fakeSqlStore{}
	mux := srpc.NewMux()
	if err := sql_rpc.SRPCRegisterSql(mux, NewStore(fakeStore)); err != nil {
		t.Fatalf("register sql rpc: %v", err)
	}

	client := sql_rpc.NewSRPCSqlClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
	store := sql_rpc_client.NewStore(client)

	writeTx, err := store.NewSqlTransaction(ctx, true, "primary")
	if err != nil {
		t.Fatalf("new write tx: %v", err)
	}
	readTx, err := store.NewSqlTransaction(ctx, false, "readonly")
	if err != nil {
		t.Fatalf("new read tx: %v", err)
	}
	if writeTx.GetReadOnly() {
		t.Fatal("write tx reports read-only")
	}
	if !readTx.GetReadOnly() {
		t.Fatal("read tx reports writable")
	}

	writeOps, err := writeTx.GetSqlOps(ctx)
	if err != nil {
		t.Fatalf("write ops: %v", err)
	}
	result, err := writeOps.ExecContext(ctx, "insert into t(v) values (?)", []driver.NamedValue{
		{Ordinal: 1, Value: []byte{1, 2, 3}},
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("rows affected = %d, want 1", rowsAffected)
	}

	readOps, err := readTx.GetSqlOps(ctx)
	if err != nil {
		t.Fatalf("read ops: %v", err)
	}
	rows, err := readOps.QueryContext(ctx, "select id, name, payload from t", nil)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if cols := rows.Columns(); len(cols) != 3 || cols[0] != "id" || cols[1] != "name" || cols[2] != "payload" {
		t.Fatalf("columns = %v, want [id name payload]", cols)
	}
	columnTypes := rows.(driver.RowsColumnTypeDatabaseTypeName)
	if got := columnTypes.ColumnTypeDatabaseTypeName(2); got != "BLOB" {
		t.Fatalf("column type = %q, want BLOB", got)
	}
	dest := make([]driver.Value, 3)
	if err := rows.Next(dest); err != nil {
		t.Fatalf("next: %v", err)
	}
	if dest[0] != int64(7) || dest[1] != "alice" {
		t.Fatalf("row scalar values = %#v, want id/name", dest)
	}
	payload, ok := dest[2].([]byte)
	if !ok || string(payload) != "\x01\x02\x03" {
		t.Fatalf("payload = %#v, want []byte{1,2,3}", dest[2])
	}
	if err := rows.Next(dest); err != io.EOF {
		t.Fatalf("next after row = %v, want EOF", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("rows close: %v", err)
	}

	if err := writeTx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	readTx.Discard()

	fakeStore.mtx.Lock()
	defer fakeStore.mtx.Unlock()
	if len(fakeStore.txs) != 2 {
		t.Fatalf("tx count = %d, want 2", len(fakeStore.txs))
	}
	if !fakeStore.txs[0].committed {
		t.Fatal("write tx was not committed")
	}
	if !fakeStore.txs[1].discarded {
		t.Fatal("read tx was not discarded")
	}
	if fakeStore.txs[0].dsn != "primary" || fakeStore.txs[1].dsn != "readonly" {
		t.Fatalf("dsns = %q, %q; want primary, readonly", fakeStore.txs[0].dsn, fakeStore.txs[1].dsn)
	}
}

type fakeSqlStore struct {
	mtx sync.Mutex
	txs []*fakeSqlTx
}

func (s *fakeSqlStore) NewSqlTransaction(ctx context.Context, write bool, dsn string) (hydra_sql.SqlTransaction, error) {
	tx := &fakeSqlTx{
		readOnly: !write,
		dsn:      dsn,
		ops:      &fakeSqlOps{},
	}
	s.mtx.Lock()
	s.txs = append(s.txs, tx)
	s.mtx.Unlock()
	return tx, nil
}

type fakeSqlTx struct {
	readOnly  bool
	dsn       string
	ops       hydra_sql.SqlOps
	committed bool
	discarded bool
}

func (t *fakeSqlTx) Commit(ctx context.Context) error {
	t.committed = true
	return nil
}

func (t *fakeSqlTx) Discard() {
	t.discarded = true
}

func (t *fakeSqlTx) GetReadOnly() bool {
	return t.readOnly
}

func (t *fakeSqlTx) GetSqlOps(ctx context.Context) (hydra_sql.SqlOps, error) {
	return t.ops, nil
}

type fakeSqlOps struct{}

func (o *fakeSqlOps) Exec(query string, args []driver.Value) (driver.Result, error) {
	return o.ExecContext(context.Background(), query, sql_rpc.ValuesToNamedValues(args))
}

func (o *fakeSqlOps) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return fakeResult{
		lastInsertID: int64(len(args)),
		rowsAffected: int64(len(args)),
	}, nil
}

func (o *fakeSqlOps) Query(query string, args []driver.Value) (driver.Rows, error) {
	return o.QueryContext(context.Background(), query, sql_rpc.ValuesToNamedValues(args))
}

func (o *fakeSqlOps) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return &fakeRows{
		columns: []string{"id", "name", "payload"},
		types:   []string{"BIGINT", "TEXT", "BLOB"},
		rows: [][]driver.Value{
			{int64(7), "alice", []byte{1, 2, 3}},
		},
	}, nil
}

type fakeResult struct {
	lastInsertID int64
	rowsAffected int64
}

func (r fakeResult) LastInsertId() (int64, error) {
	return r.lastInsertID, nil
}

func (r fakeResult) RowsAffected() (int64, error) {
	return r.rowsAffected, nil
}

type fakeRows struct {
	columns []string
	types   []string
	rows    [][]driver.Value
	index   int
}

func (r *fakeRows) Columns() []string {
	return r.columns
}

func (r *fakeRows) Close() error {
	return nil
}

func (r *fakeRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.index])
	r.index++
	return nil
}

func (r *fakeRows) ColumnTypeDatabaseTypeName(index int) string {
	if index < 0 || index >= len(r.types) {
		return ""
	}
	return r.types[index]
}

var (
	_ hydra_sql.SqlStore                    = (*fakeSqlStore)(nil)
	_ hydra_sql.SqlTransaction              = (*fakeSqlTx)(nil)
	_ hydra_sql.SqlOps                      = (*fakeSqlOps)(nil)
	_ driver.Rows                           = (*fakeRows)(nil)
	_ driver.RowsColumnTypeDatabaseTypeName = (*fakeRows)(nil)
)
