package provider_local

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/testbed"
)

// seedPairingWorldPayload publishes through the real Space engine, whose page
// view may retain derived World pages until an ordered save stores them.
func seedPairingWorldPayload(ctx context.Context, t *testing.T, tb *testbed.Testbed, account *ProviderAccount, ref *sobject.SharedObjectRef, key string, checkpoint bool) (*block.BlockRef, []byte) {
	// Start the same Space body controller that owns browser Worlds.
	t.Helper()
	tb.StaticResolver.AddFactory(space_sobject.NewFactory(tb.Bus))
	_, controller, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&space_sobject.Config{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(controller.Release)

	// Retain the production body and its World engine through pairing.
	mounted, release, err := space.ExMountSpaceSoBody(ctx, tb.Bus, ref, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release.Release)
	engine := mounted.GetSharedObjectBody().GetWorldEngine()

	// Store payload bytes as a normal editor does, before committing its World.
	so, releaseSO, err := account.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSO()
	data, err := (&block_mock.Example{Msg: string(bytes.Repeat([]byte("engine account "+key+"\n"), 8192))}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	leaf, _, err := so.GetBlockStore().PutBlock(ctx, data, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}

	// Appoint the actual storage owner to checkpoint the source edit immediately.
	if checkpoint {
		if _, err := so.(*SharedObject).SetSequencer(ctx, so.GetPeerID().String()); err != nil {
			t.Fatal(err)
		}
	}

	// Commit the object through the engine instead of prewriting World pages.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	obj, err := tx.CreateObject(ctx, key, &bucket.ObjectRef{RootRef: leaf})
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, tx, key, "test/replica-payload"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Record the derived root separately from the durable advertised graph.
	var derived *block.BlockRef
	if err := engine.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		derived = cursor.GetRef().GetRootRef().CloneVT()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Logf("derived World root %s", derived.MarshalString())

	// Observe the production checkpoint before inspecting or exporting its graph.
	if checkpoint {
		states, releaseState, err := so.AccessSharedObjectState(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer releaseState()
		if _, err := states.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
			// Decode the durable checkpoint rather than a locally derived root.
			if snapshot == nil {
				return false, nil
			}
			cp, err := snapshot.GetCheckpoint(ctx)
			if err != nil {
				return false, err
			}
			head := &sobject_world_engine.InnerState{}
			if err := head.UnmarshalVT(cp.GetStateData()); err != nil {
				return false, err
			}
			return !derived.GetEmpty() && head.GetHeadRef().GetRootRef().EqualVT(derived), nil
		}, nil); err != nil {
			t.Fatal(err)
		}
	}

	// Inspect advertised roots through exactly the DEX server's local lookup.
	logPairingStoredRoots(ctx, t, tb, so, derived)
	return leaf, data
}

// logPairingStoredRoots distinguishes missing bytes from unknown child refs
// for the graph the current durable checkpoint and operation set advertise.
func logPairingStoredRoots(ctx context.Context, t *testing.T, tb *testbed.Testbed, so sobject.SharedObject, derived ...*block.BlockRef) {
	// Read the advertised graph without forcing an extra engine/storage fence.
	t.Helper()
	snapshot, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Resolve checkpoint and operation payload roots from the same snapshot.
	span, root, err := sobject_world_engine.PublishedGraph(ctx, snapshot, space_world_optypes.LookupWorldOp)
	if err != nil {
		t.Fatal(err)
	}
	refs := []*block.BlockRef{root}
	if span != nil {
		refs = span.Refs
	}
	advertised := len(refs)
	refs = append(refs, derived...)

	// Hold the same bucket lookup DEX uses to serve local bytes and metadata.
	value, _, reference, err := bucket_lookup.ExBuildBucketLookup(ctx, tb.Bus, false, so.GetBlockStore().GetID(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reference.Release()
	lookup, err := value.GetLookup(ctx)
	if err != nil || lookup == nil {
		t.Fatalf("local bucket lookup unavailable: %v", err)
	}

	// Walk advertised children as DEX would, keeping derived probes separate.
	pending := slices.Clone(refs[:advertised])
	seen := make(map[string]bool)
	for len(pending) != 0 {
		// Visit each advertised block only once, including shared child pages.
		ref := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if ref.GetEmpty() || seen[ref.MarshalString()] {
			continue
		}
		seen[ref.MarshalString()] = true

		// Require both bytes and outgoing edges through the serving lookup.
		stored, err := lookup.LookupStoredBlock(ctx, ref, bucket_lookup.WithLocalOnly())
		if err != nil {
			t.Fatal(err)
		}
		if stored == nil {
			t.Fatalf("advertised block %s: body=false", ref.MarshalString())
		}
		if !stored.RefsKnown {
			t.Fatalf("advertised block %s: refsKnown=false", ref.MarshalString())
		}
		t.Logf("advertised block %s: body=true bytes=%d refsKnown=true children=%d", ref.MarshalString(), len(stored.Data), len(stored.Refs))
		pending = append(pending, stored.Refs...)
	}

	// Log derived page-view roots without requiring their physical storage.
	for _, ref := range refs[advertised:] {
		// An operation-only snapshot may keep its derived World in memory.
		stored, err := lookup.LookupStoredBlock(ctx, ref, bucket_lookup.WithLocalOnly())
		if err != nil {
			t.Fatal(err)
		}
		if stored == nil {
			t.Logf("derived root %s: body=false", ref.MarshalString())
			continue
		}
		t.Logf("derived root %s: body=true bytes=%d refsKnown=%v children=%d", ref.MarshalString(), len(stored.Data), stored.RefsKnown, len(stored.Refs))
	}
}
