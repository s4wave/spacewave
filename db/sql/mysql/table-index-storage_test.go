package mysql

import (
	"context"
	stdsql "database/sql"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"

	sqle "github.com/dolthub/go-mysql-server"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// engineContext returns a context and session bound to engine, which runs only
// queries whose context it built, with ctx's current database selected.
func engineContext(ctx *sql.Context, engine *sqle.Engine) *sql.Context {
	bound := engine.NewContext(ctx)
	bound.SetCurrentDatabase(ctx.GetCurrentDatabase())
	return bound
}

// queryIndexTest executes the engine's real query and row-iterator interfaces.
func queryIndexTest(ctx *sql.Context, engine *sqle.Engine, query string) ([]sql.Row, error) {
	// Drain the result so write errors and lookup-join probes are observed.
	_, rows, _, err := engine.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close(ctx)
	var result []sql.Row
	for {
		row, err := rows.Next(ctx)
		if err == io.EOF {
			return result, nil
		}
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}
}

// TestStoredPrimaryKeyReads verifies that equality consumers fetch only one row key.
// Tree navigation may read several branch blocks; unrelated row values must not load.
func TestStoredPrimaryKeyReads(t *testing.T) {
	// Populate an actual SQL engine over a private block store.
	ctx := sql.NewContext(t.Context())
	ctx.SetCurrentDatabase("probe")
	database := NewMysql(&indexTestRoot{store: block_mock.NewMockStore(0)}, nil)

	// Create the database and expose its write transaction to the SQL engine.
	tx, err := database.NewMysqlTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.OpenDatabase(ctx, "probe", true); err != nil {
		t.Fatal(err)
	}
	provider, err := tx.BuildDatabaseProvider(ctx)
	if err != nil {
		t.Fatal(err)
	}
	engine := sqle.NewDefault(provider)
	ctx = engineContext(ctx, engine)

	// Populate many inner rows and three distinct foreign-key probes.
	statements := []string{
		"CREATE TABLE users (id BIGINT PRIMARY KEY, name VARCHAR(64) NOT NULL)",
		"CREATE TABLE orders (id BIGINT PRIMARY KEY, user_id BIGINT NOT NULL)",
		"INSERT INTO orders VALUES (1, 37), (2, 41), (3, 53)",
	}
	for id := range 64 {
		value := strconv.Itoa(id)
		statements = append(statements, "INSERT INTO users VALUES ("+value+", 'user-"+value+"')")
	}
	statements = append(statements, "CREATE INDEX idx_name ON users(name)")
	for _, statement := range statements {
		if _, err := queryIndexTest(ctx, engine, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Identify committed inner-row blocks through the production table iterator.
	tx, err = database.NewMysqlTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}

	// Open the inner table to enumerate its committed row identities.
	db, err := tx.OpenDatabase(ctx, "probe", false)
	if err != nil {
		t.Fatal(err)
	}
	table, _, err := db.GetTableInsensitive(ctx, "users")
	if err != nil {
		t.Fatal(err)
	}
	users := table.(*Table)

	// Capture row references without fetching or decoding their values.
	rowRefs := make(map[string]struct{}, 64)
	for part := range users.root.TablePartitions {
		partition, err := users.PartitionAtIndex(part)
		if err != nil {
			t.Fatal(err)
		}
		tree, err := partition.BuildTreeTx(ctx, false, false)
		if err != nil {
			t.Fatal(err)
		}
		it := tree.BlockIterate(ctx, nil, false, false)
		for it.Next() {
			rowRefs[it.ValueCursor().GetRef().MarshalString()] = struct{}{}
		}
		if err := it.Err(); err != nil {
			t.Fatal(err)
		}
		it.Close()
		tree.Discard()
	}
	tx.Discard()
	if len(rowRefs) != 64 {
		t.Fatalf("found %d stored user keys, want 64", len(rowRefs))
	}

	// Each query starts cold; count fetched row keys independently of tree branches.
	cases := []struct {
		// name identifies the indexed consumer.
		name string
		// query is the SQL operation under test.
		query string
		// reads is the number of distinct user row keys that may be fetched.
		reads int
		// duplicate indicates that the operation must reject a duplicate primary key.
		duplicate bool
	}{
		{"point", "SELECT name FROM users WHERE id = 37", 1, false},
		{"secondary point", "SELECT name FROM users WHERE name = 'user-37'", 1, false},
		{"lookup join", "SELECT orders.id, users.name FROM orders JOIN users ON users.id = orders.user_id", 3, false},
		{"uniqueness conflict", "INSERT INTO users VALUES (37, 'duplicate')", 1, true},
		{"uniqueness miss", "INSERT INTO users VALUES (100, 'new')", 0, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			// Open a fresh transaction and record every fetched block identity.
			log := block.NewAccessLogger()
			ctx := sql.NewContext(block.WithAccessLogger(t.Context(), log))
			ctx.SetCurrentDatabase("probe")

			// Build the query engine against a fresh transaction with no decoded rows.
			tx, err := database.NewMysqlTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			provider, err := tx.BuildDatabaseProvider(ctx)
			if err != nil {
				t.Fatal(err)
			}
			engine := sqle.NewDefault(provider)
			ctx = engineContext(ctx, engine)

			// Require the join test to exercise the planner's lookup-join path.
			if test.name == "lookup join" {
				plan, err := queryIndexTest(ctx, engine, "EXPLAIN FORMAT=tree "+test.query)
				if err != nil {
					t.Fatal(err)
				}
				lines := make([]string, len(plan))
				for i, row := range plan {
					lines[i] = row[0].(string)
				}
				if !strings.Contains(strings.Join(lines, "\n"), "LookupJoin") {
					t.Fatalf("expected lookup join, got %v", lines)
				}
			}

			// Exercise one indexed consumer and check its success or duplicate error.
			result, err := queryIndexTest(ctx, engine, test.query)
			if (err != nil) != test.duplicate {
				t.Fatalf("duplicate=%v, result=%v, error=%v", test.duplicate, result, err)
			}

			// Count only stored user row values, not branch nodes or outer rows.
			reads := 0
			for _, access := range log.Snapshot().GetAccesses() {
				if _, ok := rowRefs[access.GetRef().MarshalString()]; ok {
					reads++
				}
			}
			if reads != test.reads {
				t.Fatalf("fetched %d user row keys, want %d", reads, test.reads)
			}
			if test.name == "lookup join" && len(result) != 3 {
				t.Fatalf("lookup join returned %d rows, want 3", len(result))
			}
			if strings.Contains(test.name, "point") && (len(result) != 1 || result[0][0] != "user-37") {
				t.Fatalf("point lookup returned %v", result)
			}
		})
	}
}

// TestSecondaryIndexRollback restores every tree after a partially applied statement.
func TestSecondaryIndexRollback(t *testing.T) {
	// Create distinct primary and secondary identities for a multi-row failure.
	ctx := t.Context()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE TABLE rollback_indexed (id BIGINT PRIMARY KEY, tag VARCHAR(32) UNIQUE)",
		"INSERT INTO rollback_indexed VALUES (1, 'a'), (2, 'b'), (3, 'c')",
	)

	// Each operation changes at least one row before a later row conflicts.
	for _, statement := range []string{
		"INSERT INTO rollback_indexed VALUES (4, 'd'), (5, 'a')",
		"UPDATE rollback_indexed SET tag = 'same'",
		"UPDATE rollback_indexed SET id = 10",
	} {
		if _, err := db.ExecContext(ctx, statement); err == nil {
			t.Fatalf("accepted conflicting statement: %s", statement)
		}

		// Verify the original key pairs through secondary lookups after rollback.
		for id, tag := range []string{"a", "b", "c"} {
			var got int64
			if err := db.QueryRowContext(ctx, "SELECT id FROM rollback_indexed WHERE tag = ?", tag).Scan(&got); err != nil {
				t.Fatalf("%s: %v", statement, err)
			}
			if got != int64(id+1) {
				t.Fatalf("%s: tag %s has primary key %d", statement, tag, got)
			}
		}

		// No inserted or rekeyed row may remain in either the primary or secondary tree.
		var count int64
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM rollback_indexed").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 3 {
			t.Fatalf("%s: rollback retained %d rows, want 3", statement, count)
		}
	}
}

// TestSecondaryIndexMutations verifies stored references through schema and row edits.
func TestSecondaryIndexMutations(t *testing.T) {
	// Backfill unique, duplicate and NULL secondary keys from committed rows.
	ctx := t.Context()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE TABLE indexed (id BIGINT PRIMARY KEY, tag VARCHAR(32), qty BIGINT)",
		"INSERT INTO indexed VALUES (1, 'alpha', 7), (2, 'beta', 7), (3, NULL, 9), (4, NULL, 9)",
		"CREATE UNIQUE INDEX idx_tag ON indexed(tag)",
		"CREATE INDEX idx_qty ON indexed(qty)",
		"UPDATE indexed SET id = 11, tag = 'gamma', qty = 8 WHERE id = 1",
		"DELETE FROM indexed WHERE id = 2",
		"ALTER TABLE indexed CHANGE COLUMN tag label VARCHAR(64)",
		"ALTER TABLE indexed MODIFY COLUMN id VARCHAR(16)",
	)

	// Query through each index after primary rekeying and indexed-column renaming.
	for _, test := range []struct {
		// predicate selects an indexed tuple.
		predicate string
		// want is the remaining number of matching rows.
		want int64
	}{
		{"label = 'alpha'", 0}, {"label = 'beta'", 0}, {"label = 'gamma'", 1},
		{"qty = 7", 0}, {"qty = 8", 1}, {"qty = 9", 2}, {"label IS NULL", 2},
		{"id = '11'", 1}, {"id = '1'", 0},
	} {
		var count int64
		query := "SELECT COUNT(*) FROM indexed WHERE " + test.predicate
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if count != test.want {
			t.Fatalf("%s: count=%d, want %d", query, count, test.want)
		}
	}

	// Reject duplicates, then remove the first index while preserving the second tree.
	if _, err := db.ExecContext(ctx, "INSERT INTO indexed (id, label, qty) VALUES ('20', 'gamma', 10)"); err == nil {
		t.Fatal("accepted a duplicate secondary key")
	}
	for _, step := range []struct {
		// query mutates the surviving index.
		query string
		// count is the expected number of indexed duplicate values.
		count int64
	}{
		{"DROP INDEX idx_tag ON indexed", 2},
		{"INSERT INTO indexed (id, label, qty) VALUES ('20', 'gamma', 9)", 3},
		{"DELETE FROM indexed WHERE id = '3'", 2},
	} {
		execMysqlStatements(t, ctx, db, step.query)
		var count int64
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM indexed WHERE qty = 9").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != step.count {
			t.Fatalf("%s: surviving secondary tree has %d matching rows, want %d", step.query, count, step.count)
		}
	}
}

// TestKeylessIndexDuplicates preserves duplicate rows and their separate references.
func TestKeylessIndexDuplicates(t *testing.T) {
	// Create duplicate keyless rows and a nonunique secondary index.
	ctx := t.Context()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE TABLE keyless (name VARCHAR(32), qty BIGINT)",
		"CREATE INDEX idx_name ON keyless(name)",
		"INSERT INTO keyless VALUES ('same', 1), ('same', 1), ('other', 2)",
		"UPDATE keyless SET qty = 3 WHERE name = 'same'",
		"DELETE FROM keyless WHERE name = 'other'",
	)

	// Both duplicate references must survive the full-row identity change.
	rows, err := db.QueryContext(ctx, "SELECT name, qty FROM keyless WHERE name = 'same'")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var name string
		var qty int64
		if err := rows.Scan(&name, &qty); err != nil {
			t.Fatal(err)
		}
		values = append(values, name+":"+strconv.FormatInt(qty, 10))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(values, " ") != "same:3 same:3" {
		t.Fatalf("keyless duplicates: %v", values)
	}
}

// TestIndexProbeConversionDoesNotMatchClampedKeys checks heterogeneous lookup joins.
func TestIndexProbeConversionDoesNotMatchClampedKeys(t *testing.T) {
	// Probe a narrow integer primary key with values outside its stored domain.
	ctx := t.Context()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE TABLE narrow_keys (id TINYINT PRIMARY KEY)",
		"INSERT INTO narrow_keys VALUES (127), (-128)",
		"CREATE TABLE wide_keys (id BIGINT PRIMARY KEY, target BIGINT)",
		"INSERT INTO wide_keys VALUES (1, 128), (2, -129), (3, 127)",
	)

	// Only the representable equality tuple may resolve a stored primary key.
	var count int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM wide_keys JOIN narrow_keys ON narrow_keys.id = wide_keys.target").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("out-of-range lookup probes matched %d rows, want 1", count)
	}
}

// TestPrefixIndexLookupJoin excludes rows sharing only a stored prefix.
func TestPrefixIndexLookupJoin(t *testing.T) {
	// Build a prefix bucket containing one exact and one inexact join candidate.
	ctx := t.Context()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE TABLE prefix_keys (id BIGINT PRIMARY KEY, name VARCHAR(32))",
		"CREATE INDEX idx_prefix ON prefix_keys(name(3))",
		"INSERT INTO prefix_keys VALUES (1, 'abc-one'), (2, 'abc-two')",
		"CREATE TABLE prefix_probes (id BIGINT PRIMARY KEY, name VARCHAR(32))",
		"INSERT INTO prefix_probes VALUES (1, 'abc-one')",
	)

	// Lookup joins must recheck fetched prefix candidates against the full value.
	var count int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM prefix_probes JOIN prefix_keys ON prefix_keys.name = prefix_probes.name").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("prefix lookup join returned %d rows, want 1", count)
	}
}

// queryStrings returns the first column of every row, with NULL as "NULL".
func queryStrings(t *testing.T, ctx context.Context, db *stdsql.DB, query string, args ...any) []string {
	// Run the query.
	t.Helper()
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()

	// Collect the first column of each row.
	var values []string
	for rows.Next() {
		var value stdsql.NullString
		if err := rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		if !value.Valid {
			value.String = "NULL"
		}
		values = append(values, value.String)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return values
}

// TestUnsupportedPredicatesScan returns correct rows for ranges the index walks
// as a span or cannot serve at all.
func TestUnsupportedPredicatesScan(t *testing.T) {
	// Index a composite key and the primary key, then ask for ranges they cannot probe.
	ctx := t.Context()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE TABLE scans (id BIGINT PRIMARY KEY, a BIGINT, b BIGINT)",
		"CREATE INDEX idx_ab ON scans(a, b)",
		"INSERT INTO scans VALUES (1, 1, 1), (3, 1, 2), (5, 2, 1), (7, 1, 1), (9, 2, 2)",
	)

	// A condition only on a later column scans; spans of number columns walk keys.
	for _, test := range []struct {
		// query selects through an unsupported predicate.
		query string
		// want is the matching ids in order.
		want string
	}{
		{"SELECT id FROM scans WHERE b = 1 ORDER BY id", "1 5 7"},
		{"SELECT id FROM scans WHERE a > 1 AND b = 1 ORDER BY id", "5"},
		{"SELECT id FROM scans WHERE id > 5 ORDER BY id", "7 9"},
		{"SELECT id FROM scans WHERE id BETWEEN 3 AND 7 ORDER BY id", "3 5 7"},
	} {
		if got := strings.Join(queryStrings(t, ctx, db, test.query), " "); got != test.want {
			t.Fatalf("%s: ids %q, want %q", test.query, got, test.want)
		}
	}
}

// TestNumberSpanLookups serves spans of number columns through the index and
// returns the rows a table scan returns, across negatives, signed zero, NULL,
// rounded and out-of-range bounds.
func TestNumberSpanLookups(t *testing.T) {
	// Store numbers on both sides of zero beside NULLs.
	ctx := t.Context()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE TABLE nums (id BIGINT PRIMARY KEY, g VARCHAR(8), n BIGINT, u TINYINT UNSIGNED, d DOUBLE)",
		"CREATE INDEX idx_n ON nums(n)",
		"CREATE INDEX idx_u ON nums(u)",
		"CREATE INDEX idx_d ON nums(d)",
		"CREATE INDEX idx_gn ON nums(g, n)",
		`INSERT INTO nums VALUES
			(1, 'a', -5, 0, -2.5), (2, 'a', -1, 1, -0.0), (3, 'a', 0, 200, 0),
			(4, 'b', 1, 255, 1.5), (5, 'b', 2, NULL, NULL), (6, 'b', 3, 7, 1e20),
			(7, 'a', 100, 8, -1e20), (8, NULL, NULL, 9, 2), (9, 'a', 9223372036854775807, 10, 3),
			(10, 'b', -9223372036854775808, 11, 4)`,
	)

	// Each span plans an index access and returns the rows of the same
	// predicate written so that it scans.
	for _, test := range []struct {
		// where selects through an index.
		where string
		// scan is the same predicate on expressions no index serves.
		scan string
	}{
		{"n > -2", "n + 0 > -2"},
		{"n >= -5 AND n < 3", "n + 0 >= -5 AND n + 0 < 3"},
		{"n <= 0", "n + 0 <= 0"},
		{"n BETWEEN -1 AND 2", "n + 0 BETWEEN -1 AND 2"},
		{"n > 2.5", "n + 0 > 2.5"},
		{"n < -0.5", "n + 0 < -0.5"},
		{"n > 0 OR n < -3", "n + 0 > 0 OR n + 0 < -3"},
		{"n < -1e30", "n + 0 < -1e30"},
		{"n > 1e30", "n + 0 > 1e30"},
		{"n IS NOT NULL", "(n + 0) IS NOT NULL"},
		{"u > -5", "u + 0 > -5"},
		{"u < 300", "u + 0 < 300"},
		{"u >= 200", "u + 0 >= 200"},
		{"u <= -1", "u + 0 <= -1"},
		{"d > -1", "d + 0 > -1"},
		{"d <= -0.0", "d + 0 <= -0.0"},
		{"d >= 0", "d + 0 >= 0"},
		{"d < 1.5", "d + 0 < 1.5"},
		{"g = 'a' AND n > 0", "CONCAT(g, '') = 'a' AND n + 0 > 0"},
		{"g = 'b' AND n <= 2", "CONCAT(g, '') = 'b' AND n + 0 <= 2"},
		{"id > 3 AND id <= 7", "id + 0 > 3 AND id + 0 <= 7"},
	} {
		query := "SELECT id FROM nums WHERE " + test.where + " ORDER BY id"
		plan := strings.Join(queryStrings(t, ctx, db, "EXPLAIN FORMAT=tree "+query), "\n")
		if !strings.Contains(plan, "IndexedTableAccess") {
			t.Fatalf("%s: plan scans the table:\n%s", query, plan)
		}
		got := strings.Join(queryStrings(t, ctx, db, query), " ")
		want := strings.Join(queryStrings(t, ctx, db, "SELECT id FROM nums WHERE "+test.scan+" ORDER BY id"), " ")
		if got != want {
			t.Fatalf("%s: ids %q, want %q", query, got, want)
		}
	}
}

// TestLeadingColumnLookups serves equality on the leading columns of a
// composite primary or secondary key through the index.
func TestLeadingColumnLookups(t *testing.T) {
	// Store keys whose text is a byte prefix of another key's text.
	ctx := t.Context()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE TABLE pairs (a VARCHAR(16), b BIGINT, c VARCHAR(16), d BIGINT, PRIMARY KEY (a, b))",
		"CREATE INDEX idx_cd ON pairs(c, d)",
		"INSERT INTO pairs VALUES ('x', 1, 'p', 1), ('x', 2, 'p', 2), ('xy', 1, 'pq', 1), ('y', 1, NULL, 3), ('y', 2, NULL, 4)",
	)

	// Each lookup plans an index access and returns only its own rows.
	for _, test := range []struct {
		// query selects by the leading index columns.
		query string
		// want is the matching rows in order.
		want string
	}{
		{"SELECT CONCAT(a, b) FROM pairs WHERE a = 'x' ORDER BY b", "x1 x2"},
		{"SELECT CONCAT(a, b) FROM pairs WHERE a = 'x' AND b > 1", "x2"},
		{"SELECT CONCAT(a, b) FROM pairs WHERE c = 'p' ORDER BY a, b", "x1 x2"},
		{"SELECT CONCAT(a, b) FROM pairs WHERE c = 'pq' AND d = 1", "xy1"},
		{"SELECT CONCAT(a, b) FROM pairs WHERE c IS NULL ORDER BY b", "y1 y2"},
		{"SELECT CONCAT(a, b) FROM pairs WHERE a IN ('x', 'xy') ORDER BY a, b", "x1 x2 xy1"},
	} {
		plan := strings.Join(queryStrings(t, ctx, db, "EXPLAIN FORMAT=tree "+test.query), "\n")
		if !strings.Contains(plan, "IndexedTableAccess") {
			t.Fatalf("%s: plan scans the table:\n%s", test.query, plan)
		}
		if got := strings.Join(queryStrings(t, ctx, db, test.query), " "); got != test.want {
			t.Fatalf("%s: rows %q, want %q", test.query, got, test.want)
		}
	}
}

// TestLookupJoinNullKeys never matches a NULL outer key to NULL inner rows.
func TestLookupJoinNullKeys(t *testing.T) {
	// Store NULL keys on both sides of an indexed join.
	ctx := t.Context()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE TABLE inner_keys (id BIGINT PRIMARY KEY, k BIGINT)",
		"CREATE INDEX idx_k ON inner_keys(k)",
		"INSERT INTO inner_keys VALUES (1, NULL), (2, NULL), (3, 7), (4, 8)",
		"CREATE TABLE outer_keys (id BIGINT PRIMARY KEY, k BIGINT)",
		"INSERT INTO outer_keys VALUES (1, NULL), (2, 7), (3, 9)",
	)

	// Only the equal non-null key joins; a NULL outer key joins nothing.
	query := "SELECT CONCAT(outer_keys.id, ':', inner_keys.id) FROM outer_keys JOIN inner_keys ON inner_keys.k = outer_keys.k ORDER BY outer_keys.id"
	if got := strings.Join(queryStrings(t, ctx, db, query), " "); got != "2:3" {
		t.Fatalf("lookup join rows %q, want %q", got, "2:3")
	}
	query = "SELECT outer_keys.id FROM outer_keys JOIN inner_keys ON inner_keys.k <=> outer_keys.k ORDER BY outer_keys.id, inner_keys.id"
	if got := strings.Join(queryStrings(t, ctx, db, query), " "); got != "1 1 2" {
		t.Fatalf("null-safe join outer ids %q, want %q", got, "1 1 2")
	}
}

// TestFailedAlterKeepsTable leaves rows and indexes untouched when a rewrite fails.
func TestFailedAlterKeepsTable(t *testing.T) {
	for _, test := range []struct {
		// name is the failure kind and the suffix of the table it alters.
		name string
		// rows are the stored (id, v) tuples.
		rows string
		// alter is the statement that must fail.
		alter string
		// probe is the value of row 2, found through the unique index.
		probe string
	}{
		{"narrow", "(1, 'one'), (2, 'two'), (3, 'long-value')", "MODIFY COLUMN v VARCHAR(4)", "two"},
		{"unique conflict", "(1, '1'), (2, '01')", "MODIFY COLUMN v BIGINT", "01"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Create an indexed table whose rewrite the statement cannot complete.
			ctx := t.Context()
			_, db := newMysqlTestDB(t, ctx, "probe")
			defer db.Close()
			execMysqlStatements(t, ctx, db,
				"CREATE TABLE altered (id BIGINT PRIMARY KEY, v VARCHAR(16))",
				"CREATE UNIQUE INDEX idx_v ON altered(v)",
				"INSERT INTO altered VALUES "+test.rows,
			)
			before := queryStrings(t, ctx, db, "SELECT CONCAT(id, ':', v) FROM altered ORDER BY id")
			if _, err := db.ExecContext(ctx, "ALTER TABLE altered "+test.alter); err == nil {
				t.Fatal("accepted a failing alter")
			}

			// The original rows, column type and unique index must still serve queries.
			if got := queryStrings(t, ctx, db, "SELECT CONCAT(id, ':', v) FROM altered ORDER BY id"); !slices.Equal(got, before) {
				t.Fatalf("rows after failed alter %v, want %v", got, before)
			}
			if got := queryStrings(t, ctx, db, "SELECT id FROM altered WHERE v = ?", test.probe); !slices.Equal(got, []string{"2"}) {
				t.Fatalf("lookup of %q returned %v, want [2]", test.probe, got)
			}
			if _, err := db.ExecContext(ctx, "INSERT INTO altered VALUES (9, ?)", test.probe); err == nil {
				t.Fatal("unique index no longer rejects duplicates")
			}
			if _, err := db.ExecContext(ctx, "INSERT INTO altered VALUES (10, 'longer than four')"); err != nil {
				t.Fatalf("column type was narrowed: %v", err)
			}
		})
	}
}
