//go:build !js && !wasip1

package s4wave_sql_world

import (
	"context"
	"database/sql/driver"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/types"
	"github.com/dolthub/vitess/go/mysql"
	"github.com/dolthub/vitess/go/sqltypes"
	querypb "github.com/dolthub/vitess/go/vt/proto/query"
	"github.com/dolthub/vitess/go/vt/sqlparser"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_mysql "github.com/s4wave/spacewave/db/sql/mysql"
)

// mysqlSession owns one client's variables and optional storage transaction.
// A commit reply follows World publication. Disconnect never interrupts or
// undoes publication and never retries a mutation whose outcome is unknown.
type mysqlSession struct {
	// BaseSession retains SQL variables independently of storage transactions.
	*sql.BaseSession
	// server owns cancellation and the attached World SQL object.
	server *mysqlServer
	// conn holds the protocol's transaction status bits.
	conn *mysql.Conn
	// tx is retained only during an active command or explicit transaction.
	tx hydra_sql.SqlTransaction
	// settings holds changed session variables for deterministic World replay.
	settings map[string]any
}

// newMysqlSession creates a connection-local engine session.
func newMysqlSession(server *mysqlServer, conn *mysql.Conn) *mysqlSession {
	base := sql.NewBaseSessionWithClientServerWithVariables(
		"Spacewave",
		sql.Client{User: conn.User},
		conn.ConnectionID,
		server.store.inner.Variables(),
	)
	return &mysqlSession{BaseSession: base, server: server, conn: conn, settings: make(map[string]any)}
}

// SetSessionVariable records the value the engine accepted for later replay.
func (s *mysqlSession) SetSessionVariable(ctx *sql.Context, name string, value any) error {
	// Validate and normalize values using the engine's session variable registry.
	if err := s.BaseSession.SetSessionVariable(ctx, name, value); err != nil {
		return err
	}

	// Keep only the current value of each changed variable.
	v, err := s.GetSessionVariable(ctx, name)
	if err == nil {
		s.settings[name] = v
	}
	return err
}

// parserOptions reflects session-local SQL mode.
func (s *mysqlSession) parserOptions() sqlparser.ParserOptions {
	return sql.LoadSqlMode(sql.NewContext(s.server.ctx, sql.WithSession(s))).ParserOptions()
}

// open starts storage work and records the current session's replay prelude.
func (s *mysqlSession) open(ctx context.Context, write bool) error {
	// Reuse the explicit transaction; idle connections hold no storage locks.
	if s.tx != nil {
		return nil
	}
	ctx = sql_mysql.WithSession(ctx, s)
	t, err := s.server.store.NewSqlTransaction(ctx, write, "/"+s.GetCurrentDatabase())
	if err != nil {
		return err
	}
	s.tx = t
	if !write {
		return nil
	}

	// Replay needs session settings even when this transaction did not set them.
	ops, err := t.GetSqlOps(ctx)
	if err != nil {
		s.discard()
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(s.settings)) {
		value := s.settings[name]
		v, err := driver.DefaultParameterConverter.ConvertValue(value)
		if err == nil {
			_, err = ops.ExecContext(ctx, "SET SESSION "+sqlparser.String(sqlparser.NewColIdent(name))+" = ?", []driver.NamedValue{{Ordinal: 1, Value: v}})
		}
		if err != nil {
			s.discard()
			return err
		}
	}
	return nil
}

// discard rolls back uncommitted storage work and clears protocol transaction state.
func (s *mysqlSession) discard() {
	if s.tx != nil {
		s.tx.Discard()
		s.tx = nil
	}
	s.conn.StatusFlags &^= mysql.ServerInTransaction
}

// commit detaches the transaction before publishing and never retries its writes.
// It publishes under the endpoint's context, not the command's: a commit that a
// departing client interrupted between its SQL root and the World would leave a
// private root, which stops the endpoint for every client.
func (s *mysqlSession) commit() error {
	// An empty COMMIT succeeds without creating a storage transaction.
	if s.tx == nil {
		return nil
	}
	t := s.tx
	s.tx = nil
	s.conn.StatusFlags &^= mysql.ServerInTransaction
	err := t.Commit(s.server.ctx)
	t.Discard()

	// Reopen from World before accepting further work after uncertain publication.
	if errors.Is(err, ErrCommitPersisted) {
		s.server.cancel(err)
	}
	return err
}

// query executes one parsed command and acknowledges mutations after publication.
func (s *mysqlSession) query(ctx context.Context, query string, args []driver.NamedValue, callback mysql.ResultSpoolFn) error {
	// Parse using this connection's SQL mode before changing its transaction.
	if err := ctx.Err(); err != nil {
		return err
	}
	statement, err := sqlparser.ParseWithOptions(ctx, query, s.parserOptions())
	if err != nil {
		return err
	}
	switch st := statement.(type) {
	case *sqlparser.Begin:
		if err := s.commit(); err != nil {
			return err
		}
		if err := s.open(ctx, !strings.Contains(strings.ToLower(st.TransactionCharacteristic), "read only")); err != nil {
			return err
		}
		s.conn.StatusFlags |= mysql.ServerInTransaction
		return callback(&sqltypes.Result{}, false)
	case *sqlparser.Commit:
		if err := s.commit(); err != nil {
			return err
		}
		return callback(&sqltypes.Result{}, false)
	case *sqlparser.Rollback:
		s.discard()
		return callback(&sqltypes.Result{}, false)
	}

	// Session-only commands use short read handles; explicit transactions retain writes.
	own := s.tx == nil
	auto := s.conn.StatusFlags&mysql.ServerStatusAutocommit != 0
	read, sessionOnly := false, false
	switch statement.(type) {
	case sqlparser.SelectStatement, *sqlparser.Show, *sqlparser.OtherRead, *sqlparser.Explain:
		read = true
	case *sqlparser.Set, *sqlparser.Use:
		sessionOnly = true
	}

	// Acquire only the storage mode needed by this command and session.
	short := own && (auto || sessionOnly)
	write := !sessionOnly && (!read || !auto)
	if err := s.open(ctx, write); err != nil {
		return err
	}
	if !short {
		s.conn.StatusFlags |= mysql.ServerInTransaction
	}
	defer func() {
		if short {
			s.discard()
		}
	}()
	ops, err := s.tx.GetSqlOps(ctx)
	if err != nil {
		return err
	}

	// Row-producing commands keep their transaction until the stream is complete.
	switch statement.(type) {
	case sqlparser.SelectStatement, *sqlparser.Show, *sqlparser.OtherRead, *sqlparser.Explain:
		rows, err := ops.QueryContext(ctx, query, args)
		if err != nil {
			return err
		}
		if err := streamMysqlRows(rows, callback); err != nil {
			return err
		}
		return nil
	}

	// Execute the statement and read the resulting session mode.
	result, err := ops.ExecContext(ctx, query, args)
	if err != nil {
		return err
	}
	value, _ := s.GetSessionVariable(sql.NewContext(ctx, sql.WithSession(s)), "autocommit")

	// Enabling autocommit commits the explicit transaction before acknowledging SET.
	_, setting := statement.(*sqlparser.Set)
	if short && write || setting && !short && value != int8(0) && s.conn.StatusFlags&mysql.ServerStatusAutocommit == 0 {
		if err := s.commit(); err != nil {
			return err
		}
	}

	// Return an OK packet only after publication and update the client's mode.
	if value == int8(0) {
		s.conn.StatusFlags &^= mysql.ServerStatusAutocommit
	} else {
		s.conn.StatusFlags |= mysql.ServerStatusAutocommit
	}
	changed, _ := result.RowsAffected()
	inserted, _ := result.LastInsertId()

	// Restore the native driver's uint64 bit pattern on the MySQL wire.
	return callback(&sqltypes.Result{RowsAffected: uint64(changed), InsertID: uint64(inserted)}, false) //nolint:gosec
}

// prepare analyzes against the current catalog without retaining a prepared table.
func (s *mysqlSession) prepare(ctx context.Context, query string) ([]*querypb.Field, error) {
	// Open a short read transaction unless this client already holds a transaction.
	auto := s.tx == nil
	if err := s.open(ctx, false); err != nil {
		return nil, err
	}
	if auto {
		defer s.discard()
	}

	// Resolve the native catalog without bypassing World transaction ownership.
	t := s.tx
	if wrapped, ok := t.(*worldBackedSqlTx); ok {
		t = wrapped.inner
	}
	native := t.(*sql_mysql.SqlTx)
	sqlCtx := sql.NewContext(ctx, sql.WithSession(s))

	// Bind against this transaction, retaining only the parser AST in the session.
	node, err := native.PrepareQuery(ctx, s, query)
	if err != nil {
		return nil, err
	}
	schema := node.Schema(sqlCtx)
	if types.IsOkResultSchema(schema) {
		return nil, nil
	}
	fields := make([]*querypb.Field, len(schema))
	for i, column := range schema {
		fields[i] = &querypb.Field{
			Name:         column.Name,
			Type:         column.Type.Type(),
			Charset:      uint32(sql.Collation_utf8mb4_general_ci),
			ColumnLength: column.Type.MaxTextResponseByteLength(sqlCtx),
		}
	}
	return fields, nil
}

// mysqlBindArgs translates Vitess bind variables into driver values in wire order.
func mysqlBindArgs(p *mysql.PrepareData) ([]driver.NamedValue, error) {
	args := make([]driver.NamedValue, int(p.ParamsCount))
	for i := range args {
		v, err := sqltypes.BindVariableToValue(p.BindVars["v"+strconv.Itoa(i+1)])
		if err != nil {
			return nil, err
		}
		value, err := sqltypes.ToNative(v)
		if err != nil {
			return nil, err
		}
		args[i] = driver.NamedValue{Ordinal: i + 1, Value: value}
	}
	return args, nil
}
