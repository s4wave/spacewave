//go:build !js

package resource_world_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	"github.com/sirupsen/logrus"
)

// TestTypedObjectResourceCloseCancelsBlockedFactory checks mount teardown during unary acquisition.
func TestTypedObjectResourceCloseCancelsBlockedFactory(t *testing.T) {
	// Block a real typed factory until its granting mount cancels the lifecycle.
	f := newTypedWatchFixture(t, "close-factory")
	const typeID = "test/blocked-factory"
	const objectKey = "blocked-factory/object"
	entered := make(chan struct{})
	objType := objecttype.NewObjectType(typeID, func(
		ctx context.Context,
		_ *logrus.Entry,
		_ bus.Bus,
		_ world.Engine,
		_ world.WorldState,
		_ string,
	) (srpc.Invoker, func(), error) {
		// Signal factory entry before waiting for mount cancellation.
		close(entered)
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})

	// Resolve the object's type through the production controller bus.
	controller := objecttype_controller.NewController(func(_ context.Context, id string) (objecttype.ObjectType, error) {
		if id == typeID {
			return objType, nil
		}
		return nil, nil
	})
	release, err := f.tb.Bus.AddController(t.Context(), controller, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	f.setType(t, objectKey, typeID, false)
	baseline := f.server.CountTrackedResources()

	// Enter unary acquisition over the actual Resource connection.
	typed := sdk_world.NewSRPCTypedObjectResourceServiceClient(f.rpc)
	acquired := make(chan error, 1)
	go func() {
		_, err := typed.AccessTypedObject(t.Context(), &sdk_world.AccessTypedObjectRequest{ObjectKey: objectKey})
		acquired <- err
	}()
	<-entered

	// Close the mount while acquisition holds the keyed factory lock.
	closed := make(chan struct{})
	go func() {
		f.mount.Close()
		close(closed)
	}()
	<-closed

	// Require cancellation to cross RPC without allocating a typed child.
	if err := <-acquired; err == nil || err.Error() != context.Canceled.Error() {
		t.Fatalf("AccessTypedObject error = %v, want context canceled", err)
	}
	f.count(t, baseline)
}
