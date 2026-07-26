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
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

func TestWorldBackedSqlFirstCommitFromEmptyRootLands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	objectKey := "sql/empty-root-db"
	createEmptySqlDbObject(t, ctx, tb.WorldState, objectKey)

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

	rootTx := openSqlTx(t, ctx, store, true, "")
	execSql(t, ctx, rootTx, "CREATE DATABASE quickstart")
	commitSql(t, ctx, rootTx)

	writeTx := openSqlTx(t, ctx, store, true, "/quickstart")
	execSql(t, ctx, writeTx, "CREATE TABLE notes (id BIGINT NOT NULL PRIMARY KEY, body TEXT NOT NULL)")
	execSql(t, ctx, writeTx, "INSERT INTO notes (id, body) VALUES (1, 'first')")
	commitSql(t, ctx, writeTx)

	finalStore, closeFn := openWorldBackedSql(t, ctx, tb.WorldState, objectKey)
	defer closeFn()
	readTx := openSqlTx(t, ctx, finalStore, false, "/quickstart")
	defer readTx.Discard()
	if body := querySingleString(t, ctx, readTx, "SELECT body FROM notes WHERE id = 1"); body != "first" {
		t.Fatalf("SELECT body = %q, want first", body)
	}
}

func TestWorldBackedSqlFirstCommitFromEmptyObjectRefLands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	objectKey := "sql/empty-object-ref-db"
	if _, err := tb.WorldState.CreateObject(ctx, objectKey, &bucket.ObjectRef{}); err != nil {
		t.Fatalf("CreateObject(%s): %v", objectKey, err)
	}
	if err := world_types.SetObjectType(ctx, tb.WorldState, objectKey, s4wave_sql_world.SqlDbTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}

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

	rootTx := openSqlTx(t, ctx, store, true, "")
	execSql(t, ctx, rootTx, "CREATE DATABASE quickstart")
	commitSql(t, ctx, rootTx)

	writeTx := openSqlTx(t, ctx, store, true, "/quickstart")
	execSql(t, ctx, writeTx, "CREATE TABLE notes (id BIGINT NOT NULL PRIMARY KEY, body TEXT NOT NULL)")
	execSql(t, ctx, writeTx, "INSERT INTO notes (id, body) VALUES (1, 'first')")
	commitSql(t, ctx, writeTx)

	finalStore, closeFn := openWorldBackedSql(t, ctx, tb.WorldState, objectKey)
	defer closeFn()
	readTx := openSqlTx(t, ctx, finalStore, false, "/quickstart")
	defer readTx.Discard()
	if body := querySingleString(t, ctx, readTx, "SELECT body FROM notes WHERE id = 1"); body != "first" {
		t.Fatalf("SELECT body = %q, want first", body)
	}
}

func TestWorldBackedSqlFirstCommitFromNilRootLands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	objectKey := "sql/nil-root-db"
	if _, err := tb.WorldState.CreateObject(ctx, objectKey, nil); err != nil {
		t.Fatalf("CreateObject(%s): %v", objectKey, err)
	}
	if err := world_types.SetObjectType(ctx, tb.WorldState, objectKey, s4wave_sql_world.SqlDbTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}

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

	rootTx := openSqlTx(t, ctx, store, true, "")
	execSql(t, ctx, rootTx, "CREATE DATABASE quickstart")
	commitSql(t, ctx, rootTx)

	writeTx := openSqlTx(t, ctx, store, true, "/quickstart")
	execSql(t, ctx, writeTx, "CREATE TABLE notes (id BIGINT NOT NULL PRIMARY KEY, body TEXT NOT NULL)")
	execSql(t, ctx, writeTx, "INSERT INTO notes (id, body) VALUES (1, 'first')")
	commitSql(t, ctx, writeTx)

	finalStore, closeFn := openWorldBackedSql(t, ctx, tb.WorldState, objectKey)
	defer closeFn()
	readTx := openSqlTx(t, ctx, finalStore, false, "/quickstart")
	defer readTx.Discard()
	if body := querySingleString(t, ctx, readTx, "SELECT body FROM notes WHERE id = 1"); body != "first" {
		t.Fatalf("SELECT body = %q, want first", body)
	}
}

func TestWorldBackedSqlConcurrentCommitsLandInWorldObjectRoot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	objectKey := "sql/concurrent-db"
	createSqlDbObject(t, ctx, tb.WorldState, objectKey, true)
	seedConcurrentSqlTable(t, ctx, tb, objectKey)

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
	for clientIdx, store := range stores {
		for writeIdx := range writesPerClient {
			clientIdx := clientIdx
			writeIdx := writeIdx
			store := store
			wg.Go(func() {
				<-start
				id := clientIdx*writesPerClient + writeIdx + 1
				name := "name-" + strconv.Itoa(id)
				tx, err := store.NewSqlTransaction(ctx, true, "/alpha")
				if err != nil {
					results <- result{client: clientIdx, id: id, err: err}
					return
				}
				ops, err := tx.GetSqlOps(ctx)
				if err != nil {
					tx.Discard()
					results <- result{client: clientIdx, id: id, err: err}
					return
				}
				_, err = ops.ExecContext(ctx, "INSERT INTO soak (id, name) VALUES (?, ?)", []driver.NamedValue{
					{Ordinal: 1, Value: int64(id)},
					{Ordinal: 2, Value: name},
				})
				if err != nil {
					tx.Discard()
					results <- result{client: clientIdx, id: id, err: err}
					return
				}
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

	store := sql_rpc_client.NewStore(sql_rpc.NewSRPCSqlClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv)))))
	rootTx := openSqlTx(t, ctx, store, true, "")
	execSql(t, ctx, rootTx, "CREATE DATABASE alpha")
	commitSql(t, ctx, rootTx)

	writeTx := openSqlTx(t, ctx, store, true, "/alpha")
	execSql(t, ctx, writeTx, "CREATE TABLE soak (id BIGINT NOT NULL PRIMARY KEY, name TEXT NOT NULL)")
	commitSql(t, ctx, writeTx)
}
