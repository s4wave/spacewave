package s4wave_kv_world_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_rpc "github.com/s4wave/spacewave/db/kvtx/rpc"
	kvtx_rpc_client "github.com/s4wave/spacewave/db/kvtx/rpc/client"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

func TestKvStoreFactoryCommitsWorldBackedRootAndReplaysOp(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	objectKey := "kv/test-store"
	beforeRoot := createKvStoreObject(t, ctx, tb.WorldState, objectKey, true)

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
	defer cleanup()

	store := kvtx_rpc_client.NewStore(kvtx_rpc.NewSRPCKvtxClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv)))))
	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(write): %v", err)
	}
	for _, kv := range []struct {
		key string
		val string
	}{
		{"alpha", "one"},
		{"beta", "two"},
		{"gamma", "three"},
	} {
		if err := writeTx.Set(ctx, []byte(kv.key), []byte(kv.val)); err != nil {
			writeTx.Discard()
			t.Fatalf("Set(%s): %v", kv.key, err)
		}
	}
	if err := writeTx.Commit(ctx); err != nil {
		writeTx.Discard()
		t.Fatalf("Commit: %v", err)
	}
	writeTx.Discard()

	readTx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction(read): %v", err)
	}
	defer readTx.Discard()
	for _, kv := range []struct {
		key string
		val string
	}{
		{"alpha", "one"},
		{"beta", "two"},
		{"gamma", "three"},
	} {
		got, found, err := readTx.Get(ctx, []byte(kv.key))
		if err != nil {
			t.Fatalf("Get(%s): %v", kv.key, err)
		}
		if !found || string(got) != kv.val {
			t.Fatalf("Get(%s) = %q, %v; want %q, true", kv.key, got, found, kv.val)
		}
	}

	afterRoot := getObjectRoot(t, ctx, tb.WorldState, objectKey)
	if beforeRoot.EqualsRef(afterRoot) {
		t.Fatal("world object root did not advance")
	}

	lookupOp, err := optypes.LookupWorldOp(ctx, s4wave_kv_world.KvSetRootOpId)
	if err != nil {
		t.Fatalf("LookupWorldOp(%s): %v", s4wave_kv_world.KvSetRootOpId, err)
	}
	if _, ok := lookupOp.(*s4wave_kv_world.KvSetRootOp); !ok {
		t.Fatalf("LookupWorldOp returned %T, want *KvSetRootOp", lookupOp)
	}

	op := s4wave_kv_world.NewKvSetRootOp(objectKey, afterRoot, afterRoot, nil)
	data, err := op.MarshalBlock()
	if err != nil {
		t.Fatalf("MarshalBlock: %v", err)
	}
	if err := lookupOp.UnmarshalBlock(data); err != nil {
		t.Fatalf("UnmarshalBlock: %v", err)
	}
	if !lookupOp.(*s4wave_kv_world.KvSetRootOp).GetRootRef().EqualsRef(afterRoot) {
		t.Fatal("replayed KvSetRootOp root ref did not round-trip")
	}
	if _, sysErr, err := tb.WorldState.ApplyWorldOp(ctx, lookupOp, ""); err != nil || sysErr {
		t.Fatalf("replay ApplyWorldOp sysErr=%v err=%v", sysErr, err)
	}
}

func TestKvStoreFactoryWatchStreamsCommittedSetAndDeleteSnapshots(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	objectKey := "kv/watch-store"
	createKvStoreObject(t, ctx, tb.WorldState, objectKey, true)

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
	defer cleanup()

	rpcClient := kvtx_rpc.NewSRPCKvtxClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv))))
	store := kvtx_rpc_client.NewStore(rpcClient)

	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()
	watch, err := rpcClient.Watch(watchCtx, &kvtx_rpc.KvtxWatchRequest{Prefix: []byte("watch/")})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer watch.Close()

	expectWatchSnapshot(t, watch, map[string]string{})

	commitSetThroughRPC(t, ctx, store, "watch/alpha", "one")
	expectWatchSnapshot(t, watch, map[string]string{
		"watch/alpha": "one",
	})

	commitDeleteThroughRPC(t, ctx, store, "watch/alpha")
	expectWatchSnapshot(t, watch, map[string]string{})
}

func TestWorldBackedStoreReportsCommitPersistedWhenWorldRootUpdateFails(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	objectKey := "kv/untyped-store"
	beforeRoot := createKvStoreObject(t, ctx, tb.WorldState, objectKey, false)
	store, cleanup := openWorldBackedStore(t, ctx, tb.WorldState, objectKey)
	defer cleanup()

	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(write): %v", err)
	}
	if err := writeTx.Set(ctx, []byte("persisted"), []byte("inner-root")); err != nil {
		writeTx.Discard()
		t.Fatalf("Set: %v", err)
	}
	err = writeTx.Commit(ctx)
	if !errors.Is(err, s4wave_kv_world.ErrCommitPersisted) {
		t.Fatalf("Commit error = %v, want ErrCommitPersisted", err)
	}
	writeTx.Discard()

	readTx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction(read): %v", err)
	}
	defer readTx.Discard()
	got, found, err := readTx.Get(ctx, []byte("persisted"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found || string(got) != "inner-root" {
		t.Fatalf("inner store value = %q, %v; want inner-root, true", got, found)
	}

	afterRoot := getObjectRoot(t, ctx, tb.WorldState, objectKey)
	if !beforeRoot.EqualsRef(afterRoot) {
		t.Fatal("world object root advanced despite failed ApplyWorldOp")
	}
}

func expectWatchSnapshot(t *testing.T, watch kvtx_rpc.SRPCKvtx_WatchClient, want map[string]string) {
	t.Helper()
	resp, err := watch.Recv()
	if err != nil {
		t.Fatalf("Watch Recv: %v", err)
	}
	if errStr := resp.GetError(); errStr != "" {
		t.Fatalf("Watch response error = %q", errStr)
	}

	got := make(map[string]string, len(resp.GetEntries()))
	for _, entry := range resp.GetEntries() {
		key := string(entry.GetKey())
		if _, ok := got[key]; ok {
			t.Fatalf("Watch snapshot has duplicate key %q", key)
		}
		got[key] = string(entry.GetValue())
	}
	if len(got) != len(want) {
		t.Fatalf("Watch snapshot = %v, want %v", got, want)
	}
	for key, wantValue := range want {
		gotValue, ok := got[key]
		if !ok {
			t.Fatalf("Watch snapshot = %v, missing key %q", got, key)
		}
		if gotValue != wantValue {
			t.Fatalf("Watch snapshot[%q] = %q, want %q", key, gotValue, wantValue)
		}
	}
}

func commitSetThroughRPC(t *testing.T, ctx context.Context, store kvtx.Store, key, value string) {
	t.Helper()
	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(write): %v", err)
	}
	defer writeTx.Discard()
	if err := writeTx.Set(ctx, []byte(key), []byte(value)); err != nil {
		t.Fatalf("Set(%s): %v", key, err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatalf("Commit set %s: %v", key, err)
	}
}

func commitDeleteThroughRPC(t *testing.T, ctx context.Context, store kvtx.Store, key string) {
	t.Helper()
	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(write): %v", err)
	}
	defer writeTx.Discard()
	if err := writeTx.Delete(ctx, []byte(key)); err != nil {
		t.Fatalf("Delete(%s): %v", key, err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatalf("Commit delete %s: %v", key, err)
	}
}

func createKvStoreObject(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string, setType bool) *bucket.ObjectRef {
	t.Helper()
	_, rootRef, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(kvtx_block.NewKeyValueStoreForWorkload(kvtx_block.WorkloadClassDefault), true)
		return nil
	})
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}
	if setType {
		if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_kv_world.KvStoreTypeID); err != nil {
			t.Fatalf("SetObjectType(%s): %v", objectKey, err)
		}
	}
	return rootRef.Clone()
}

// createEmptyKvStoreObject creates a kv/store object with an empty initial root,
// mirroring the browser quickstart path that creates the object before any block
// is written, so the first commit advances the root from an empty base.
func createEmptyKvStoreObject(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string) {
	t.Helper()
	if _, _, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		return nil
	}); err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}
	if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_kv_world.KvStoreTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}
}

func getObjectRoot(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string) *bucket.ObjectRef {
	t.Helper()
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	if err != nil {
		t.Fatalf("MustGetObject(%s): %v", objectKey, err)
	}
	root, _, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatalf("GetRootRef(%s): %v", objectKey, err)
	}
	return root.Clone()
}

func openWorldBackedStore(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
) (kvtx.Store, func()) {
	t.Helper()
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	if err != nil {
		t.Fatalf("MustGetObject(%s): %v", objectKey, err)
	}
	var store *s4wave_kv_world.WorldBackedStore
	if err := obj.AccessWorldState(ctx, nil, func(root *bucket_lookup.Cursor) error {
		var err error
		store, err = s4wave_kv_world.NewWorldBackedStore(ctx, logrus.NewEntry(logrus.New()), root.Clone(), ws, objectKey)
		return err
	}); err != nil {
		t.Fatalf("AccessWorldState(%s): %v", objectKey, err)
	}
	return store, store.Close
}
