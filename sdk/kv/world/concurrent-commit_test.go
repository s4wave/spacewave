package s4wave_kv_world_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_rpc "github.com/s4wave/spacewave/db/kvtx/rpc"
	kvtx_rpc_client "github.com/s4wave/spacewave/db/kvtx/rpc/client"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

func TestWorldBackedStoreFirstCommitFromEmptyRootLands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	objectKey := "kv/empty-root-store"
	createEmptyKvStoreObject(t, ctx, tb.WorldState, objectKey)

	inv, cleanup, err := s4wave_kv_world.KvStoreFactory(
		ctx,
		logrus.NewEntry(logrus.New()),
		tb.Bus,
		tb.BusEngine,
		tb.WorldState,
		objectKey,
	)
	if err != nil {
		t.Fatalf("KvStoreFactory: %v", err)
	}
	t.Cleanup(cleanup)
	store := kvtx_rpc_client.NewStore(kvtx_rpc.NewSRPCKvtxClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv)))))

	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(write): %v", err)
	}
	if err := tx.Set(ctx, []byte("hello"), []byte("world")); err != nil {
		tx.Discard()
		t.Fatalf("Set: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		tx.Discard()
		t.Fatalf("first commit from empty root: %v", err)
	}
	tx.Discard()

	finalStore, closeFn := openWorldBackedStore(t, ctx, tb.WorldState, objectKey)
	defer closeFn()
	readTx, err := finalStore.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction(read): %v", err)
	}
	defer readTx.Discard()
	got, found, err := readTx.Get(ctx, []byte("hello"))
	if err != nil {
		t.Fatalf("Get(hello): %v", err)
	}
	if !found || string(got) != "world" {
		t.Fatalf("Get(hello) = %q, %v; want %q, true", got, found, "world")
	}
}

func TestWorldBackedStoreConcurrentCommitsLandInWorldObjectRoot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	objectKey := "kv/concurrent-store"
	createKvStoreObject(t, ctx, tb.WorldState, objectKey, true)

	stores := make([]kvtx.Store, 0, 2)
	for idx := range 2 {
		inv, cleanup, err := s4wave_kv_world.KvStoreFactory(
			ctx,
			logrus.NewEntry(logrus.New()),
			tb.Bus,
			tb.BusEngine,
			tb.WorldState,
			objectKey,
		)
		if err != nil {
			t.Fatalf("KvStoreFactory(%d): %v", idx, err)
		}
		t.Cleanup(cleanup)
		store := kvtx_rpc_client.NewStore(kvtx_rpc.NewSRPCKvtxClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv)))))
		stores = append(stores, store)
	}

	type result struct {
		client int
		key    string
		val    string
		err    error
	}
	const writesPerClient = 6
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
				key := "client-" + strconv.Itoa(clientIdx) + "/key-" + strconv.Itoa(writeIdx)
				val := "value-" + strconv.Itoa(clientIdx) + "-" + strconv.Itoa(writeIdx)
				tx, err := store.NewTransaction(ctx, true)
				if err != nil {
					results <- result{client: clientIdx, key: key, err: err}
					return
				}
				if err := tx.Set(ctx, []byte(key), []byte(val)); err != nil {
					tx.Discard()
					results <- result{client: clientIdx, key: key, err: err}
					return
				}
				if err := tx.Commit(ctx); err != nil {
					tx.Discard()
					results <- result{client: clientIdx, key: key, err: err}
					return
				}
				tx.Discard()
				results <- result{client: clientIdx, key: key, val: val}
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
		t.Fatalf("concurrent KV commits did not finish: %v", ctx.Err())
	}
	close(results)

	successes := make([]result, 0, len(stores)*writesPerClient)
	for res := range results {
		if res.err != nil {
			t.Fatalf("client %d commit %s: %v", res.client, res.key, res.err)
		}
		successes = append(successes, res)
	}
	if len(successes) != len(stores)*writesPerClient {
		t.Fatalf("successful commits = %d, want %d", len(successes), len(stores)*writesPerClient)
	}

	finalStore, cleanup := openWorldBackedStore(t, ctx, tb.WorldState, objectKey)
	defer cleanup()
	readTx, err := finalStore.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction(read): %v", err)
	}
	defer readTx.Discard()
	for _, res := range successes {
		got, found, err := readTx.Get(ctx, []byte(res.key))
		if err != nil {
			t.Fatalf("Get(%s): %v", res.key, err)
		}
		if !found || string(got) != res.val {
			t.Fatalf("Get(%s) = %q, %v; want %q, true", res.key, got, found, res.val)
		}
	}
}
