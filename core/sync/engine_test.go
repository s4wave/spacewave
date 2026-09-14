//go:build !js && !bldr_sqlite

package sync_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	core_sync "github.com/s4wave/spacewave/core/sync"
	"github.com/s4wave/spacewave/db/block"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_rpc "github.com/s4wave/spacewave/db/kvtx/rpc"
	kvtx_rpc_client "github.com/s4wave/spacewave/db/kvtx/rpc/client"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

func TestEngineReopensWorldCollectionAndDetachesResource(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	backing := storage_native.NewBoltDB(false, t.TempDir())
	engine, err := core_sync.Open(ctx, le, backing)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	{
		createdObject, _, err := world.CreateWorldObject(ctx, engine.State, "todos", func(cursor *block.Cursor) error {
			cursor.SetBlock(kvtx_block.NewKeyValueStoreForWorkload(kvtx_block.WorkloadClassDefault), true)
			return nil
		})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := world_types.SetObjectType(ctx, engine.State, "todos", kv_world.KvStoreTypeID); err != nil {
		t.Fatal(err)
	}
	mux, release, err := core_sync.AttachEngine(le, engine.Bus, engine.World)
	if err != nil {
		t.Fatal(err)
	}
	client, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))))
	if err != nil {
		t.Fatal(err)
	}
	remote, err := sdk_world.NewEngine(client, client.AccessRootResource())
	if err != nil {
		t.Fatal(err)
	}
	rootClient, err := remote.GetResourceRef().GetClient()
	if err != nil {
		t.Fatal(err)
	}
	typed, err := sdk_world.NewSRPCTypedObjectResourceServiceClient(rootClient).AccessTypedObject(ctx, &sdk_world.AccessTypedObjectRequest{ObjectKey: "todos"})
	if err != nil {
		t.Fatal(err)
	}
	kvRef := client.CreateResourceReference(typed.ResourceId)
	kvClient, err := kvRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	store := kvtx_rpc_client.NewStore(kvtx_rpc.NewSRPCKvtxClient(kvClient))
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("first"), []byte("ship the RC")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx.Discard()
	kvRef.Release()
	remote.Release()
	client.Release()
	select {
	case <-client.Done():
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	release()
	if _, err := engine.World.GetSeqno(ctx); err != nil {
		t.Fatalf("detaching stopped supplied engine: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := core_sync.Open(ctx, le, backing)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	invoker, closeStore, err := kv_world.KvStoreFactory(ctx, le, reopened.Bus, reopened.World, reopened.State, "todos")
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()
	reader := kvtx_rpc_client.NewStore(kvtx_rpc.NewSRPCKvtxClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker)))))
	read, err := reader.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	value, exists, err := read.Get(ctx, []byte("first"))
	if err != nil || !exists || string(value) != "ship the RC" {
		t.Fatalf("reopened record: %q, %v, %v", value, exists, err)
	}
}
