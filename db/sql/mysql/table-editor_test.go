package mysql

import (
	"slices"
	"strings"
	"testing"

	sqle "github.com/dolthub/go-mysql-server"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/pkg/errors"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// openEditorTestTable creates a table through SQL and returns its engine and table handle.
func openEditorTestTable(t *testing.T, name string, statements ...string) (*sql.Context, *sqle.Engine, *Table) {
	// Create a private database over an in-memory block store.
	t.Helper()
	ctx := sql.NewContext(t.Context())
	ctx.SetCurrentDatabase("probe")
	database := NewMysql(&indexTestRoot{store: block_mock.NewMockStore(0)}, nil)

	// Keep its writes in one transaction.
	tx, err := database.NewMysqlTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Discard)
	db, err := tx.OpenDatabase(ctx, "probe", true)
	if err != nil {
		t.Fatal(err)
	}

	// Build an engine over the transaction's databases.
	provider, err := tx.BuildDatabaseProvider(ctx)
	if err != nil {
		t.Fatal(err)
	}
	engine := sqle.NewDefault(provider)
	ctx = engineContext(ctx, engine)

	// Run the setup statements and resolve the table they built.
	for _, statement := range statements {
		if _, err := queryIndexTest(ctx, engine, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	table, _, err := db.GetTableInsensitive(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, engine, table.(*Table)
}

// queryRowStrings runs a query and returns its rows formatted and sorted.
func queryRowStrings(t *testing.T, ctx *sql.Context, engine *sqle.Engine, query string) []string {
	// Run the query.
	t.Helper()
	rows, err := queryIndexTest(ctx, engine, query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}

	// Format each row and sort the result, because scans promise no order.
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = sql.FormatRow(row)
	}
	slices.Sort(out)
	return out
}

// TestKeylessMutationTargetsStoredRow edits the exact duplicate the engine supplies.
// Under a case-insensitive collation 'a' and 'A' share a stored key prefix.
func TestKeylessMutationTargetsStoredRow(t *testing.T) {
	// Store the case variants with the one that is not edited first.
	ctx, engine, _ := openEditorTestTable(t, "keyless_ci",
		"CREATE TABLE keyless_ci (x VARCHAR(16) COLLATE utf8mb4_general_ci)",
		"INSERT INTO keyless_ci VALUES ('A'), ('a')",
	)

	// Compare binary values so each statement selects only one case variant.
	for _, test := range []struct {
		// statement edits one stored case variant.
		statement string
		// want is the formatted remaining rows.
		want string
	}{
		{"UPDATE keyless_ci SET x = 'z' WHERE BINARY x = 'a'", "[A] [z]"},
		{"DELETE FROM keyless_ci WHERE BINARY x = 'A'", "[z]"},
	} {
		if _, err := queryIndexTest(ctx, engine, test.statement); err != nil {
			t.Fatalf("%s: %v", test.statement, err)
		}
		got := strings.Join(queryRowStrings(t, ctx, engine, "SELECT x FROM keyless_ci"), " ")
		if got != test.want {
			t.Fatalf("%s: rows %s, want %s", test.statement, got, test.want)
		}
	}
}

// TestEditorDiscardChangesRestoresRows undoes deletes, updates and inserts of a failed statement.
func TestEditorDiscardChangesRestoresRows(t *testing.T) {
	for _, test := range []struct {
		// name identifies the table kind and the table it creates.
		name string
		// statements create and fill the table.
		statements []string
		// deleted, updated and updatedTo are the rows the failed statement edits.
		deleted, updated, updatedTo sql.Row
		// inserted is the row the failed statement adds.
		inserted sql.Row
	}{
		{
			name: "rows_pk",
			statements: []string{
				"CREATE TABLE rows_pk (id BIGINT PRIMARY KEY, tag VARCHAR(16) UNIQUE)",
				"INSERT INTO rows_pk VALUES (1, 'a'), (2, 'b'), (3, 'c')",
			},
			deleted: sql.Row{int64(1), "a"}, updated: sql.Row{int64(2), "b"}, updatedTo: sql.Row{int64(2), "x"},
			inserted: sql.Row{int64(4), "d"},
		},
		{
			name: "rows_keyless",
			statements: []string{
				"CREATE TABLE rows_keyless (tag VARCHAR(16), qty BIGINT)",
				"CREATE INDEX idx_tag ON rows_keyless(tag)",
				"INSERT INTO rows_keyless VALUES ('a', 1), ('a', 1), ('b', 2), ('c', 3)",
			},
			deleted: sql.Row{"a", int64(1)}, updated: sql.Row{"b", int64(2)}, updatedTo: sql.Row{"b", int64(5)},
			inserted: sql.Row{"d", int64(4)},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Record the scan, every edited secondary lookup and the row counter.
			ctx, engine, table := openEditorTestTable(t, test.name, test.statements...)
			snapshot := func() []string {
				out := queryRowStrings(t, ctx, engine, "SELECT * FROM "+test.name)
				for _, tag := range []string{"a", "b", "d", "x"} {
					out = append(out, queryRowStrings(t, ctx, engine, "SELECT * FROM "+test.name+" WHERE tag = '"+tag+"'")...)
					out = append(out, "lookup "+tag)
				}
				return out
			}
			before := snapshot()
			nonce := table.root.RowNonce

			// Apply one row of each edit kind, then fail the statement.
			editor := table.NewTableEditor(ctx)
			editor.StatementBegin(ctx)
			if err := editor.Delete(ctx, test.deleted); err != nil {
				t.Fatal(err)
			}
			if err := editor.Update(ctx, test.updated, test.updatedTo); err != nil {
				t.Fatal(err)
			}
			if err := editor.Insert(ctx, test.inserted); err != nil {
				t.Fatal(err)
			}
			if err := editor.DiscardChanges(ctx, errors.New("statement failed")); err != nil {
				t.Fatal(err)
			}

			// Every row, lookup and the hidden counter must match the start.
			if after := snapshot(); !slices.Equal(after, before) {
				t.Fatalf("after rollback %v, want %v", after, before)
			}
			if table.root.RowNonce != nonce {
				t.Fatalf("row nonce %d after rollback, want %d", table.root.RowNonce, nonce)
			}
		})
	}
}
