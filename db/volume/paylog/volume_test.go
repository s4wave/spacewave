//go:build !js

package paylog

import (
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/volume/device"
)

// TestVolumeCollect checks that a Volume in the store's domain collects an
// orphaned block from the store, keeps an owned one, and that both outcomes
// survive a reopen.
func TestVolumeCollect(t *testing.T) {
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			// Publish two blocks and orphan the second.
			ctx := t.Context()
			d := device.NewMemory()
			s := reopen(t, e, d, nil)
			v := openVolume(t, s)
			keep, drop := publish(t, v, "keep"), publish(t, v, "drop")
			orphan(t, v, drop)

			// Collect the orphan.
			stats, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(ctx)
			if err != nil || stats.BlocksSwept != 1 {
				t.Fatalf("collect stats=%+v err=%v", stats, err)
			}
			checkCollected(t, s, v, keep, drop)

			// Reopen and check the outcome again.
			closeVolume(t, v)
			s = reopen(t, e, d, s)
			v = openVolume(t, s)
			checkCollected(t, s, v, keep, drop)
			closeVolume(t, v)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// openVolume opens a Volume in the store's domain under m/.
func openVolume(t *testing.T, s *Store) *volume_kvtx.Volume {
	t.Helper()
	v, err := volume_kvtx.NewVolume(t.Context(), "test/paylog", store_kvkey.NewDefaultKVKey(), s.Domain([]byte("m/")), &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// closeVolume closes v, which leaves its store open.
func closeVolume(t *testing.T, v *volume_kvtx.Volume) {
	t.Helper()
	if err := v.Close(); err != nil {
		t.Fatal(err)
	}
}

// publish publishes one block owned by bucket "owner" with a head named id,
// and returns its reference.
func publish(t *testing.T, v *volume_kvtx.Volume, id string) *block.BlockRef {
	// Build the block and a head update that always applies.
	t.Helper()
	data := []byte("block for " + id)
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	head := &block.AtomicHeadUpdate{ObjectStoreID: id, Key: []byte("head"), Replace: func(context.Context, []byte, bool) ([]byte, error) {
		return []byte(id), nil
	}}

	// Publish the block and its ownership in one commit.
	p := &block.AtomicPublication{
		Entries:  []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}},
		BucketID: "owner",
		TrackGC:  true,
		Heads:    []*block.AtomicHeadUpdate{head},
	}
	if err := v.PublishAtomic(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	return ref
}

// orphan releases the bucket's ownership of ref.
func orphan(t *testing.T, v *volume_kvtx.Volume, ref *block.BlockRef) {
	t.Helper()
	edge := block_gc.RefEdge{Subject: block_gc.BucketIRI("owner"), Object: block_gc.BlockIRI(ref)}
	if err := v.GetRefGraph().ApplyRefBatch(t.Context(), nil, []block_gc.RefEdge{edge}); err != nil {
		t.Fatal(err)
	}
}

// checkCollected checks that keep is stored with its owner and drop is gone
// from the store and the graph.
func checkCollected(t *testing.T, s *Store, v *volume_kvtx.Volume, keep, drop *block.BlockRef) {
	t.Helper()
	for _, c := range []struct {
		ref  *block.BlockRef
		want bool
	}{{keep, true}, {drop, false}} {
		// Check the payload in the store.
		found, err := s.GetBlockExists(t.Context(), c.ref)
		if err != nil || found != c.want {
			t.Fatalf("block %s stored=%v want %v err=%v", c.ref.MarshalString(), found, c.want, err)
		}

		// Check the owner in the graph.
		owners, err := v.GetRefGraph().GetIncomingRefs(t.Context(), block_gc.BlockIRI(c.ref))
		if err != nil {
			t.Fatal(err)
		}
		if owned := slices.Contains(owners, block_gc.BucketIRI("owner")); owned != c.want || (!c.want && len(owners) != 0) {
			t.Fatalf("block %s owners %v, want owned %v", c.ref.MarshalString(), owners, c.want)
		}
	}
}
