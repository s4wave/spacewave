package volume_world_test

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	volume_world "github.com/s4wave/spacewave/db/volume/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// TestWorldVolumeAtomicPublicationFencesAndRollsBack checks that nested volume
// publication preserves rejected and discarded state and survives reopening.
func TestWorldVolumeAtomicPublicationFencesAndRollsBack(t *testing.T) {
	// Start real block storage for a World whose commits defer their durable head.
	ctx := t.Context()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Record only durable World heads so recovery cannot read an unfenced root.
	cur, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var durableHead atomic.Pointer[bucket.ObjectRef]
	commitHead := func(_ context.Context, _, next *bucket.ObjectRef) error {
		durableHead.Store(next.Clone())
		return nil
	}
	engine, err := world_block.NewEngine(ctx, tb.Logger, cur, world_mock.LookupMockOp, commitHead, false, world_block.WithDeferredDurability())
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	// Open the nested volume on the deferred parent World.
	conf := &volume_world.Config{ObjectKey: "atomic-volume"}
	vol, err := volume_world.NewVolumeWithEngine(ctx, tb.Logger, tb.Bus, transform_all.BuildFactorySet(), conf, engine)
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()

	// Publish block bytes and their metadata reference through the nested fence.
	data := []byte("accepted nested block")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	head := []byte(ref.MarshalString())
	p := &block.AtomicPublication{
		Entries: []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}},
		Heads: []*block.AtomicHeadUpdate{{
			ObjectStoreID: "nested", Key: []byte("head"),
			Replace: func(_ context.Context, _ []byte, found bool) ([]byte, error) {
				if found {
					return nil, errors.New("initial head already exists")
				}
				return head, nil
			},
		}},
	}
	if err := vol.PublishAtomic(ctx, p); err != nil {
		t.Fatal(err)
	}

	// A successful nested commit must already have fenced the deferred World.
	if saved := durableHead.Load(); saved == nil || !saved.EqualsRef(engine.GetRootRef()) {
		t.Fatal("publication returned before its World root became durable")
	}

	// Reject a prepared block after the validator reads its transaction overlay.
	rejectedData := []byte("rejected nested block")
	rejectedRef, err := block.BuildBlockRef(rejectedData, nil)
	if err != nil {
		t.Fatal(err)
	}
	rejected := errors.New("reject prepared publication")
	p.Entries = []*block.PutBatchEntry{{Ref: rejectedRef, Data: rejectedData, RefsKnown: true}}
	p.Heads[0].Replace = func(_ context.Context, current []byte, found bool) ([]byte, error) {
		if !found || !bytes.Equal(current, head) {
			return nil, errors.New("accepted head changed")
		}
		return []byte(rejectedRef.MarshalString()), nil
	}
	p.Validate = func(ctx context.Context, prepared block.StoreOps) error {
		// Confirm the validator sees the candidate before rejecting its publication.
		got, found, err := prepared.GetBlock(ctx, rejectedRef)
		if err != nil || !found || !bytes.Equal(got, rejectedData) {
			t.Errorf("prepared block=%q found=%v err=%v", got, found, err)
		}
		return rejected
	}
	if err := vol.PublishAtomic(ctx, p); !errors.Is(err, rejected) {
		t.Fatalf("rejected publication: %v", err)
	}

	// Discard a direct metadata overwrite before it can publish a World root.
	key := append(vol.GetKvKey().GetObjectStorePrefixByID("nested"), []byte("head")...)
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, key, []byte("discarded head")); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	tx.Discard()
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}

	// Drop the parent engine without a caller Sync and recover from its saved head.
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	recoveryCursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	recoveryCursor.SetRootRef(durableHead.Load().GetRootRef())
	recovered, err := world_block.NewEngine(ctx, tb.Logger, recoveryCursor, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()

	// Require the accepted head and bytes together after reopening durable storage.
	reopened, err := volume_world.NewVolumeWithEngine(ctx, tb.Logger, tb.Bus, transform_all.BuildFactorySet(), conf, recovered)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	read, err := reopened.GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	gotHead, found, err := read.Get(ctx, key)
	read.Discard()
	if err != nil || !found || !bytes.Equal(gotHead, head) {
		t.Fatalf("reopened head=%q found=%v err=%v", gotHead, found, err)
	}

	// The saved head retains its accepted bytes and excludes the rejected candidate.
	got, found, err := reopened.GetBlock(ctx, ref)
	if err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("accepted block=%q found=%v err=%v", got, found, err)
	}
	if found, err := reopened.GetBlockExists(ctx, rejectedRef); err != nil || found {
		t.Fatalf("rejected block exists=%v err=%v", found, err)
	}
}
