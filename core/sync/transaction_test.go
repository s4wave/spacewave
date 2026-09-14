//go:build !js && !bldr_sqlite

package sync_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	core_sync "github.com/s4wave/spacewave/core/sync"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_rpc "github.com/s4wave/spacewave/db/kvtx/rpc"
	kvtx_rpc_client "github.com/s4wave/spacewave/db/kvtx/rpc/client"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// TestTransaction proves a supplied World transaction publishes KvStoreFactory
// writes atomically: discarded creates vanish, committed creates become visible
// together through a separate Resource consumer and survive Sync, Close, reopen.
func TestTransaction(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	backing := storage_native.NewBoltDB(false, t.TempDir())
	engine, err := core_sync.Open(ctx, le, backing)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	t.Run("DiscardRollsBackSuppliedWorldTransaction", func(t *testing.T) {
		// Create two kv/store objects and commit one KV write to each inside
		// the supplied World transaction.
		wtx, err := engine.World.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for _, objectKey := range []string{"kv/tx-rb-alpha", "kv/tx-rb-beta"} {
			if err := createKvStoreObjectIn(ctx, wtx, objectKey); err != nil {
				wtx.Discard()
				t.Fatalf("create %s: %v", objectKey, err)
			}
		}
		for _, objectKey := range []string{"kv/tx-rb-alpha", "kv/tx-rb-beta"} {
			if err := commitKvThroughFactory(ctx, le, engine.Bus, engine.World, wtx, objectKey, "draft", "lost"); err != nil {
				wtx.Discard()
				t.Fatalf("write %s: %v", objectKey, err)
			}
		}

		// Discard the World transaction and require every create and write to
		// be absent from the published engine state.
		wtx.Discard()
		for _, objectKey := range []string{"kv/tx-rb-alpha", "kv/tx-rb-beta"} {
			obj, found, err := engine.State.GetObject(ctx, objectKey)
			if err != nil {
				t.Fatalf("get %s: %v", objectKey, err)
			}
			if found {
				world.ReleaseObjectState(obj)
				t.Fatalf("discarded create %s is visible in engine state", objectKey)
			}
		}
	})

	t.Run("CommitPublishesSuppliedWorldTransaction", func(t *testing.T) {
		// Create two kv/store objects and one internal receipt store inside the
		// supplied World transaction.
		wtx, err := engine.World.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for _, objectKey := range []string{"kv/tx-alpha", "kv/tx-beta", "kv/tx-receipts"} {
			if err := createKvStoreObjectIn(ctx, wtx, objectKey); err != nil {
				wtx.Discard()
				t.Fatalf("create %s: %v", objectKey, err)
			}
		}

		// Commit the KV roots sequentially; they share the transaction storage writer.
		for _, want := range []struct {
			objectKey, key, value string
		}{
			{"kv/tx-alpha", "alpha", "one"},
			{"kv/tx-beta", "beta", "two"},
			{"kv/tx-receipts", "receipt/op-1", "accepted"},
		} {
			if err := commitKvThroughFactory(ctx, le, engine.Bus, engine.World, wtx, want.objectKey, want.key, want.value); err != nil {
				wtx.Discard()
				t.Fatalf("write %s: %v", want.objectKey, err)
			}
		}
		if err := wtx.Commit(ctx); err != nil {
			wtx.Discard()
			t.Fatalf("commit world tx: %v", err)
		}
		wtx.Discard()

		// Verify all three stores together through a separate Resource consumer.
		mux, release, err := core_sync.AttachEngine(le, engine.Bus, engine.World)
		if err != nil {
			t.Fatal(err)
		}
		client, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))))
		if err != nil {
			release()
			t.Fatal(err)
		}
		remote, err := sdk_world.NewEngine(client, client.AccessRootResource())
		if err != nil {
			client.Release()
			release()
			t.Fatal(err)
		}
		rootClient, err := remote.GetResourceRef().GetClient()
		if err != nil {
			remote.Release()
			client.Release()
			release()
			t.Fatal(err)
		}
		for _, want := range []struct {
			objectKey, key, value string
		}{
			{"kv/tx-alpha", "alpha", "one"},
			{"kv/tx-beta", "beta", "two"},
			{"kv/tx-receipts", "receipt/op-1", "accepted"},
		} {
			store, kvRef, err := accessKvResource(ctx, client, rootClient, want.objectKey)
			if err != nil {
				t.Fatalf("access %s: %v", want.objectKey, err)
			}
			expectKvValue(t, ctx, store, want.key, want.value)
			kvRef.Release()
		}

		// Release the mount and require the supplied engine to stay usable.
		remote.Release()
		client.Release()
		select {
		case <-client.Done():
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		release()
		if _, err := engine.World.GetSeqno(ctx); err != nil {
			t.Fatalf("releasing the mount stopped the supplied engine: %v", err)
		}

		// Require the committed values to survive Sync, Close, and reopen.
		if _, err := engine.World.Sync(ctx); err != nil {
			t.Fatalf("sync: %v", err)
		}
		if err := engine.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := core_sync.Open(ctx, le, backing)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = reopened.Close() })
		for _, want := range []struct {
			objectKey, key, value string
		}{
			{"kv/tx-alpha", "alpha", "one"},
			{"kv/tx-beta", "beta", "two"},
			{"kv/tx-receipts", "receipt/op-1", "accepted"},
		} {
			invoker, closeStore, err := kv_world.KvStoreFactory(ctx, le, reopened.Bus, reopened.World, reopened.State, want.objectKey)
			if err != nil {
				t.Fatalf("reopen %s: %v", want.objectKey, err)
			}
			store := kvtx_rpc_client.NewStore(kvtx_rpc.NewSRPCKvtxClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker)))))
			expectKvValue(t, ctx, store, want.key, want.value)
			closeStore()
		}
	})
}

// createKvStoreObjectIn creates a typed kv/store object in the supplied World state.
func createKvStoreObjectIn(ctx context.Context, ws world.WorldState, objectKey string) error {
	{
		createdObject, _, err := world.CreateWorldObject(ctx, ws, objectKey, func(cursor *block.Cursor) error {
			cursor.SetBlock(kvtx_block.NewKeyValueStoreForWorkload(kvtx_block.WorkloadClassDefault), true)
			return nil
		})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			return err
		}
	}
	return world_types.SetObjectType(ctx, ws, objectKey, kv_world.KvStoreTypeID)
}

// commitKvThroughFactory opens the kv/store object through KvStoreFactory against
// the supplied World state and commits one key/value pair in one inner transaction.
func commitKvThroughFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey, key, value string,
) error {
	invoker, closeStore, err := kv_world.KvStoreFactory(ctx, le, b, engine, ws, objectKey)
	if err != nil {
		return err
	}
	defer closeStore()
	store := kvtx_rpc_client.NewStore(kvtx_rpc.NewSRPCKvtxClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker)))))
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	if err := tx.Set(ctx, []byte(key), []byte(value)); err != nil {
		tx.Discard()
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		tx.Discard()
		return err
	}
	tx.Discard()
	return nil
}

// accessKvResource opens a kv/store object through the mounted engine Resource
// and returns its KVTX RPC store with the reference the caller must release.
func accessKvResource(ctx context.Context, client *resource_client.Client, rootClient srpc.Client, objectKey string) (kvtx.Store, resource_client.ResourceRef, error) {
	typed, err := sdk_world.NewSRPCTypedObjectResourceServiceClient(rootClient).AccessTypedObject(ctx, &sdk_world.AccessTypedObjectRequest{ObjectKey: objectKey})
	if err != nil {
		return nil, nil, err
	}
	kvRef := client.CreateResourceReference(typed.ResourceId)
	kvClient, err := kvRef.GetClient()
	if err != nil {
		kvRef.Release()
		return nil, nil, err
	}
	return kvtx_rpc_client.NewStore(kvtx_rpc.NewSRPCKvtxClient(kvClient)), kvRef, nil
}

// expectKvValue reads one key through a read-only KVTX transaction and requires the exact value.
func expectKvValue(t *testing.T, ctx context.Context, store kvtx.Store, key, value string) {
	t.Helper()
	read, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	got, exists, err := read.Get(ctx, []byte(key))
	if err != nil {
		t.Fatal(err)
	}
	if !exists || string(got) != value {
		t.Fatalf("key %s = %q, %v; want %q, true", key, got, exists, value)
	}
}
