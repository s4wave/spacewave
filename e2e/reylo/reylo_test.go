//go:build !skip_e2e && !js

// Package reylo_test builds a plugin whose queries Reylo types against the
// plugin's SQL schema, then runs the built plugin against a World-backed
// sql/db and checks every statement against the transaction it ran on.
package reylo_test

import (
	"bufio"
	"bytes"
	"context"
	"database/sql/driver"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/dolthub/vitess/go/vt/sqlparser"
	"github.com/s4wave/spacewave/db/block"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_mysql "github.com/s4wave/spacewave/db/sql/mysql"
	mysql_devserver "github.com/s4wave/spacewave/db/sql/mysql/devserver"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	sql_rpc_server "github.com/s4wave/spacewave/db/sql/rpc/server"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
	"github.com/s4wave/spacewave/testbed"
)

// pluginDir is the plugin directory: its migrations, queries and entrypoint.
const pluginDir = "plugin"

// TestReyloPluginTransactions builds the plugin with Reylo typing its queries
// against the plugin's migrations, runs it against a World-backed sql/db, and
// checks that every query passed one transaction ran on that transaction.
func TestReyloPluginTransactions(t *testing.T) {
	// Bound the build and the run.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Build the plugin against its schema.
	schema := readSchema(t)
	bundle := buildPlugin(t, ctx, schema)

	// Open the database with the plugin's migrations, then log from empty.
	rec := openRecordedDb(t, ctx)
	applySchema(t, ctx, rec, schema)
	rec.reset()

	// Run the plugin against the database, as a host serves it.
	mux := srpc.NewMux()
	if err := sql_rpc.SRPCRegisterSql(mux, sql_rpc_server.NewStore(rec)); err != nil {
		t.Fatal(err)
	}
	statements := runPlugin(t, ctx, bundle, serveUnix(t, ctx, mux))

	// Each step ran its statements on one transaction, which ended as the
	// step required.
	insert, list, get := statements["insertItem"], statements["listItems"], statements["getItem"]
	want := []txRecord{
		{Write: true, Statements: []string{insert, insert, list}, Outcome: "committed"},
		{Statements: []string{list}, Outcome: "discarded"},
		{Write: true, Statements: []string{insert}, Outcome: "discarded"},
		{Statements: []string{get}, Outcome: "discarded"},
		{Statements: []string{get}, Outcome: "discarded"},
		{Write: true, Statements: []string{insert}, Outcome: "discarded"},
		{Write: true, Statements: []string{statements["renameItems"]}, Outcome: "committed"},
		{Write: true, Outcome: "discarded"},
		{Write: true, Statements: []string{insert}, Outcome: "discarded"},
		{Statements: []string{get}, Outcome: "discarded"},
	}

	// Reylo's own freshness probes ran inside those same transactions. Set
	// them aside, since its cache decides when they run.
	got := rec.records()
	probes := 0
	for i := range got {
		n := len(got[i].Statements)
		got[i].Statements = slices.DeleteFunc(got[i].Statements, isFreshnessProbe)
		probes += n - len(got[i].Statements)
	}
	if probes == 0 {
		t.Fatal("Reylo ran no schema freshness probe")
	}

	// Compare the transactions in order.
	if !slices.EqualFunc(got, want, txRecord.equal) {
		t.Fatalf("transactions:\n got %s\nwant %s", recordsText(got), recordsText(want))
	}
}

// openRecordedDb opens a World-backed sql/db on a testbed, recording each
// transaction it runs.
func openRecordedDb(t *testing.T, ctx context.Context) *recordingStore {
	// Start the testbed.
	t.Helper()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Create the typed sql/db object.
	objectKey := "sql/reylo"
	obj, _, err := world.CreateWorldObject(ctx, tb.WorldState, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(sql_mysql.NewRootBlock(), true)
		return nil
	})
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, tb.WorldState, objectKey, s4wave_sql_world.SqlDbTypeID); err != nil {
		t.Fatal(err)
	}

	// Open it as the sql/db object type does.
	store, err := s4wave_sql_world.NewWorldBackedSql(ctx, tb.WorldState, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return &recordingStore{inner: store}
}

// serveUnix serves mux on a new Unix socket until ctx ends and returns the
// socket path.
func serveUnix(t *testing.T, ctx context.Context, mux srpc.Mux) string {
	// Use a short directory: socket paths have a small length limit.
	t.Helper()
	dir, err := os.MkdirTemp("", "reylo")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	// Listen and accept connections in the background.
	path := filepath.Join(dir, "sql.sock")
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "unix", path)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srpc.AcceptMuxedListener(ctx, lis, srpc.NewServer(mux), nil) }()
	return path
}

// runPlugin runs the bundle with bun against the socket and returns the
// compiled statement of each query, which the plugin reports.
func runPlugin(t *testing.T, ctx context.Context, bundle, sockPath string) map[string]string {
	// Run the plugin to completion.
	t.Helper()
	cmd := exec.CommandContext(ctx, "bun", bundle, sockPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("plugin: %v\n%s%s", err, out, stderr.String())
	}

	// Read the plugin's report lines. The last line says the plugin
	// finished.
	statements := make(map[string]string)
	ok := false
	var p fastjson.Parser
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		v, err := p.ParseBytes(scanner.Bytes())
		if err != nil {
			t.Fatalf("plugin output %q: %v", scanner.Text(), err)
		}
		v.GetObject("statements").Visit(func(key []byte, v *fastjson.Value) {
			statements[string(key)] = string(v.GetStringBytes())
		})
		ok = ok || v.GetBool("ok")
	}
	if !ok {
		t.Fatalf("plugin did not finish:\n%s%s", out, stderr.String())
	}
	return statements
}

// readSchema reads the plugin's migrations, in name order.
func readSchema(t *testing.T) []string {
	t.Helper()
	_, texts, err := mysql_devserver.ReadSchemaFiles([]string{filepath.Join(pluginDir, "migrations")})
	if err != nil {
		t.Fatal(err)
	}
	return texts
}

// buildPlugin type checks the plugin and bundles it with Reylo, both against
// a dev database built from the schema, and returns the bundle path.
func buildPlugin(t *testing.T, ctx context.Context, schema []string) string {
	// Find the repository's tools. The build needs its installed packages.
	t.Helper()
	bin, err := filepath.Abs(filepath.Join("..", "..", "node_modules", ".bin"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(bin, "vite")); err != nil {
		t.Skip("node_modules not installed: run bun install")
	}
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun not found")
	}

	// Serve the schema as the plugin build does.
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := mysql_devserver.Start(ctx, lis, "app", schema)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	// Type check the plugin, then bundle it.
	outDir := t.TempDir()
	for _, args := range [][]string{
		{filepath.Join(bin, "tsrs"), "--noEmit", "--runExternalCode", "-p", "tsconfig.json"},
		{filepath.Join(bin, "vite"), "build", "--logLevel", "warn", "--outDir", outDir},
	} {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // the repository's own build tools.
		cmd.Dir = pluginDir
		cmd.Env = append(os.Environ(), "DATABASE_URL="+srv.URL())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", filepath.Base(args[0]), err, out)
		}
	}
	return filepath.Join(outDir, "main.js")
}

// applySchema creates the app database and runs each schema statement in it.
func applySchema(t *testing.T, ctx context.Context, store hydra_sql.SqlStore, schema []string) {
	// run commits the statements in one transaction on the dsn's database.
	t.Helper()
	run := func(dsn string, stmts []string) {
		// Open the transaction and its operations.
		tx, err := store.NewSqlTransaction(ctx, true, dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		ops, err := tx.GetSqlOps(ctx)
		if err != nil {
			t.Fatal(err)
		}

		// Run each statement, then commit.
		for _, stmt := range stmts {
			if _, err := ops.ExecContext(ctx, stmt, nil); err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Create the database, then build its tables.
	run("", []string{"CREATE DATABASE app"})
	var stmts []string
	for _, text := range schema {
		pieces, err := sqlparser.SplitStatementToPieces(text)
		if err != nil {
			t.Fatal(err)
		}
		for _, piece := range pieces {
			if strings.TrimSpace(piece) != "" {
				stmts = append(stmts, piece)
			}
		}
	}
	run("/app", stmts)
}

// txRecord is one transaction as the store saw it: its mode, the statements
// run on it in order, and how it ended.
type txRecord struct {
	Write      bool
	Statements []string
	Outcome    string
}

// equal compares two records.
func (r txRecord) equal(o txRecord) bool {
	return r.Write == o.Write && r.Outcome == o.Outcome && slices.Equal(r.Statements, o.Statements)
}

// isFreshnessProbe reports whether a statement is Reylo's check that the
// tables a query reads still have the columns it was typed against. Reylo
// runs it on the client the query was given, when its cache has no answer.
func isFreshnessProbe(query string) bool {
	return strings.HasPrefix(query, "select c.table_name as fresh_table,")
}

// recordingStore records each transaction the inner store opens.
type recordingStore struct {
	inner hydra_sql.SqlStore
	mtx   sync.Mutex
	txs   []*txRecord
}

// NewSqlTransaction opens a transaction on the inner store and records it.
func (s *recordingStore) NewSqlTransaction(ctx context.Context, write bool, dsn string) (hydra_sql.SqlTransaction, error) {
	// Open the inner transaction.
	tx, err := s.inner.NewSqlTransaction(ctx, write, dsn)
	if err != nil {
		return nil, err
	}

	// Append its record.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	rec := &txRecord{Write: write}
	s.txs = append(s.txs, rec)
	return &recordingTx{SqlTransaction: tx, s: s, rec: rec}, nil
}

// reset forgets the recorded transactions.
func (s *recordingStore) reset() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.txs = nil
}

// records returns a copy of the recorded transactions.
func (s *recordingStore) records() []txRecord {
	// Copy each record under the lock.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	out := make([]txRecord, len(s.txs))
	for i, rec := range s.txs {
		out[i] = txRecord{Write: rec.Write, Statements: slices.Clone(rec.Statements), Outcome: rec.Outcome}
	}
	return out
}

// update changes a record under the store's lock.
func (s *recordingStore) update(fn func()) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	fn()
}

// recordingTx records the statements run on a transaction and how it ends:
// the first Commit or Discard decides the outcome.
type recordingTx struct {
	hydra_sql.SqlTransaction
	s   *recordingStore
	rec *txRecord
}

// GetSqlOps returns the transaction's operations, recording each statement.
func (t *recordingTx) GetSqlOps(ctx context.Context) (hydra_sql.SqlOps, error) {
	ops, err := t.SqlTransaction.GetSqlOps(ctx)
	if err != nil {
		return nil, err
	}
	return &recordingOps{SqlOps: ops, tx: t}, nil
}

// Commit commits the transaction and records the result.
func (t *recordingTx) Commit(ctx context.Context) error {
	// Commit, then record whether it succeeded.
	err := t.SqlTransaction.Commit(ctx)
	outcome := "committed"
	if err != nil {
		outcome = "commit failed"
	}
	t.end(outcome)
	return err
}

// Discard discards the transaction and records it.
func (t *recordingTx) Discard() {
	t.SqlTransaction.Discard()
	t.end("discarded")
}

// end records the outcome unless one is already recorded.
func (t *recordingTx) end(outcome string) {
	t.s.update(func() {
		if t.rec.Outcome == "" {
			t.rec.Outcome = outcome
		}
	})
}

// add records a statement.
func (t *recordingTx) add(query string) {
	t.s.update(func() { t.rec.Statements = append(t.rec.Statements, query) })
}

// recordingOps records each statement before running it. The server runs
// statements through the context forms.
type recordingOps struct {
	hydra_sql.SqlOps
	tx *recordingTx
}

// ExecContext records and runs a statement.
func (o *recordingOps) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	o.tx.add(query)
	return o.SqlOps.ExecContext(ctx, query, args)
}

// QueryContext records and runs a query.
func (o *recordingOps) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	o.tx.add(query)
	return o.SqlOps.QueryContext(ctx, query, args)
}

// recordsText formats records for a failure message, one per line.
func recordsText(recs []txRecord) string {
	var b strings.Builder
	for _, rec := range recs {
		fmt.Fprintf(&b, "\n  write=%v %s %q", rec.Write, rec.Outcome, rec.Statements)
	}
	return b.String()
}
