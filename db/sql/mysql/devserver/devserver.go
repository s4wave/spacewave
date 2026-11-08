// Package mysql_devserver serves an in-memory Spacewave SQL database over the
// MySQL wire protocol, so a build-time tool that types queries by preparing
// them, such as Reylo, asks the same engine a Space's sql/db objects run on.
package mysql_devserver

import (
	"context"
	"database/sql/driver"
	"net"
	"strings"

	sqle "github.com/dolthub/go-mysql-server"
	gms_server "github.com/dolthub/go-mysql-server/server"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/vitess/go/mysql"
	"github.com/dolthub/vitess/go/vt/sqlparser"
	"github.com/pkg/errors"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	sql_mysql "github.com/s4wave/spacewave/db/sql/mysql"
)

// Server is a running dev SQL endpoint. Its database is built once from the
// schema and read-only afterwards: it answers prepares and catalog reads, and
// any statement that writes fails.
type Server struct {
	// srv is the MySQL protocol server.
	srv *gms_server.Server
	// store owns the engine used to construct the schema.
	store *sql_mysql.Mysql
	// engine owns the read-only protocol catalog.
	engine *sqle.Engine
	// tx is the read transaction every connection shares.
	tx *sql_mysql.Tx
	// url is the endpoint's mysql:// URL.
	url string
	// done is closed when srv stops serving.
	done chan struct{}
	// err is the serve error, set before done is closed.
	err error
}

// Start builds database dbName from the schema statements and serves it on
// lis until Close. It takes ownership of lis.
func Start(ctx context.Context, lis net.Listener, dbName string, schema []string) (_ *Server, retErr error) {
	// Build the database from the schema in a write transaction.
	store := sql_mysql.NewMysql(&memoryRoot{store: block_mock.NewMockStore(0)}, nil)
	defer func() {
		if retErr != nil {
			_ = store.Close()
		}
	}()
	if err := applySchema(ctx, store, dbName, schema); err != nil {
		_ = lis.Close()
		return nil, err
	}

	// Open the read transaction the engine serves from.
	tx, err := store.NewMysqlTransaction(ctx, false)
	if err != nil {
		_ = lis.Close()
		return nil, err
	}
	catalog, err := tx.BuildDatabaseProvider(ctx)
	if err != nil {
		tx.Discard()
		_ = lis.Close()
		return nil, err
	}

	// Serve the engine on the listener, with one session per connection.
	engine := sqle.NewDefault(catalog)
	defer func() {
		if retErr != nil {
			_ = engine.Close()
		}
	}()
	buildSession := func(ctx context.Context, conn *mysql.Conn, addr string) (sql.Session, error) {
		client := sql.Client{Address: conn.RemoteAddr().String(), User: conn.User}
		return sql.NewBaseSessionWithClientServerWithVariables(addr, client, conn.ConnectionID, engine.Variables()), nil
	}
	srv, err := gms_server.NewServer(gms_server.Config{Listener: lis}, engine, sql.NewContext, buildSession, nil)
	if err != nil {
		tx.Discard()
		_ = lis.Close()
		return nil, err
	}
	s := &Server{
		srv:   srv,
		store: store, engine: engine,
		tx:   tx,
		url:  "mysql://root@" + lis.Addr().String() + "/" + dbName,
		done: make(chan struct{}),
	}
	go func() {
		s.err = srv.Start()
		close(s.done)
	}()
	return s, nil
}

// URL returns the endpoint as a mysql:// URL naming the database, the form
// DATABASE_URL takes.
func (s *Server) URL() string {
	return s.url
}

// Close stops the server, waits for it to stop serving, and releases the
// database.
func (s *Server) Close() error {
	// Stop protocol admission before releasing the shared read transaction.
	err := s.srv.Close()
	<-s.done
	s.tx.Discard()

	// Close both engines after the protocol server and its catalog release.
	engineErr := s.engine.Close()
	storeErr := s.store.Close()

	// Preserve the first shutdown failure while releasing every owned engine.
	if err != nil {
		return err
	}
	if engineErr != nil {
		return engineErr
	}
	return storeErr
}

// applySchema creates database dbName and runs each statement of the schema
// in it, then commits.
func applySchema(ctx context.Context, store *sql_mysql.Mysql, dbName string, schema []string) error {
	// Create the database in a write transaction.
	tx, err := store.NewMysqlTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if _, err := tx.OpenDatabase(ctx, dbName, true); err != nil {
		return errors.Wrap(err, "create database")
	}

	// Run each schema statement on a connection to the database.
	conn, err := sql_mysql.NewSqlConn(ctx, tx, "/"+dbName)
	if err != nil {
		return err
	}
	defer conn.Close()
	for _, text := range schema {
		stmts, err := sqlparser.SplitStatementToPieces(text)
		if err != nil {
			return errors.Wrap(err, "split schema")
		}
		for _, stmt := range stmts {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := conn.ExecContext(ctx, stmt, []driver.NamedValue{}); err != nil {
				return errors.Wrapf(err, "schema statement %q", stmt)
			}
		}
	}

	// Commit the built database.
	return tx.Commit(ctx)
}
