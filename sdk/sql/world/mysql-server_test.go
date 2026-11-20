//go:build !js && !wasip1

package s4wave_sql_world_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dolthub/vitess/go/mysql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
	"github.com/s4wave/spacewave/testbed"
)

// TestMySQLWorldEndpoint walks real client transactions, variables and shutdown.
func TestMySQLWorldEndpoint(t *testing.T) {
	// Build a real World SQL object and serve its protocol on a private listener.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	key := "sql/wire-test"
	createSqlDbObject(t, ctx, tb.WorldState, key, true)
	ws := &heldWorld{WorldState: tb.WorldState, entered: make(chan struct{}), release: make(chan struct{})}
	store, err := s4wave_sql_world.NewWorldBackedSql(ctx, ws, key)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	addr, stop := serveMySQLWorld(t, ctx, store, "127.0.0.1:0")
	defer stop()

	// Create schema and write a prepared statement through the real MySQL driver.
	db, err := sql.Open("mysql", "root@tcp("+addr+")/")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, query := range []string{"CREATE DATABASE app", "USE app", "CREATE TABLE issue (id BIGINT PRIMARY KEY AUTO_INCREMENT, body TEXT NOT NULL)", "SET SESSION time_zone = '+00:00'"} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	result, err := db.ExecContext(ctx, "INSERT INTO issue (body) VALUES (?)", "original")
	if err != nil {
		t.Fatal(err)
	}
	if id, err := result.LastInsertId(); err != nil || id != 1 {
		t.Fatalf("first generated ID=%d err=%v", id, err)
	}

	// Preserve the authenticated client identity in the engine session.
	var user string
	if err := db.QueryRowContext(ctx, "SELECT CURRENT_USER()").Scan(&user); err != nil || !strings.HasPrefix(user, "root@") {
		t.Fatalf("authenticated user=%q err=%v", user, err)
	}

	// Roll back a mutation and read the original row through another prepared query.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE issue SET body = ? WHERE id = ?", "discarded", 1); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var body string
	if err := db.QueryRowContext(ctx, "SELECT body FROM issue WHERE id = ?", 1).Scan(&body); err != nil || body != "original" {
		t.Fatalf("readback body=%q err=%v", body, err)
	}

	// SQL autocommit mode commits on enable and preserves client variables.
	for _, query := range []string{"SET autocommit = 0", "INSERT INTO issue (body) VALUES ('mode commit')", "SET autocommit = 1"} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	if err := db.QueryRowContext(ctx, "SELECT @@session.time_zone").Scan(&body); err != nil || body != "+00:00" {
		t.Fatalf("session timezone=%q err=%v", body, err)
	}

	// A second connection selects the same object and sees the committed row.
	other, err := sql.Open("mysql", "root@tcp("+addr+")/app")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := other.QueryRowContext(ctx, "SELECT body FROM issue WHERE id = 1").Scan(&body); err != nil || body != "original" {
		t.Fatalf("reconnect body=%q err=%v", body, err)
	}

	// A command whose client left while it waited for the writer never executes.
	held, err := other.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	abandonMySQLCommand(t, ctx, addr, "INSERT INTO app.issue (body) VALUES ('queued')", nil)
	if err := held.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := other.QueryRowContext(ctx, "SELECT COUNT(*) FROM issue").Scan(&count); err != nil || count != 2 {
		t.Fatalf("abandoned queued command: count=%d err=%v", count, err)
	}

	// A client that leaves during publication neither interrupts the commit nor
	// stops the endpoint: the next reader waits for the publication and sees its row.
	ws.hold.Store(true)
	abandonMySQLCommand(t, ctx, addr, "INSERT INTO app.issue (body) VALUES ('publishing')", ws.entered)
	close(ws.release)
	if err := other.QueryRowContext(ctx, "SELECT COUNT(*) FROM issue").Scan(&count); err != nil || count != 3 {
		t.Fatalf("abandoned publishing command: count=%d err=%v", count, err)
	}

	// Include a client that received the greeting but never completed its handshake.
	raw, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if err := raw.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Read(make([]byte, 256)); err != nil {
		t.Fatal(err)
	}

	// Endpoint cancellation must release an idle explicit transaction before return.
	pending, err := other.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer pending.Rollback()
	cancel()
}

// heldWorld holds one publication so a test can act while a commit is inside it.
type heldWorld struct {
	world.WorldState
	// hold arms the next publication to wait for release.
	hold atomic.Bool
	// entered closes when the held publication starts.
	entered chan struct{}
	// release closes to let the held publication continue.
	release chan struct{}
}

// ApplyWorldOp holds an armed publication, then fails it as the World would if
// its context ended meanwhile.
func (w *heldWorld) ApplyWorldOp(ctx context.Context, op world.Operation, sender peer.ID) (uint64, bool, error) {
	if w.hold.CompareAndSwap(true, false) {
		close(w.entered)
		<-w.release
		if err := ctx.Err(); err != nil {
			return 0, false, err
		}
	}
	return w.WorldState.ApplyWorldOp(ctx, op, sender)
}

// abandonMySQLCommand sends query on its own connection and leaves without
// reading the reply, after working closes when it is non-nil. It returns once
// the server has closed the connection. The server cancels an abandoned command
// before it closes the socket, so by then the endpoint has seen the client go.
func abandonMySQLCommand(t *testing.T, ctx context.Context, addr, query string, working <-chan struct{}) {
	// Select no database: that would wait behind the writer a caller may hold.
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	c, err := mysql.Connect(ctx, &mysql.ConnParams{Host: host, Port: port, Uname: "root"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.WriteComQuery(query); err != nil {
		t.Fatal(err)
	}
	if working != nil {
		<-working
	}

	// Close the sending half only, so the server's own close stays observable.
	if err := c.Conn.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if err := c.Conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, c.Conn); errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("server kept an abandoned connection: %s", query)
	}
}

// serveMySQLWorld owns a test endpoint and returns an idempotent joined stop.
func serveMySQLWorld(t *testing.T, ctx context.Context, store *s4wave_sql_world.WorldBackedSql, addr string) (string, func()) {
	// Bind before serving so the caller can connect without a readiness delay.
	t.Helper()
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	var serveErr error
	go func() {
		serveErr = store.ServeMySQL(ctx, lis, mysql.NewAuthServerNone())
		close(done)
	}()

	// Waiting for cleanup also publishes the endpoint's final error.
	return lis.Addr().String(), func() {
		cancel()
		<-done
		if serveErr != nil && !errors.Is(serveErr, context.Canceled) {
			t.Errorf("SQL endpoint stop: %v", serveErr)
		}
	}
}
