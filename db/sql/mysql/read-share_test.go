package mysql

import (
	"context"
	"testing"

	sqle "github.com/dolthub/go-mysql-server"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// TestReadTransactionSharesDecodedBlocks checks that a read transaction takes
// the table's tree nodes from the decoded block cache without cloning them.
func TestReadTransactionSharesDecodedBlocks(t *testing.T) {
	// Commit a table with enough rows to span several tree nodes.
	ctx := t.Context()
	store := block_mock.NewMockStore(0)
	database := NewMysql(&indexTestRoot{store: store}, nil)
	runMysqlTx(t, ctx, database, true,
		"CREATE TABLE rows_t (id BIGINT PRIMARY KEY, name TEXT)",
		"INSERT INTO rows_t VALUES (1,'a'),(2,'b'),(3,'c'),(4,'d'),(5,'e'),(6,'f'),(7,'g'),(8,'h')",
	)

	// Scan the table twice in read transactions sharing one operation cache.
	opCtx, counter := block.WithReadCounter(ctx)
	opCtx = block.WithReadOperationStore(opCtx, store)
	for range 2 {
		runMysqlTx(t, opCtx, database, false, "SELECT * FROM rows_t")
	}

	// Verify the second scan hit the cache and nothing was cloned.
	snapshot := counter.Snapshot()
	if snapshot.DecodedBlockCacheHitCount == 0 || snapshot.DecodedBlockCloneCount != 0 {
		t.Fatalf("unexpected decoded cache counters: %+v", snapshot)
	}
}

// runMysqlTx runs statements in one transaction on the probe database and
// commits it when write is set.
func runMysqlTx(t *testing.T, ctx context.Context, database *Mysql, write bool, statements ...string) {
	// Open the transaction and its database.
	t.Helper()
	sqlCtx := sql.NewContext(ctx)
	sqlCtx.SetCurrentDatabase("probe")
	tx, err := database.NewMysqlTransaction(sqlCtx, write)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if _, err := tx.OpenDatabase(sqlCtx, "probe", write); err != nil {
		t.Fatal(err)
	}

	// Run the statements on an engine over the transaction's databases.
	provider, err := tx.BuildDatabaseProvider(sqlCtx)
	if err != nil {
		t.Fatal(err)
	}
	engine := sqle.NewDefault(provider)
	sqlCtx = engineContext(sqlCtx, engine)
	for _, statement := range statements {
		if _, err := queryIndexTest(sqlCtx, engine, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}

	// Commit a write transaction.
	if write {
		if err := tx.Commit(sqlCtx); err != nil {
			t.Fatal(err)
		}
	}
}
