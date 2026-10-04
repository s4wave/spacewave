//go:build !js

package resource_world_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	"github.com/sirupsen/logrus"
)

// TestTypedObjectGrantRetirementPrecedesReleaseDelivery checks unary attribution
// through a real World watch while the exact grant's release is withheld.
func TestTypedObjectGrantRetirementPrecedesReleaseDelivery(t *testing.T) {
	// Exercise causal cancellation, late success and an independent child failure.
	for _, name := range []string{"factory-canceled", "late-success", "child-grant-retired"} {
		t.Run(name, func(t *testing.T) {
			// Hold the selected release on the actual ResourceClient stream.
			gate := &resourceReleaseSendGate{entered: make(chan struct{}), resume: make(chan struct{})}
			f := newTypedWatchFixture(t, "grant-retirement", func(stream srpc.Stream) srpc.Stream {
				gate.Stream = stream
				return gate
			})
			t.Cleanup(gate.release)

			// Block the typed factory until its exact granting mount retires.
			const typeID = "test/retiring-grant"
			const objectKey = "retiring-grant/object"
			entered := make(chan struct{})
			cause := make(chan error, 1)
			var cleanups atomic.Int32
			objType := objecttype.NewObjectType(typeID, func(ctx context.Context, _ *logrus.Entry, _ bus.Bus, _ world.Engine, _ world.WorldState, _ string) (srpc.Invoker, func(), error) {
				// Let exact mount retirement terminate the blocked factory.
				close(entered)
				<-ctx.Done()
				cause <- context.Cause(ctx)
				if name == "child-grant-retired" {
					return nil, nil, sdk_world.ErrTypedObjectGrantRetired
				}
				if name != "late-success" {
					return nil, nil, ctx.Err()
				}

				// Return an acquired invoker whose disposal must occur exactly once.
				return srpc.NewMux(), func() { cleanups.Add(1) }, nil
			})

			// Register the gated factory through the production controller bus.
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

			// Open the World watch that owns snapshot replacement and retirement.
			watch, err := sdk_world.NewSRPCWatchWorldStateResourceServiceClient(f.rpc).WatchWorldState(t.Context(), &sdk_world.WatchWorldStateRequest{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = watch.Close() })

			// Adopt its first snapshot as the exact grant for this acquisition.
			first, err := watch.Recv()
			if err != nil {
				t.Fatal(err)
			}
			grant := f.resources.CreateResourceReference(first.GetResourceId())
			t.Cleanup(grant.Release)
			gate.grantID.Store(grant.GetResourceID())
			rpc, err := grant.GetClient()
			if err != nil {
				t.Fatal(err)
			}

			// Enter acquisition under that exact granting snapshot.
			acquired := make(chan error, 1)
			go func() {
				_, err := sdk_world.AccessTypedObject(t.Context(), sdk_world.NewSRPCTypedObjectResourceServiceClient(rpc), &sdk_world.AccessTypedObjectRequest{ObjectKey: objectKey})
				acquired <- err
			}()
			waitTypedWatchEvent(t, entered)

			// Replace the tracked snapshot while its factory is still blocked.
			f.setType(t, objectKey, "test/replacement-grant", false)
			waitTypedWatchEvent(t, gate.entered)
			err = <-acquired
			if name == "child-grant-retired" {
				// A child's marker remains terminal even when the granting mount retires.
				if err == nil || errors.Is(err, sdk_world.ErrTypedObjectGrantRetired) || err.Error() != sdk_world.ErrTypedObjectGrantRetired.Error() {
					t.Fatalf("independent child retirement error = %v", err)
				}
			} else if !errors.Is(err, sdk_world.ErrTypedObjectGrantRetired) {
				t.Fatalf("AccessTypedObject error = %v, want ErrTypedObjectGrantRetired", err)
			}

			// Require authoritative retirement even though local controls report live.
			if _, err := grant.GetClient(); err != nil {
				t.Fatalf("release notification escaped the gate: %v", err)
			}
			if err := <-cause; !errors.Is(err, sdk_world.ErrTypedObjectGrantRetired) {
				t.Fatalf("factory context cause = %v, want ErrTypedObjectGrantRetired", err)
			}
			if err := t.Context().Err(); err != nil {
				t.Fatalf("caller canceled during grant retirement: %v", err)
			}

			// Preserve replacement-before-release on the same watch.
			replacement, err := watch.Recv()
			if err != nil {
				t.Fatal(err)
			}
			if replacement.GetResourceId() == first.GetResourceId() {
				t.Fatal("World watch reused the retired granting snapshot")
			}
			gate.release()
			f.resources.CreateResourceReference(replacement.GetResourceId()).Release()
			grant.Release()
			_ = watch.Close()

			// Join cleanup through the server's resource barrier with the client open.
			f.count(t, baseline)
			f.mount.Close()
			wantCleanups := int32(0)
			if name == "late-success" {
				wantCleanups = 1
			}
			if got := cleanups.Load(); got != wantCleanups {
				t.Fatalf("factory cleanups = %d, want %d", got, wantCleanups)
			}
		})
	}
}

// TestTypedObjectAcquisitionTerminalCancellation keeps healthy factory failure,
// caller cancellation and Resource client cancellation distinct from retirement.
func TestTypedObjectAcquisitionTerminalCancellation(t *testing.T) {
	// Keep the granting mount healthy while each independent failure terminates acquisition.
	for _, failure := range []string{"factory-canceled", "factory-error", "child-grant-retired", "caller-canceled", "client-released"} {
		t.Run(failure, func(t *testing.T) {
			// Keep an actual engine mount and Resource connection open.
			f := newTypedWatchFixture(t, "terminal-acquisition")

			// Gate a registered factory without depending on grant retirement.
			const typeID = "test/terminal-factory"
			const objectKey = "terminal-factory/object"
			entered := make(chan struct{})
			finish := make(chan struct{})
			factoryError := errors.New("healthy granting mount factory failed")
			objType := objecttype.NewObjectType(typeID, func(ctx context.Context, _ *logrus.Entry, _ bus.Bus, _ world.Engine, _ world.WorldState, _ string) (srpc.Invoker, func(), error) {
				// Return the independent failure after the selected terminal event.
				close(entered)
				select {
				case <-finish:
				case <-ctx.Done():
					return nil, nil, ctx.Err()
				}
				if failure == "factory-error" || failure == "child-grant-retired" {
					return nil, nil, factoryError
				}
				return nil, nil, context.Canceled
			})
			if failure == "child-grant-retired" {
				factoryError = sdk_world.ErrTypedObjectGrantRetired
			}

			// Resolve the gated factory through the real controller bus.
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

			// Start the unary call with a separately cancelable caller scope.
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			acquired := make(chan error, 1)
			go func() {
				_, err := sdk_world.AccessTypedObject(ctx, sdk_world.NewSRPCTypedObjectResourceServiceClient(f.rpc), &sdk_world.AccessTypedObjectRequest{ObjectKey: objectKey})
				acquired <- err
			}()
			waitTypedWatchEvent(t, entered)

			// Trigger caller or client cancellation while the grant itself remains healthy.
			switch failure {
			case "caller-canceled":
				cancel()
			case "client-released":
				f.resources.Release()
				baseline = 0
			}
			close(finish)
			err = <-acquired
			if err == nil || errors.Is(err, sdk_world.ErrTypedObjectGrantRetired) {
				t.Fatalf("terminal acquisition error = %v", err)
			}
			if (failure == "factory-error" || failure == "child-grant-retired") && err.Error() != factoryError.Error() {
				t.Fatalf("factory failure = %v, want %v", err, factoryError)
			}
			if failure != "factory-error" && failure != "child-grant-retired" && !errors.Is(err, context.Canceled) {
				t.Fatalf("acquisition cancellation = %v, want context.Canceled", err)
			}

			// Join the blocked server factory before checking the connection's baseline.
			f.mount.Close()
			f.count(t, baseline)
		})
	}
}
