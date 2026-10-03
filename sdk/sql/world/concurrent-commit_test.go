package s4wave_sql_world_test

import (
	"context"
	"database/sql/driver"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/bucket"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	sql_rpc_client "github.com/s4wave/spacewave/db/sql/rpc/client"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

func TestWorldBackedSqlFirstCommitFromEmptyRootLands(t *testing.T) {
	// Bound the empty-root SQL commit test with a cancelable context.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start the testbed that holds the SQL World object.
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Create a typed SQL object with an empty root.
	objectKey := "sql/empty-root-db"
	createEmptySqlDbObject(t, ctx, tb.WorldState, objectKey)

	// Open an SRPC SQL client for the empty-root World object.
	inv, cleanup, err := s4wave_sql_world.SqlDbFactory(
		ctx,
		logrus.NewEntry(logrus.New()),
		tb.Bus,
		tb.BusEngine,
		tb.WorldState,
		objectKey,
	)
	if err != nil {
		t.Fatalf("SqlDbFactory: %v", err)
	}
	t.Cleanup(cleanup)
	store := sql_rpc_client.NewStore(sql_rpc.NewSRPCSqlClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv)))))

	// Commit the quickstart database into the initially empty SQL object.
	rootTx := openSqlTx(t, ctx, store, true, "")
	execSql(t, ctx, rootTx, "CREATE DATABASE quickstart")
	commitSql(t, ctx, rootTx)

	// Commit the notes table and its first row.
	writeTx := openSqlTx(t, ctx, store, true, "/quickstart")
	execSql(t, ctx, writeTx, "CREATE TABLE notes (id BIGINT NOT NULL PRIMARY KEY, body TEXT NOT NULL)")
	execSql(t, ctx, writeTx, "INSERT INTO notes (id, body) VALUES (1, 'first')")
	commitSql(t, ctx, writeTx)

	// Reopen the World SQL store and verify the first row persisted.
	finalStore, closeFn := openWorldBackedSql(t, ctx, tb.WorldState, objectKey)
	defer closeFn()
	readTx := openSqlTx(t, ctx, finalStore, false, "/quickstart")
	defer readTx.Discard()
	if body := querySingleString(t, ctx, readTx, "SELECT body FROM notes WHERE id = 1"); body != "first" {
		t.Fatalf("SELECT body = %q, want first", body)
	}
}

func TestWorldBackedSqlFirstCommitFromEmptyObjectRefLands(t *testing.T) {
	// Bound the empty-reference SQL commit test with a cancelable context.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start the testbed that holds the SQL World object.
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Create and type the SQL object with an empty ObjectRef.
	objectKey := "sql/empty-object-ref-db"
	{
		createdObject, err := tb.WorldState.CreateObject(ctx, objectKey, &bucket.ObjectRef{})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatalf("CreateObject(%s): %v", objectKey, err)
		}
	}
	if err := world_types.SetObjectType(ctx, tb.WorldState, objectKey, s4wave_sql_world.SqlDbTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}

	// Open an SRPC SQL client for the empty-reference World object.
	inv, cleanup, err := s4wave_sql_world.SqlDbFactory(
		ctx,
		logrus.NewEntry(logrus.New()),
		tb.Bus,
		tb.BusEngine,
		tb.WorldState,
		objectKey,
	)
	if err != nil {
		t.Fatalf("SqlDbFactory: %v", err)
	}
	t.Cleanup(cleanup)
	store := sql_rpc_client.NewStore(sql_rpc.NewSRPCSqlClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv)))))

	// Commit the quickstart database into the initially empty SQL object.
	rootTx := openSqlTx(t, ctx, store, true, "")
	execSql(t, ctx, rootTx, "CREATE DATABASE quickstart")
	commitSql(t, ctx, rootTx)

	// Commit the notes table and its first row.
	writeTx := openSqlTx(t, ctx, store, true, "/quickstart")
	execSql(t, ctx, writeTx, "CREATE TABLE notes (id BIGINT NOT NULL PRIMARY KEY, body TEXT NOT NULL)")
	execSql(t, ctx, writeTx, "INSERT INTO notes (id, body) VALUES (1, 'first')")
	commitSql(t, ctx, writeTx)

	// Reopen the World SQL store and verify the first row persisted.
	finalStore, closeFn := openWorldBackedSql(t, ctx, tb.WorldState, objectKey)
	defer closeFn()
	readTx := openSqlTx(t, ctx, finalStore, false, "/quickstart")
	defer readTx.Discard()
	if body := querySingleString(t, ctx, readTx, "SELECT body FROM notes WHERE id = 1"); body != "first" {
		t.Fatalf("SELECT body = %q, want first", body)
	}
}

func TestWorldBackedSqlFirstCommitFromNilRootLands(t *testing.T) {
	// Bound the nil-root SQL commit test with a cancelable context.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start the testbed that holds the SQL World object.
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Create and type the SQL object with a nil root.
	objectKey := "sql/nil-root-db"
	{
		createdObject, err := tb.WorldState.CreateObject(ctx, objectKey, nil)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatalf("CreateObject(%s): %v", objectKey, err)
		}
	}
	if err := world_types.SetObjectType(ctx, tb.WorldState, objectKey, s4wave_sql_world.SqlDbTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}

	// Open an SRPC SQL client for the nil-root World object.
	inv, cleanup, err := s4wave_sql_world.SqlDbFactory(
		ctx,
		logrus.NewEntry(logrus.New()),
		tb.Bus,
		tb.BusEngine,
		tb.WorldState,
		objectKey,
	)
	if err != nil {
		t.Fatalf("SqlDbFactory: %v", err)
	}
	t.Cleanup(cleanup)
	store := sql_rpc_client.NewStore(sql_rpc.NewSRPCSqlClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv)))))

	// Commit the quickstart database into the initially empty SQL object.
	rootTx := openSqlTx(t, ctx, store, true, "")
	execSql(t, ctx, rootTx, "CREATE DATABASE quickstart")
	commitSql(t, ctx, rootTx)

	// Commit the notes table and its first row.
	writeTx := openSqlTx(t, ctx, store, true, "/quickstart")
	execSql(t, ctx, writeTx, "CREATE TABLE notes (id BIGINT NOT NULL PRIMARY KEY, body TEXT NOT NULL)")
	execSql(t, ctx, writeTx, "INSERT INTO notes (id, body) VALUES (1, 'first')")
	commitSql(t, ctx, writeTx)

	// Reopen the World SQL store and verify the first row persisted.
	finalStore, closeFn := openWorldBackedSql(t, ctx, tb.WorldState, objectKey)
	defer closeFn()
	readTx := openSqlTx(t, ctx, finalStore, false, "/quickstart")
	defer readTx.Discard()
	if body := querySingleString(t, ctx, readTx, "SELECT body FROM notes WHERE id = 1"); body != "first" {
		t.Fatalf("SELECT body = %q, want first", body)
	}
}

func TestWorldBackedSqlConcurrentCommitsLandInWorldObjectRoot(t *testing.T) {
	// Bound the concurrent SQL commit test with a cancelable context.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Start the testbed that holds the shared SQL World object.
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Create and seed the SQL object shared by the concurrent clients.
	objectKey := "sql/concurrent-db"
	createSqlDbObject(t, ctx, tb.WorldState, objectKey, true)
	seedConcurrentSqlTable(t, ctx, tb, objectKey)

	// Open independent SRPC SQL clients against the same World object.
	stores := make([]hydra_sql.SqlStore, 0, 2)
	for idx := range 2 {
		inv, cleanup, err := s4wave_sql_world.SqlDbFactory(
			ctx,
			logrus.NewEntry(logrus.New()),
			tb.Bus,
			tb.BusEngine,
			tb.WorldState,
			objectKey,
		)
		if err != nil {
			t.Fatalf("SqlDbFactory(%d): %v", idx, err)
		}
		t.Cleanup(cleanup)
		store := sql_rpc_client.NewStore(sql_rpc.NewSRPCSqlClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv)))))
		stores = append(stores, store)
	}

	// Prepare the concurrent SQL writer results and start barrier.
	type result struct {
		client int
		id     int
		name   string
		err    error
	}
	const writesPerClient = 4
	start := make(chan struct{})
	results := make(chan result, len(stores)*writesPerClient)
	var wg sync.WaitGroup

	// Start each SQL writer with its client and row identity.
	for clientIdx, store := range stores {
		for writeIdx := range writesPerClient {
			clientIdx := clientIdx
			writeIdx := writeIdx
			store := store
			wg.Go(func() {
				// Wait for the shared start barrier and open the client write transaction.
				<-start
				id := clientIdx*writesPerClient + writeIdx + 1
				name := "name-" + strconv.Itoa(id)
				tx, err := store.NewSqlTransaction(ctx, true, "/alpha")
				if err != nil {
					results <- result{client: clientIdx, id: id, err: err}
					return
				}

				// Obtain SQL operations for the client write transaction.
				ops, err := tx.GetSqlOps(ctx)
				if err != nil {
					tx.Discard()
					results <- result{client: clientIdx, id: id, err: err}
					return
				}

				// Insert the client row and discard its transaction on failure.
				_, err = ops.ExecContext(ctx, "INSERT INTO soak (id, name) VALUES (?, ?)", []driver.NamedValue{
					{Ordinal: 1, Value: int64(id)},
					{Ordinal: 2, Value: name},
				})
				if err != nil {
					tx.Discard()
					results <- result{client: clientIdx, id: id, err: err}
					return
				}

				// Commit the client row and publish the writer result.
				if err := tx.Commit(ctx); err != nil {
					tx.Discard()
					results <- result{client: clientIdx, id: id, err: err}
					return
				}
				tx.Discard()
				results <- result{client: clientIdx, id: id, name: name}
			})
		}
	}

	// Release the SQL writer barrier and wait for every result.
	close(start)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("concurrent SQL commits did not finish: %v", ctx.Err())
	}
	close(results)

	// Collect every SQL writer result and require each commit to succeed.
	successes := make([]result, 0, len(stores)*writesPerClient)
	for res := range results {
		if res.err != nil {
			t.Fatalf("client %d commit id %d: %v", res.client, res.id, res.err)
		}
		successes = append(successes, res)
	}
	if len(successes) != len(stores)*writesPerClient {
		t.Fatalf("successful commits = %d, want %d", len(successes), len(stores)*writesPerClient)
	}

	// Reopen the SQL World root and verify every committed row.
	finalStore, cleanup := openWorldBackedSql(t, ctx, tb.WorldState, objectKey)
	defer cleanup()
	readTx := openSqlTx(t, ctx, finalStore, false, "/alpha")
	defer readTx.Discard()
	for _, res := range successes {
		name := querySingleString(t, ctx, readTx, "SELECT name FROM soak WHERE id = "+strconv.Itoa(res.id))
		if name != res.name {
			t.Fatalf("id %d name = %q, want %q", res.id, name, res.name)
		}
	}
}

func seedConcurrentSqlTable(t *testing.T, ctx context.Context, tb *testbed.Testbed, objectKey string) {
	// Open an SRPC SQL client for seeding the concurrency fixture.
	t.Helper()
	inv, cleanup, err := s4wave_sql_world.SqlDbFactory(
		ctx,
		logrus.NewEntry(logrus.New()),
		tb.Bus,
		tb.BusEngine,
		tb.WorldState,
		objectKey,
	)
	if err != nil {
		t.Fatalf("SqlDbFactory(seed): %v", err)
	}
	defer cleanup()

	// Commit the alpha database for the concurrent writers.
	store := sql_rpc_client.NewStore(sql_rpc.NewSRPCSqlClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv)))))
	rootTx := openSqlTx(t, ctx, store, true, "")
	execSql(t, ctx, rootTx, "CREATE DATABASE alpha")
	commitSql(t, ctx, rootTx)

	// Commit the soak table for the concurrent inserts.
	writeTx := openSqlTx(t, ctx, store, true, "/alpha")
	execSql(t, ctx, writeTx, "CREATE TABLE soak (id BIGINT NOT NULL PRIMARY KEY, name TEXT NOT NULL)")
	commitSql(t, ctx, writeTx)
}
