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
	// Start a testbed with a World for the RPC-backed KV store.
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Create a typed KV object and retain its initial World root.
	objectKey := "kv/test-store"
	beforeRoot := createKvStoreObject(t, ctx, tb.WorldState, objectKey, true)

	// Open the KV RPC service for the World object.
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

	// Open an RPC write transaction for the initial KV records.
	store := kvtx_rpc_client.NewStore(kvtx_rpc.NewSRPCKvtxClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv)))))
	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(write): %v", err)
	}

	// Write the sample KV records through the RPC transaction.
	for _, kv := range []struct {
		key string
		val string
	}{
		{"alpha", "one"},
		{"beta", "two"},
		{"gamma", "three"},
	} {
		// Store the sample key and value in the RPC write transaction.
		if err := writeTx.Set(ctx, []byte(kv.key), []byte(kv.val)); err != nil {
			writeTx.Discard()
			t.Fatalf("Set(%s): %v", kv.key, err)
		}
	}

	// Commit the RPC transaction to advance the World root.
	if err := writeTx.Commit(ctx); err != nil {
		writeTx.Discard()
		t.Fatalf("Commit: %v", err)
	}
	writeTx.Discard()

	// Open a read transaction to inspect the committed KV records.
	readTx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction(read): %v", err)
	}
	defer readTx.Discard()

	// Verify each sample KV record through the RPC read transaction.
	for _, kv := range []struct {
		key string
		val string
	}{
		{"alpha", "one"},
		{"beta", "two"},
		{"gamma", "three"},
	} {
		// Read the committed sample value from the RPC transaction.
		got, found, err := readTx.Get(ctx, []byte(kv.key))
		if err != nil {
			t.Fatalf("Get(%s): %v", kv.key, err)
		}

		// Require the committed KV record to retain its sample value.
		if !found || string(got) != kv.val {
			t.Fatalf("Get(%s) = %q, %v; want %q, true", kv.key, got, found, kv.val)
		}
	}

	// Require the World object root to advance after the KV commit.
	afterRoot := getObjectRoot(t, ctx, tb.WorldState, objectKey)
	if beforeRoot.EqualsRef(afterRoot) {
		t.Fatal("world object root did not advance")
	}

	// Resolve the registered KV root operation for replay.
	lookupOp, err := optypes.LookupWorldOp(ctx, s4wave_kv_world.KvSetRootOpId)
	if err != nil {
		t.Fatalf("LookupWorldOp(%s): %v", s4wave_kv_world.KvSetRootOpId, err)
	}
	if _, ok := lookupOp.(*s4wave_kv_world.KvSetRootOp); !ok {
		t.Fatalf("LookupWorldOp returned %T, want *KvSetRootOp", lookupOp)
	}

	// Encode and decode the committed KV root for operation replay.
	op := s4wave_kv_world.NewKvSetRootOp(objectKey, afterRoot, afterRoot, nil)
	data, err := op.MarshalBlock()
	if err != nil {
		t.Fatalf("MarshalBlock: %v", err)
	}
	if err := lookupOp.UnmarshalBlock(data); err != nil {
		t.Fatalf("UnmarshalBlock: %v", err)
	}

	// Require the decoded operation to preserve the committed World root.
	if !lookupOp.(*s4wave_kv_world.KvSetRootOp).GetRootRef().EqualsRef(afterRoot) {
		t.Fatal("replayed KvSetRootOp root ref did not round-trip")
	}

	// Replay the decoded KV root operation through the World.
	if _, sysErr, err := tb.WorldState.ApplyWorldOp(ctx, lookupOp, ""); err != nil || sysErr {
		t.Fatalf("replay ApplyWorldOp sysErr=%v err=%v", sysErr, err)
	}
}

func TestKvStoreFactoryWatchStreamsCommittedSetAndDeleteSnapshots(t *testing.T) {
	// Start a testbed with a World for the KV watch.
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Create the typed World object that the KV watch observes.
	objectKey := "kv/watch-store"
	createKvStoreObject(t, ctx, tb.WorldState, objectKey, true)

	// Open the KV RPC service for the watched World object.
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

	// Connect the KV store and watch client to the RPC service.
	rpcClient := kvtx_rpc.NewSRPCKvtxClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv))))
	store := kvtx_rpc_client.NewStore(rpcClient)

	// Open a cancelable RPC watch for the selected KV prefix.
	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()
	watch, err := rpcClient.Watch(watchCtx, &kvtx_rpc.KvtxWatchRequest{Prefix: []byte("watch/")})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer watch.Close()

	// Require the KV watch to deliver its initial empty snapshot.
	expectWatchSnapshot(t, watch, map[string]string{})

	// Commit a KV record through RPC to change the watched prefix.
	commitSetThroughRPC(t, ctx, store, "watch/alpha", "one")

	// Require the KV watch to deliver the committed record.
	expectWatchSnapshot(t, watch, map[string]string{
		"watch/alpha": "one",
	})

	// Delete the KV record through RPC to empty the watched prefix.
	commitDeleteThroughRPC(t, ctx, store, "watch/alpha")

	// Require the KV watch to deliver an empty snapshot after deletion.
	expectWatchSnapshot(t, watch, map[string]string{})
}

func TestWorldBackedStoreReportsCommitPersistedWhenWorldRootUpdateFails(t *testing.T) {
	// Start a testbed with a World for a rejected KV root update.
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Open an untyped World object whose KV root operation will fail.
	objectKey := "kv/untyped-store"
	beforeRoot := createKvStoreObject(t, ctx, tb.WorldState, objectKey, false)
	store, cleanup := openWorldBackedStore(t, ctx, tb.WorldState, objectKey)
	defer cleanup()

	// Open a KV write transaction on the untyped World object.
	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(write): %v", err)
	}

	// Write a KV record before attempting the rejected World update.
	if err := writeTx.Set(ctx, []byte("persisted"), []byte("inner-root")); err != nil {
		writeTx.Discard()
		t.Fatalf("Set: %v", err)
	}

	// Require the KV commit to report persisted data after the World failure.
	err = writeTx.Commit(ctx)
	if !errors.Is(err, s4wave_kv_world.ErrCommitPersisted) {
		t.Fatalf("Commit error = %v, want ErrCommitPersisted", err)
	}
	writeTx.Discard()

	// Open a read transaction on the persisted inner KV root.
	readTx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction(read): %v", err)
	}
	defer readTx.Discard()

	// Require the inner KV store to retain the record after the World failure.
	got, found, err := readTx.Get(ctx, []byte("persisted"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found || string(got) != "inner-root" {
		t.Fatalf("inner store value = %q, %v; want inner-root, true", got, found)
	}

	// Require the failed KV operation to leave the World root unchanged.
	afterRoot := getObjectRoot(t, ctx, tb.WorldState, objectKey)
	if !beforeRoot.EqualsRef(afterRoot) {
		t.Fatal("world object root advanced despite failed ApplyWorldOp")
	}
}

func expectWatchSnapshot(t *testing.T, watch kvtx_rpc.SRPCKvtx_WatchClient, want map[string]string) {
	// Receive the next KV watch snapshot and require a successful response.
	t.Helper()
	resp, err := watch.Recv()
	if err != nil {
		t.Fatalf("Watch Recv: %v", err)
	}
	if errStr := resp.GetError(); errStr != "" {
		t.Fatalf("Watch response error = %q", errStr)
	}

	// Collect KV watch entries while rejecting duplicate keys.
	got := make(map[string]string, len(resp.GetEntries()))
	for _, entry := range resp.GetEntries() {
		key := string(entry.GetKey())
		if _, ok := got[key]; ok {
			t.Fatalf("Watch snapshot has duplicate key %q", key)
		}
		got[key] = string(entry.GetValue())
	}

	// Require the KV snapshot to contain exactly the expected records.
	if len(got) != len(want) {
		t.Fatalf("Watch snapshot = %v, want %v", got, want)
	}
	for key, wantValue := range want {
		// Require the KV snapshot to contain the expected key and value.
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
	// Open an RPC write transaction for the requested KV record.
	t.Helper()
	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(write): %v", err)
	}
	defer writeTx.Discard()

	// Write the requested KV record through the RPC transaction.
	if err := writeTx.Set(ctx, []byte(key), []byte(value)); err != nil {
		t.Fatalf("Set(%s): %v", key, err)
	}

	// Commit the RPC transaction containing the KV record.
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatalf("Commit set %s: %v", key, err)
	}
}

func commitDeleteThroughRPC(t *testing.T, ctx context.Context, store kvtx.Store, key string) {
	// Open an RPC write transaction for the requested KV deletion.
	t.Helper()
	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(write): %v", err)
	}
	defer writeTx.Discard()

	// Delete the requested KV record through the RPC transaction.
	if err := writeTx.Delete(ctx, []byte(key)); err != nil {
		t.Fatalf("Delete(%s): %v", key, err)
	}

	// Commit the RPC transaction containing the KV deletion.
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatalf("Commit delete %s: %v", key, err)
	}
}

func createKvStoreObject(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string, setType bool) *bucket.ObjectRef {
	// Create the World object with an initial KV block root.
	t.Helper()
	createdObject, rootRef, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(kvtx_block.NewKeyValueStoreForWorkload(kvtx_block.WorkloadClassDefault), true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}

	// Assign the KV store type when the World object needs typed access.
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
	{
		createdObject, _, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
			return nil
		})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
		}
	}
	if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_kv_world.KvStoreTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}
}

func getObjectRoot(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string) *bucket.ObjectRef {
	// Retain the World object while inspecting its KV root.
	t.Helper()
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatalf("MustGetObject(%s): %v", objectKey, err)
	}

	// Read the current KV root from the retained World object.
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
	// Retain the World object while opening its KV store.
	t.Helper()
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatalf("MustGetObject(%s): %v", objectKey, err)
	}

	// Open a KV store against the retained World object root.
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
