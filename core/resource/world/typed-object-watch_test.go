//go:build !js

package resource_world_test

import (
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// TestWatchTypedObjectRegistryLifecycle proves standing absence, loss and actual replacements.
func TestWatchTypedObjectRegistryLifecycle(t *testing.T) {
	// Start with a real typed World object whose handler is initially absent.
	f := newTypedWatchFixture(t, "")
	const typeID = "watch/object"
	const key = "watch/one"
	f.setType(t, key, typeID, false)
	if _, err := sdk_world.NewSRPCTypedObjectResourceServiceClient(f.rpc).AccessTypedObject(t.Context(), &sdk_world.AccessTypedObjectRequest{ObjectKey: key}); err == nil {
		t.Fatal("unary access waited or succeeded without a handler")
	}
	stream, cancel := f.watch(t, f.rpc, key)
	receiveTypedWatchSnapshot(t, stream, typeID, false)
	disposed := f.demand(t, typeID, "")

	// Admit A and retain its independently adopted child.
	a := newTypedWatchHandler("A", "")
	aGeneration := f.register(t, typeID, "A", "", a)
	defer aGeneration.Release()
	first := receiveTypedWatchSnapshot(t, stream, typeID, true)
	firstRef := f.child(t, first, "A")
	defer firstRef.Release()

	// Withdrawal revokes A while its adopted reference remains retained.
	aGeneration.Release()
	receiveTypedWatchSnapshot(t, stream, typeID, false)
	f.rejected(t, firstRef)
	waitTypedWatchEvent(t, a.released)

	// A later registration must create an independently invocable generation.
	b := newTypedWatchHandler("B", "")
	bGeneration := f.register(t, typeID, "B", "", b)
	defer bGeneration.Release()
	second := receiveTypedWatchSnapshot(t, stream, typeID, true)
	if second.GetResourceId() == first.GetResourceId() {
		t.Fatal("replacement reused the old resource ID")
	}
	secondRef := f.child(t, second, "B")
	defer secondRef.Release()

	// Direct activation of C must revoke B before publishing C's new child.
	c := newTypedWatchHandler("C", "")
	cGeneration := f.register(t, typeID, "C", "", c)
	defer cGeneration.Release()
	third := receiveTypedWatchSnapshot(t, stream, typeID, true)

	// Verify replacement rejects B without automatically invoking C.
	f.rejected(t, secondRef)
	waitTypedWatchEvent(t, b.released)
	if third.GetResourceId() == second.GetResourceId() {
		t.Fatal("direct replacement reused B's resource ID")
	}
	select {
	case <-c.calls:
		t.Fatal("watch automatically invoked a child method")
	default:
	}
	thirdRef := f.child(t, third, "C")
	defer thirdRef.Release()

	// Cancellation revokes the adopted child and standing demand with the client open.
	baseline := f.server.CountTrackedResources() - 1
	cancel()
	waitTypedWatchEvent(t, c.released)
	waitTypedWatchEvent(t, disposed)
	f.count(t, baseline)
	f.rejected(t, thirdRef)
}

// TestWatchTypedObjectWorldLifecycle proves live and pending transaction transitions.
func TestWatchTypedObjectWorldLifecycle(t *testing.T) {
	for _, mutableTx := range []bool{false, true} {
		// Run the same object lifecycle through live and uncommitted mounted Worlds.
		name := "live"
		if mutableTx {
			name = "pending-transaction"
		}
		t.Run(name, func(t *testing.T) {
			// Mount the object lifecycle under the test's real World resource.
			f := newTypedWatchFixture(t, "")
			const aType = "watch/a"
			const bType = "watch/b"
			const key = "watch/mutable"

			// Retain both types through independent registry admission families.
			a := newTypedWatchHandler("A", "")
			b := newTypedWatchHandler("B", "")
			aGeneration := f.register(t, aType, "A", "", a)
			defer aGeneration.Release()
			bGeneration := f.register(t, bType, "B", "", b)
			defer bGeneration.Release()

			// Select the exact mount and mutation surface without opening a new World.
			rpc := f.rpc
			mutate := func(typeID string, remove bool) { f.setType(t, key, typeID, remove) }
			var transaction *sdk_world_engine.SDKTx
			if mutableTx {
				response, err := sdk_world.NewSRPCEngineResourceServiceClient(f.rpc).NewTransaction(t.Context(), &sdk_world.NewTransactionRequest{Write: true})
				if err != nil {
					t.Fatal(err)
				}
				ref := f.resources.CreateResourceReference(response.GetResourceId())
				transaction, err = sdk_world_engine.NewSDKTx(f.resources, ref, response.GetReadOnly())
				if err != nil {
					ref.Release()
					t.Fatal(err)
				}
				t.Cleanup(transaction.Discard)
				rpc, err = ref.GetClient()
				if err != nil {
					t.Fatal(err)
				}
				mutate = func(typeID string, remove bool) { mutateTypedWatchObject(t, transaction, key, typeID, remove) }
			}
			stream, cancel := f.watch(t, rpc, key)
			receiveTypedWatchSnapshot(t, stream, "", false)

			// Creation and type admission must wake an initially absent object watch.
			mutate(aType, false)
			first := receiveTypedWatchSnapshot(t, stream, aType, true)
			firstRef := f.child(t, first, "A")
			defer firstRef.Release()

			// A direct type change must revoke A before B becomes invocable.
			mutate(bType, false)
			direct := receiveTypedWatchSnapshot(t, stream, bType, true)
			directRef := f.child(t, direct, "B")
			defer directRef.Release()
			f.rejected(t, firstRef)
			waitTypedWatchEvent(t, a.released)

			// Removing the type revokes the current adopted child while retaining the stream.
			mutate("", false)
			receiveTypedWatchSnapshot(t, stream, "", false)
			f.rejected(t, directRef)
			waitTypedWatchEvent(t, b.released)

			// Retyping, deletion and recreation must each follow their actual World events.
			mutate(bType, false)
			second := receiveTypedWatchSnapshot(t, stream, bType, true)
			secondRef := f.child(t, second, "B")
			defer secondRef.Release()
			mutate("", true)
			receiveTypedWatchSnapshot(t, stream, "", false)
			f.rejected(t, secondRef)
			waitTypedWatchEvent(t, b.released)

			// Recreate the deleted object under its original type with a fresh child.
			mutate(aType, false)
			third := receiveTypedWatchSnapshot(t, stream, aType, true)
			thirdRef := f.child(t, third, "A")
			defer thirdRef.Release()
			if third.GetResourceId() == first.GetResourceId() {
				t.Fatal("recreated object reused a revoked child")
			}

			// Terminal mount release revokes adopted transaction children without disconnecting.
			baseline := f.server.CountTrackedResources() - 1
			disposed := f.demand(t, aType, "")
			if transaction != nil {
				transaction.Discard()
				baseline--
			} else {
				cancel()
			}
			waitTypedWatchEvent(t, a.released)
			waitTypedWatchEvent(t, disposed)
			f.count(t, baseline)
			f.rejected(t, thirdRef)
		})
	}
}

// TestWatchTypedObjectTrustedRegistryScope proves bound global and scoped mounts mask callers.
func TestWatchTypedObjectTrustedRegistryScope(t *testing.T) {
	for _, scope := range []string{"", "installation"} {
		t.Run("scope="+scope, func(t *testing.T) {
			// Mount a typed object under the selected trusted registry scope.
			f := newTypedWatchFixture(t, scope)
			const typeID = "watch/scoped"
			const key = "watch/scope"
			f.setType(t, key, typeID, false)

			// Retain different global and installation handlers for the same type.
			global := newTypedWatchHandler("global", scope)
			scoped := newTypedWatchHandler("scoped", scope)
			globalGeneration := f.register(t, typeID, "global", "", global)
			defer globalGeneration.Release()
			scopedGeneration := f.register(t, typeID, "scoped", "installation", scoped)
			defer scopedGeneration.Release()

			// The fixture injects a foreign caller scope into every mounted RPC context.
			stream, cancel := f.watch(t, f.rpc, key)
			response := receiveTypedWatchSnapshot(t, stream, typeID, true)
			label := "global"
			selected := global
			if scope != "" {
				label, selected = "scoped", scoped
			}
			ref := f.child(t, response, label)
			defer ref.Release()

			// Observe scoped demand and retain the count baseline with the client open.
			disposed := f.demand(t, typeID, scope)
			baseline := f.server.CountTrackedResources() - 1

			// Cancellation releases the selected child and its exact scoped demand.
			cancel()
			waitTypedWatchEvent(t, selected.released)
			waitTypedWatchEvent(t, disposed)
			f.count(t, baseline)
			f.rejected(t, ref)
		})
	}
}

// TestWatchTypedObjectBlockedRegistryFactory proves loss cancels actual handler construction.
func TestWatchTypedObjectBlockedRegistryFactory(t *testing.T) {
	// Block a caller-attached handler before it creates any invocation child.
	f := newTypedWatchFixture(t, "")
	const typeID = "watch/blocked"
	const key = "watch/blocked"
	f.setType(t, key, typeID, false)
	stream, cancel := f.watch(t, f.rpc, key)
	receiveTypedWatchSnapshot(t, stream, typeID, false)
	disposed := f.demand(t, typeID, "")

	// Admit a gated handler only after initial absence has retained demand.
	a := newTypedWatchHandler("blocked", "")
	gate := make(chan struct{})
	a.gate = gate
	generation := f.register(t, typeID, "blocked", "", a)
	defer generation.Release()
	waitTypedWatchEvent(t, a.started)

	// Withdraw the handler while its real factory RPC remains blocked.
	generation.Release()
	waitTypedWatchEvent(t, a.canceled)
	if got := a.server.WaitTrackedResourceCount(t.Context(), 0); got != 0 {
		t.Fatalf("blocked factory leaked %d handler resources", got)
	}

	// A replacement can satisfy the still-retained demand without invoking A.
	b := newTypedWatchHandler("replacement", "")
	replacementGeneration := f.register(t, typeID, "replacement", "", b)
	defer replacementGeneration.Release()
	response := receiveTypedWatchSnapshot(t, stream, typeID, true)
	replacementRef := f.child(t, response, "replacement")
	defer replacementRef.Release()

	// Cancel the replacement and observe complete invocation child release.
	baseline := f.server.CountTrackedResources() - 1
	cancel()
	waitTypedWatchEvent(t, b.released)
	waitTypedWatchEvent(t, disposed)
	f.count(t, baseline)
}

// TestWatchTypedObjectAbsentCancellation proves demand is withdrawn without ever creating a child.
func TestWatchTypedObjectAbsentCancellation(t *testing.T) {
	// Keep the client connection open while canceling unresolved registry demand.
	f := newTypedWatchFixture(t, "")
	const typeID = "watch/absent"
	const key = "watch/absent"
	f.setType(t, key, typeID, false)
	baseline := f.server.CountTrackedResources()
	stream, cancel := f.watch(t, f.rpc, key)
	receiveTypedWatchSnapshot(t, stream, typeID, false)
	disposed := f.demand(t, typeID, "")

	// Observe disposal through the owning directive and unchanged Resource count.
	cancel()
	waitTypedWatchEvent(t, disposed)
	f.count(t, baseline)
}

// TestWatchTypedObjectClosedMount proves cancellation before owner startup cannot hang teardown.
func TestWatchTypedObjectClosedMount(t *testing.T) {
	// Close the granting mount before the real standing RPC starts.
	f := newTypedWatchFixture(t, "")
	f.mount.Close()
	baseline := f.server.CountTrackedResources()
	stream, err := sdk_world.NewSRPCTypedObjectResourceServiceClient(f.rpc).WatchTypedObject(t.Context(), &sdk_world.WatchTypedObjectRequest{ObjectKey: "watch/closed"})
	if err == nil {
		if _, err := stream.Recv(); err == nil {
			t.Fatal("closed mount published a watch snapshot")
		}
	}

	// No owner, factory or child can survive the already-terminal mount.
	f.count(t, baseline)
}

// TestWatchTypedObjectBackpressureDoesNotRetainOldChild proves revocation is independent of Send.
func TestWatchTypedObjectBackpressureDoesNotRetainOldChild(t *testing.T) {
	// Start with an invocable adopted A child on a real gated Resource transport.
	f := newTypedWatchFixture(t, "")
	const typeID = "watch/backpressure"
	const key = "watch/backpressure"
	f.setType(t, key, typeID, false)
	a := newTypedWatchHandler("A", "")
	aGeneration := f.register(t, typeID, "A", "", a)
	defer aGeneration.Release()

	// Arm backpressure only after the initial available child has been adopted.
	blocked := make(chan struct{}, 1)
	armed := make(chan struct{})
	release := make(chan struct{})
	f.wrapWatch = func(stream srpc.Stream) srpc.Stream {
		return &typedWatchSendGate{Stream: stream, armed: armed, blocked: blocked, release: release}
	}

	// Adopt initial availability before enabling transport backpressure.
	stream, cancel := f.watch(t, f.rpc, key)
	first := receiveTypedWatchSnapshot(t, stream, typeID, true)
	ref := f.child(t, first, "A")
	defer ref.Release()
	close(armed)

	// Replacement must revoke A and construct B while the next Send remains gated.
	b := newTypedWatchHandler("B", "")
	bGeneration := f.register(t, typeID, "B", "", b)
	defer bGeneration.Release()
	waitTypedWatchEvent(t, blocked)
	waitTypedWatchEvent(t, a.released)
	waitTypedWatchEvent(t, b.started)
	f.rejected(t, ref)

	// Drain transport backpressure and invoke the replacement explicitly.
	close(release)
	second := receiveTypedWatchSnapshot(t, stream, typeID, true)
	secondRef := f.child(t, second, "B")
	defer secondRef.Release()

	// Cancel the replacement and observe complete invocation child release.
	baseline := f.server.CountTrackedResources() - 1
	cancel()
	waitTypedWatchEvent(t, b.released)
	f.count(t, baseline)
}
