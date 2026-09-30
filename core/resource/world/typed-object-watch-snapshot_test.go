//go:build !js

package resource_world_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	"github.com/sirupsen/logrus"
)

// TestWatchTypedObjectImmutableSnapshot proves pinned metadata still follows handler replacement.
func TestWatchTypedObjectImmutableSnapshot(t *testing.T) {
	// Mount a read transaction containing A before the live World changes.
	f := newTypedWatchFixture(t, "")
	const typeID = "watch/snapshot"
	const key = "watch/snapshot"
	f.setType(t, key, typeID, false)
	a := newTypedWatchHandler("A", "")
	aGeneration := f.register(t, typeID, "A", "", a)
	defer aGeneration.Release()

	// Retain the immutable read transaction as a separately owned Resource.
	response, err := sdk_world.NewSRPCEngineResourceServiceClient(f.rpc).NewTransaction(t.Context(), &sdk_world.NewTransactionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	ref := f.resources.CreateResourceReference(response.GetResourceId())
	t.Cleanup(ref.Release)
	rpc, err := ref.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Begin typed demand through the retained snapshot mount.
	stream, cancel := f.watch(t, rpc, key)
	first := receiveTypedWatchSnapshot(t, stream, typeID, true)
	firstRef := f.child(t, first, "A")
	defer firstRef.Release()

	// Delete the live object and replace its handler while retaining the read snapshot.
	f.setType(t, key, "", true)
	b := newTypedWatchHandler("B", "")
	bGeneration := f.register(t, typeID, "B", "", b)
	defer bGeneration.Release()
	second := receiveTypedWatchSnapshot(t, stream, typeID, true)

	// Verify snapshot replacement revokes the old child and grants a fresh ID.
	f.rejected(t, firstRef)
	waitTypedWatchEvent(t, a.released)
	secondRef := f.child(t, second, "B")
	defer secondRef.Release()
	if second.GetResourceId() == first.GetResourceId() {
		t.Fatal("snapshot replacement reused its old child")
	}

	// Discard must terminate demand and revoke the independently adopted snapshot child.
	baseline := f.server.CountTrackedResources() - 1
	disposed := f.demand(t, typeID, "")
	if _, err := sdk_world.NewSRPCTxResourceServiceClient(rpc).Discard(t.Context(), &sdk_world.DiscardRequest{}); err != nil {
		t.Fatal(err)
	}
	waitTypedWatchEvent(t, b.released)
	waitTypedWatchEvent(t, disposed)
	f.count(t, baseline)
	f.rejected(t, secondRef)
	cancel()
}

// TestWatchTypedObjectSnapshotMountTeardown proves tracked snapshot release closes its typed mount.
func TestWatchTypedObjectSnapshotMountTeardown(t *testing.T) {
	// Adopt a tracked snapshot and its independently adopted typed child.
	f := newTypedWatchFixture(t, "")
	const typeID = "watch/tracked"
	const key = "watch/tracked"
	f.setType(t, key, typeID, false)
	handler := newTypedWatchHandler("tracked", "")
	generation := f.register(t, typeID, "tracked", "", handler)
	defer generation.Release()

	// Start the tracked snapshot producer with an independent lifecycle.
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	snapshots, err := sdk_world.NewSRPCWatchWorldStateResourceServiceClient(f.rpc).WatchWorldState(ctx, &sdk_world.WatchWorldStateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := snapshots.Recv()
	if err != nil {
		t.Fatal(err)
	}

	// Adopt the produced snapshot independently of its typed invocation children.
	ref := f.resources.CreateResourceReference(snapshot.GetResourceId())
	t.Cleanup(ref.Release)
	rpc, err := ref.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Retain typed demand beneath the snapshot's transaction lease.
	stream, stopWatch := f.watch(t, rpc, key)
	typed := receiveTypedWatchSnapshot(t, stream, typeID, true)
	child := f.child(t, typed, "tracked")
	defer child.Release()
	disposed := f.demand(t, typeID, "")
	baseline := f.server.CountTrackedResources() - 2

	// Closing the snapshot producer must revoke typed demand before releasing its pin.
	cancel()
	waitTypedWatchEvent(t, handler.released)
	waitTypedWatchEvent(t, disposed)
	f.count(t, baseline)
	f.rejected(t, child)
	stopWatch()
}

// TestWatchTypedObjectFactoryAuthority proves factories receive the exact immutable mount.
func TestWatchTypedObjectFactoryAuthority(t *testing.T) {
	// Register an authority-observing factory alongside the real registry bridge.
	f := newTypedWatchFixture(t, "")
	const typeID = "watch/authority"
	const key = "watch/authority"
	f.setType(t, key, typeID, false)
	released := make(chan struct{}, 1)
	typeFactory := objecttype.NewObjectType(typeID, func(ctx context.Context, le *logrus.Entry, b bus.Bus, engine world.Engine, ws world.WorldState, objectKey string) (srpc.Invoker, func(), error) {
		// Reject any promotion or identity reconstruction of the granted read snapshot.
		if !ws.GetReadOnly() || objectKey != key || objecttype.EngineIDFromContext(ctx) != "" {
			return nil, nil, errors.New("factory authority escaped the granting snapshot")
		}
		found, err := ws.HasObject(ctx, "watch/later")
		if err != nil {
			return nil, nil, err
		}
		if found {
			return nil, nil, errors.New("snapshot factory observed a later accepted object")
		}

		// Read the snapshot's own type after its live object has been deleted.
		observed, err := world_types.GetObjectType(ctx, ws, objectKey)
		if err != nil {
			return nil, nil, err
		}
		if observed != typeID {
			return nil, nil, errors.New("snapshot factory lost its pinned type")
		}
		return srpc.NewMux(), func() { released <- struct{}{} }, nil
	})

	// Retain the immutable read transaction as a separately owned Resource.
	response, err := sdk_world.NewSRPCEngineResourceServiceClient(f.rpc).NewTransaction(t.Context(), &sdk_world.NewTransactionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	ref := f.resources.CreateResourceReference(response.GetResourceId())
	t.Cleanup(ref.Release)
	rpc, err := ref.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Advance the live World before admitting the snapshot-observing factory.
	f.setType(t, "watch/later", typeID, false)
	f.setType(t, key, "", true)
	ctrl := objecttype_controller.NewController(func(context.Context, string) (objecttype.ObjectType, error) { return typeFactory, nil })
	release, err := f.tb.Bus.AddController(t.Context(), ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	// Begin typed demand through the retained snapshot mount.
	stream, cancel := f.watch(t, rpc, key)
	typed := receiveTypedWatchSnapshot(t, stream, typeID, true)
	child := f.resources.CreateResourceReference(typed.GetResourceId())
	t.Cleanup(child.Release)
	baseline := f.server.CountTrackedResources() - 1
	cancel()
	waitTypedWatchEvent(t, released)
	f.count(t, baseline)
}

// TestWatchTypedObjectLateFactoryResult proves cancellation cleans uncooperative completion.
func TestWatchTypedObjectLateFactoryResult(t *testing.T) {
	for _, terminal := range []string{"stream", "mount", "client", "replacement"} {
		t.Run(terminal, func(t *testing.T) {
			// Mount a typed object before gating a deliberately late factory result.
			f := newTypedWatchFixture(t, "")
			const typeID = "watch/late"
			const key = "watch/late"
			f.setType(t, key, typeID, false)
			started := make(chan struct{}, 1)
			canceled := make(chan struct{}, 1)
			finish := make(chan struct{})
			released := make(chan struct{}, 1)

			// Return an acquired capability only after the factory's context is canceled.
			typeFactory := objecttype.NewObjectType(typeID, func(ctx context.Context, _ *logrus.Entry, _ bus.Bus, _ world.Engine, _ world.WorldState, _ string) (srpc.Invoker, func(), error) {
				// Model a factory which completes an acquisition after cancellation.
				started <- struct{}{}
				<-ctx.Done()
				canceled <- struct{}{}
				<-finish
				return srpc.NewMux(), func() { released <- struct{}{} }, nil
			})
			ctrl := objecttype_controller.NewController(func(context.Context, string) (objecttype.ObjectType, error) { return typeFactory, nil })
			release, err := f.tb.Bus.AddController(t.Context(), ctrl, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(release)

			// Retain standing demand while the factory has not published a child.
			baseline := f.server.CountTrackedResources()
			stream, cancel := f.watch(t, f.rpc, key)
			receiveTypedWatchSnapshot(t, stream, typeID, false)
			waitTypedWatchEvent(t, started)
			disposed := f.demand(t, typeID, "")

			// End the selected authority or remove the current handler generation.
			switch terminal {
			case "stream":
				cancel()
			case "mount":
				f.mount.Close()
			case "client":
				f.resources.Release()
				baseline = 0
			case "replacement":
				release()
			}
			waitTypedWatchEvent(t, canceled)
			f.count(t, baseline)

			// Admit a replacement while the superseded factory still holds its late result.
			var replacement *sdk_world.WatchTypedObjectResponse
			if terminal == "replacement" {
				next := objecttype.NewObjectType(typeID, func(context.Context, *logrus.Entry, bus.Bus, world.Engine, world.WorldState, string) (srpc.Invoker, func(), error) {
					return srpc.NewMux(srpc.InvokerFunc(func(serviceID, methodID string, stream srpc.Stream) (bool, error) {
						// Identify the caller's explicit invocation of the replacement child.
						if serviceID != echo.SRPCEchoerServiceID || methodID != "Echo" {
							return false, nil
						}
						if err := stream.MsgRecv(&echo.EchoMsg{}); err != nil {
							return true, err
						}
						return true, stream.MsgSend(&echo.EchoMsg{Body: "replacement"})
					})), func() {}, nil
				})
				nextController := objecttype_controller.NewController(func(context.Context, string) (objecttype.ObjectType, error) { return next, nil })
				releaseNext, err := f.tb.Bus.AddController(t.Context(), nextController, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(releaseNext)
				replacement = receiveTypedWatchSnapshot(t, stream, typeID, true)
				replacementRef := f.child(t, replacement, "replacement")
				defer replacementRef.Release()
			}

			// Clean the late capability without publishing or replacing a current child.
			close(finish)
			waitTypedWatchEvent(t, released)
			if replacement != nil {
				currentRef := f.child(t, replacement, "replacement")
				defer currentRef.Release()
				cancel()
			}
			waitTypedWatchEvent(t, disposed)
			f.count(t, baseline)
		})
	}
}

// TestWatchTypedObjectMountAndClientCleanup proves terminal cancellation releases adopted children.
func TestWatchTypedObjectMountAndClientCleanup(t *testing.T) {
	for _, clientEnds := range []bool{false, true} {
		t.Run(map[bool]string{false: "mount", true: "client"}[clientEnds], func(t *testing.T) {
			// Keep a real adopted typed child alive until its granting lifecycle ends.
			f := newTypedWatchFixture(t, "")
			const typeID = "watch/terminal"
			const key = "watch/terminal"
			f.setType(t, key, typeID, false)
			handler := newTypedWatchHandler("terminal", "")
			generation := f.register(t, typeID, "terminal", "", handler)
			defer generation.Release()

			// Adopt a current child before ending the granting lifecycle.
			stream, cancel := f.watch(t, f.rpc, key)
			response := receiveTypedWatchSnapshot(t, stream, typeID, true)
			ref := f.child(t, response, "terminal")
			defer ref.Release()
			disposed := f.demand(t, typeID, "")
			baseline := f.server.CountTrackedResources() - 1

			// End only the selected lifecycle and observe all owned cleanup events.
			if clientEnds {
				f.resources.Release()
				baseline = 0
			} else {
				f.mount.Close()
			}
			waitTypedWatchEvent(t, handler.released)
			waitTypedWatchEvent(t, disposed)
			f.count(t, baseline)
			f.rejected(t, ref)
			cancel()
		})
	}
}
