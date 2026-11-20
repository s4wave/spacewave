//go:build !js && !wasip1

package s4wave_sql_world

import (
	"context"
	"net"
	"sync"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/vitess/go/mysql"
	"github.com/dolthub/vitess/go/sqltypes"
	querypb "github.com/dolthub/vitess/go/vt/proto/query"
	"github.com/dolthub/vitess/go/vt/sqlparser"
)

// mysqlServer owns protocol clients until their SQL transactions have ended.
type mysqlServer struct {
	// ctx cancels active commands when the endpoint stops.
	ctx context.Context
	// cancel stops the endpoint after an indeterminate publication.
	cancel context.CancelCauseFunc
	// store publishes SQL changes through its World object.
	store *WorldBackedSql
	// listener owns accepted sockets before their protocol goroutines start.
	listener net.Listener
	// mtx protects membership, never SQL execution or publication.
	mtx sync.Mutex
	// clients includes accepted sockets still awaiting their handshake.
	clients map[net.Conn]*mysqlSession
	// users joins transaction cleanup, including incomplete handshakes.
	users sync.WaitGroup
}

// ServeMySQL serves this World object's databases using the MySQL protocol.
// It owns listener and returns only after every client transaction is released.
// The caller owns the store and must Close it afterwards. Authentication and
// listener reachability are supplied by the attachment's owner.
func (s *WorldBackedSql) ServeMySQL(ctx context.Context, listener net.Listener, auth mysql.AuthServer) error {
	// Own endpoint cancellation independently from the storage object's lifetime.
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	h := &mysqlServer{ctx: ctx, cancel: cancel, store: s, listener: listener, clients: make(map[net.Conn]*mysqlSession)}
	l, err := mysql.NewFromListener(h, auth, h, 0, 0)
	if err != nil {
		_ = listener.Close()
		return err
	}
	l.ServerVersion = "8.0.33-Spacewave"

	// Closing admission wakes Accept; closing sockets wakes idle client readers.
	stopped := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		// Stop admission and snapshot only the currently accepted sockets.
		l.Close()
		h.mtx.Lock()
		clients := make([]net.Conn, 0, len(h.clients))
		for c := range h.clients {
			clients = append(clients, c)
		}
		h.mtx.Unlock()

		// Close sockets outside the membership lock, then publish completed stop.
		for _, c := range clients {
			_ = c.Close()
		}
		close(stopped)
	})
	defer stop()
	l.Accept()
	cancel(nil)
	<-stopped
	h.users.Wait()
	return context.Cause(ctx)
}

// Accept registers a socket before the protocol goroutine can run or stop.
func (s *mysqlServer) Accept() (net.Conn, error) {
	// Accept without holding the client membership lock.
	c, err := s.listener.Accept()
	if err != nil {
		if s.ctx.Err() == nil {
			s.cancel(err)
		}
		return nil, err
	}

	// Exclude admission after cancellation, including an accept racing with stop.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.ctx.Err() != nil {
		_ = c.Close()
		return nil, s.ctx.Err()
	}
	s.users.Add(1)
	s.clients[c] = nil
	return c, nil
}

// Close ends socket admission.
func (s *mysqlServer) Close() error { return s.listener.Close() }

// Addr reports the attachment's listener address.
func (s *mysqlServer) Addr() net.Addr { return s.listener.Addr() }

// NewConnection initializes client state before authentication.
func (s *mysqlServer) NewConnection(c *mysql.Conn) {
	// Associate the protocol session with the already admitted raw socket.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.clients[c.Conn] = newMysqlSession(s, c)
	c.StatusFlags |= mysql.ServerStatusAutocommit
	c.DisableClientMultiStatements = true
}

// session resolves state owned by one serialized protocol connection.
func (s *mysqlServer) session(c *mysql.Conn) *mysqlSession {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.clients[c.Conn]
}

// ConnectionClosed discards unpublished work before releasing endpoint custody.
func (s *mysqlServer) ConnectionClosed(c *mysql.Conn) {
	// Release the transaction outside the membership lock.
	client := s.session(c)
	client.discard()

	// Complete the admitted user's lifetime after all SQL locks are released.
	s.mtx.Lock()
	delete(s.clients, c.Conn)
	s.mtx.Unlock()
	s.users.Done()
}

// ConnectionAuthenticated records the identity accepted by the supplied auth server.
func (s *mysqlServer) ConnectionAuthenticated(c *mysql.Conn) error {
	s.session(c).SetClient(sql.Client{User: c.User, Address: c.RemoteAddr().String(), Capabilities: c.Capabilities})
	return nil
}

// ConnectionAborted retains normal ConnectionClosed cleanup.
func (s *mysqlServer) ConnectionAborted(*mysql.Conn, string) error { return nil }

// ComInitDB validates and selects the client's default database.
func (s *mysqlServer) ComInitDB(c *mysql.Conn, name string) error {
	return s.ComQuery(s.ctx, c, "USE "+sqlparser.String(sqlparser.NewTableIdent(name)), func(*sqltypes.Result, bool) error { return nil })
}

// ComQuery executes and streams a command under the client's transaction.
func (s *mysqlServer) ComQuery(_ context.Context, c *mysql.Conn, query string, callback mysql.ResultSpoolFn) error {
	ctx, stop, finish := s.watchCommand(c)
	defer finish()
	return mysqlCommandError(s.session(c).query(ctx, query, nil, finalMysqlResult(stop, callback)))
}

// ComMultiQuery delegates the single statement allowed by this attachment.
func (s *mysqlServer) ComMultiQuery(ctx context.Context, c *mysql.Conn, query string, callback mysql.ResultSpoolFn) (string, error) {
	return "", s.ComQuery(ctx, c, query, callback)
}

// ComPrepare analyzes metadata without executing or retaining storage handles.
func (s *mysqlServer) ComPrepare(_ context.Context, c *mysql.Conn, query string, _ *mysql.PrepareData) ([]*querypb.Field, error) {
	ctx, _, finish := s.watchCommand(c)
	defer finish()
	fields, err := s.session(c).prepare(ctx, query)
	return fields, mysqlCommandError(err)
}

// ComStmtExecute converts bound arguments and streams the original SQL command.
func (s *mysqlServer) ComStmtExecute(_ context.Context, c *mysql.Conn, p *mysql.PrepareData, callback func(*sqltypes.Result) error) error {
	// Preserve protocol parameter order when translating the bound values.
	args, err := mysqlBindArgs(p)
	if err != nil {
		return err
	}

	// The SQL store owns query execution, statement recording and publication.
	ctx, stop, finish := s.watchCommand(c)
	defer finish()
	return mysqlCommandError(s.session(c).query(ctx, p.PrepareStmt, args, finalMysqlResult(stop, func(r *sqltypes.Result, _ bool) error { return callback(r) })))
}

// watchCommand cancels a command's work on client disconnect and joins its socket
// watch. A commit that has started is exempt: it runs under the endpoint's context.
func (s *mysqlServer) watchCommand(c *mysql.Conn) (context.Context, func(), func()) {
	// The protocol is half duplex: incoming bytes during work abandon the command.
	ctx, cancel := context.WithCancelCause(s.ctx)
	watchCtx, stopWatch := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		if err := c.WaitForClientActivity(watchCtx); err != nil {
			cancel(err)
			c.Close()
		}
		close(done)
	}()

	// Join before normal protocol reads resume, including on an execution error.
	stop := func() {
		stopWatch()
		<-done
	}
	return ctx, stop, func() {
		stop()
		cancel(nil)
	}
}

// finalMysqlResult ends the read watch before the client can send its next command.
func finalMysqlResult(stop func(), callback mysql.ResultSpoolFn) mysql.ResultSpoolFn {
	return func(r *sqltypes.Result, more bool) error {
		if len(r.Rows) < mysqlRowsBatch {
			stop()
		}
		return callback(r, more)
	}
}

// mysqlCommandError preserves SQL error numbers without returning a typed nil.
func mysqlCommandError(err error) error {
	if err == nil {
		return nil
	}
	return sql.CastSQLError(err)
}

// WarningCount reports the engine session's current warnings.
func (s *mysqlServer) WarningCount(c *mysql.Conn) uint16 { return s.session(c).WarningCount() }

// ComResetConnection rolls back and replaces the client's session state.
func (s *mysqlServer) ComResetConnection(c *mysql.Conn) error {
	// Discard storage state before giving the pooled connection a fresh session.
	s.session(c).discard()
	s.NewConnection(c)
	return s.ConnectionAuthenticated(c)
}

// ParserOptionsForConnection uses the SQL mode selected by this client.
func (s *mysqlServer) ParserOptionsForConnection(c *mysql.Conn) (sqlparser.ParserOptions, error) {
	return s.session(c).parserOptions(), nil
}

// _ verifies the protocol and listener interfaces.
var (
	_ mysql.Handler = (*mysqlServer)(nil)
	_ net.Listener  = (*mysqlServer)(nil)
)
