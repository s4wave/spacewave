//go:build !js

package resource_world

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	sdk_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	"github.com/sirupsen/logrus"
)

// TestTypedObjectResourceDisposesFactoryResult checks successful results across mount and child retirement.
func TestTypedObjectResourceDisposesFactoryResult(t *testing.T) {
	// Exercise Close's enumeration and both ways a child can retire before enumeration.
	for _, retirement := range []string{"mount-close", "canceled-before-enumeration", "rejected-registration"} {
		t.Run(retirement, func(t *testing.T) {
			// Start the in-memory World and its real controller bus.
			ctx := t.Context()
			tb, err := world_testbed.Default(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tb.Release)

			// Create the World object used by the layout factory.
			const typeID = "test/late-factory"
			const objectKey = "late-factory/object"
			tx, err := tb.Engine.NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			obj, err := tx.CreateObject(ctx, objectKey, nil)
			world.ReleaseObjectState(obj)
			if err != nil {
				t.Fatal(err)
			}

			// Commit the type assignment that selects the gated factory.
			if err := world_types.SetObjectType(ctx, tx, objectKey, typeID); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}

			// Acquire a real invoker before delaying the factory's successful return.
			entered := make(chan struct{})
			returnFactory := make(chan struct{})
			released := make(chan struct{}, 1)
			var cleanups atomic.Int32
			objType := objecttype.NewObjectType(typeID, func(
				ctx context.Context,
				le *logrus.Entry,
				b bus.Bus,
				engine world.Engine,
				ws world.WorldState,
				key string,
			) (srpc.Invoker, func(), error) {
				// Retain the layout's real watch and release callback before cancellation.
				invoker, cleanup, err := sdk_layout_world.ObjectLayoutFactory(ctx, le, b, engine, ws, key)
				if err != nil {
					return nil, nil, err
				}

				// Return the acquired layout after the chosen retirement event.
				close(entered)
				switch retirement {
				case "mount-close":
					<-ctx.Done()
				default:
					<-returnFactory
				}
				return invoker, func() {
					cleanup()
					cleanups.Add(1)
					released <- struct{}{}
				}, nil
			})

			// Resolve the gated factory through the production controller bus.
			controller := objecttype_controller.NewController(func(_ context.Context, id string) (objecttype.ObjectType, error) {
				if id == typeID {
					return objType, nil
				}
				return nil, nil
			})
			releaseController, err := tb.Bus.AddController(ctx, controller, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(releaseController)

			// Keep cancellation separate from Close to force key removal before enumeration.
			lifecycleCtx, cancel := context.WithCancel(ctx)
			t.Cleanup(cancel)
			mount := NewTypedObjectResourceWithContext(lifecycleCtx, tb.Logger, tb.Bus, world.NewEngineWorldState(tb.Engine, true), tb.Engine)
			t.Cleanup(mount.Close)
			t.Cleanup(func() {
				select {
				case <-returnFactory:
				default:
					close(returnFactory)
				}
			})
			mux := srpc.NewMux()
			if err := sdk_world.SRPCRegisterTypedObjectResourceService(mux, mount); err != nil {
				t.Fatal(err)
			}

			// Capture the actual invocation scope without changing child registration.
			invoked := make(chan resource_server.ResourceClientContext, 1)
			handled := make(chan error, 1)
			parent := srpc.InvokerFunc(func(serviceID, methodID string, stream srpc.Stream) (bool, error) {
				invoked <- resource_server.GetResourceClientContext(stream.Context())
				ok, err := mux.InvokeMethod(serviceID, methodID, stream)
				handled <- err
				return ok, err
			})

			// Create a releasable parent while retaining the connection's permanent root.
			root := srpc.InvokerFunc(func(_, _ string, stream srpc.Stream) (bool, error) {
				// Consume the client's request for a typed mount.
				if err := stream.MsgRecv(new(sdk_world.AccessTypedObjectRequest)); err != nil {
					return true, err
				}

				// Leave mount Close explicit so the test controls keyed enumeration.
				resourceCtx, err := resource_server.MustGetResourceClientContext(stream.Context())
				if err != nil {
					return true, err
				}
				id, err := resourceCtx.AddResource(parent, nil)
				if err != nil {
					return true, err
				}

				// Transfer the parent reference through the actual Resource response.
				return true, stream.MsgSend(&sdk_world.AccessTypedObjectResponse{ResourceId: id})
			})

			// Serve the typed mount through the production Resource protocol.
			server := resource_server.NewResourceServer(root)
			service := srpc.NewMux()
			if err := server.Register(service); err != nil {
				t.Fatal(err)
			}

			// Keep the client generation open throughout factory disposal.
			resources, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(service)))))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(resources.Release)

			// Retain the root reference and its baseline server resource count.
			rootRef := resources.AccessRootResource()
			t.Cleanup(rootRef.Release)
			rootRPC, err := rootRef.GetClient()
			if err != nil {
				t.Fatal(err)
			}

			// Adopt the parent whose retirement must reject a later typed child.
			parentResponse, err := sdk_world.NewSRPCTypedObjectResourceServiceClient(rootRPC).AccessTypedObject(ctx, &sdk_world.AccessTypedObjectRequest{})
			if err != nil {
				t.Fatal(err)
			}
			parentRef := resources.CreateResourceReference(parentResponse.GetResourceId())
			t.Cleanup(parentRef.Release)
			rpc, err := parentRef.GetClient()
			if err != nil {
				t.Fatal(err)
			}
			baseline := server.CountTrackedResources()

			// Enter the factory through the real Resource client and server connection.
			acquired := make(chan error, 1)
			go func() {
				response, err := sdk_world.AccessTypedObject(ctx, sdk_world.NewSRPCTypedObjectResourceServiceClient(rpc), &sdk_world.AccessTypedObjectRequest{ObjectKey: objectKey})
				if response != nil && response.GetResourceId() != 0 {
					resources.CreateResourceReference(response.GetResourceId()).Release()
				}
				acquired <- err
			}()
			resourceCtx := <-invoked
			<-entered

			// Retire demand before the factory returns its successfully acquired invoker.
			wantError := context.Canceled.Error()
			switch retirement {
			case "mount-close":
				wantError = ""
				mount.Close()
			case "canceled-before-enumeration":
				cancel()
				fallthrough
			case "rejected-registration":
				if !resourceCtx.ReleaseResource(parentRef.GetResourceID()) {
					t.Fatal("invocation parent was not released")
				}
				baseline--
				close(returnFactory)
			}

			// Join the server method even when parent retirement cancels the client call first.
			if retirement == "rejected-registration" {
				wantError = ""
			}
			servedError := ""
			if err := <-handled; err != nil {
				servedError = err.Error()
			}
			if retirement == "rejected-registration" && servedError == context.Canceled.Error() {
				// A processed release may cancel the stream before its typed response.
				if _, err := parentRef.GetClient(); !errors.Is(err, resource.ErrResourceOrClientReleased) {
					t.Fatalf("canceled acquisition retained a live parent: %v", err)
				}
				wantError = context.Canceled.Error()
			}
			if servedError != wantError {
				t.Fatalf("served AccessTypedObject error = %s, want %s", servedError, wantError)
			}
			if err := <-acquired; err == nil {
				t.Fatal("retired factory result was published to the client")
			}

			// Require removed-key disposal before Close can enumerate any handle.
			if keys := mount.objects.GetKeys(); len(keys) != 0 {
				t.Fatalf("retired typed keys = %v, want none", keys)
			}
			<-released
			if got := cleanups.Load(); got != 1 {
				t.Fatalf("factory cleanups before final Close = %d, want 1", got)
			}

			// Repeat mount teardown after the key has already disappeared.
			mount.Close()
			mount.Close()
			if got := cleanups.Load(); got != 1 {
				t.Fatalf("factory cleanups after repeated Close = %d, want 1", got)
			}

			// Observe the server's resource barrier while the client generation remains live.
			if got := server.WaitTrackedResourceCount(ctx, baseline); got != baseline {
				t.Fatalf("tracked resources = %d, want %d", got, baseline)
			}
			if err := resourceCtx.Context().Err(); err != nil {
				t.Fatalf("Resource connection retired: %v", err)
			}
		})
	}
}
