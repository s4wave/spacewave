//go:build !js

package paylog

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
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
			orphan(t, v, "drop", drop)

			// Collect the orphan.
			res, err := v.Collect(ctx)
			if err != nil || res.Freed != 1 {
				t.Fatalf("collect result=%+v err=%v", res, err)
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

// openVolume opens a Volume in the store's domain under m/, whose collector
// frees at once.
func openVolume(t *testing.T, s *Store) *volume_kvtx.Volume {
	t.Helper()
	conf := &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"}
	v, err := volume_kvtx.NewVolume(t.Context(), "test/paylog", store_kvkey.NewDefaultKVKey(), s.Domain([]byte("m/")), conf, false, false, nil, nil)
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

// publish publishes one block held by a root of bucket "owner" named id, with
// a head named id, and returns its reference.
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

	// Publish the block and its root in one commit.
	p := &block.AtomicPublication{
		Entries:  []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}},
		BucketID: "owner",
		TrackGC:  true,
		Heads:    []*block.AtomicHeadUpdate{head},
		Roots:    []block.NamedRoot{{Name: id, Ref: ref}},
	}
	if err := v.PublishAtomic(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	return ref
}

// orphan releases the root id of bucket "owner", which holds ref.
func orphan(t *testing.T, v *volume_kvtx.Volume, id string, ref *block.BlockRef) {
	t.Helper()
	if err := v.SetBucketRoots(t.Context(), "owner", nil, []block.NamedRoot{{Name: id, Previous: ref}}); err != nil {
		t.Fatal(err)
	}
}

// checkCollected checks that keep is stored with its root and child list and
// drop is gone from the store and the liveness records.
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

		// Check its child list.
		listed, err := v.HasChildLists(t.Context(), []*block.BlockRef{c.ref})
		if err != nil {
			t.Fatal(err)
		}
		if listed[0] != c.want {
			t.Fatalf("block %s listed %v, want %v", c.ref.MarshalString(), listed[0], c.want)
		}
	}
}
