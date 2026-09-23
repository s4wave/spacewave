//go:build !js

package resource_world_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// TestTypedObjectSnapshotPreservesReadAuthority proves typed access cannot escape its transaction.
func TestTypedObjectSnapshotPreservesReadAuthority(t *testing.T) {
	// Register a factory that exposes the supplied World's read-only contract.
	ctx := t.Context()
	tb, release := setupWorldTestbed(ctx, t)
	defer release()
	const typeID = "test/snapshot"
	const objectKey = "snapshot/object"
	snapshotType := objecttype.NewObjectType(typeID, func(
		ctx context.Context,
		le *logrus.Entry,
		b bus.Bus,
		engine world.Engine,
		ws world.WorldState,
		key string,
	) (srpc.Invoker, func(), error) {
		if !ws.GetReadOnly() {
			return nil, nil, fmt.Errorf("snapshot was promoted to a writable World")
		}
		obj, found, err := ws.GetObject(ctx, "snapshot/later")
		world.ReleaseObjectState(obj)
		if err != nil {
			return nil, nil, err
		}
		if found {
			return nil, nil, fmt.Errorf("snapshot observed a later accepted object")
		}
		return srpc.NewMux(), func() {}, nil
	})
	client, engine, cleanup := setupWorldResourceClientWithObjectTypesAndExtras(ctx, t, tb, map[string]objecttype.ObjectType{typeID: snapshotType})
	defer cleanup()
	engineRef := client.CreateResourceReference(engine.GetResourceRef().GetResourceID())
	worldEngine, err := sdk_world_engine.NewSDKEngine(client, engineRef)
	if err != nil {
		engineRef.Release()
		t.Fatal(err)
	}
	defer worldEngine.Release()

	// Open the snapshot before accepting another object in the live World.
	write, err := worldEngine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer write.Discard()
	obj, err := write.CreateObject(ctx, objectKey, nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, write, objectKey, typeID); err != nil {
		t.Fatal(err)
	}
	if err := write.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Release()
	later, err := worldEngine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer later.Discard()
	obj, err = later.CreateObject(ctx, "snapshot/later", nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := later.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Access the registered type through the real Resource RPC connection.
	rpc, err := snapshot.GetResourceRef().GetClient()
	if err != nil {
		t.Fatal(err)
	}
	typed := sdk_world.NewSRPCTypedObjectResourceServiceClient(rpc)
	access, err := typed.AccessTypedObject(ctx, &sdk_world.AccessTypedObjectRequest{ObjectKey: objectKey})
	if err != nil {
		t.Fatal(err)
	}
	client.CreateResourceReference(access.GetResourceId()).Release()
}
