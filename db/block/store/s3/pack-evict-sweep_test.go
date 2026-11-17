//go:build !tinygo

package block_store_s3

import (
	"errors"
	"testing"

	"github.com/s4wave/spacewave/net/hash"
)

// TestEvictCountsOnlyRegisteredPacks quarantines the only pack holding some
// blocks and finds eviction unable to use it: the halt names the sweep, the
// locate places none of its blocks, and its verify refuses it as
// unregistered. After a lift the pack counts again and verifies.
func TestEvictCountsOnlyRegisteredPacks(t *testing.T) {
	eachIndexVolume(t, func(t *testing.T, kind string) {
		ctx := t.Context()
		_, client := newFakeBucket(t, nil)
		s := openSweepStore(client, newIndexKV(t, kind))
		defer s.Close()
		refs := adoptBlocks(ctx, t, s, "q", 3)
		id := packOf(t, s, refs[0])
		hashes := make([]*hash.Hash, len(refs))
		for i, ref := range refs {
			hashes[i] = ref.GetHash()
		}
		locs, err := s.EvictLocate(ctx, hashes)
		if err != nil || locs[0].ID != id {
			t.Fatalf("locate before the quarantine = %+v, %v", locs, err)
		}
		size := locs[0].Size

		// Quarantined, the pack counts for nothing.
		if err := s.Quarantine(ctx, []string{id}); err != nil {
			t.Fatal(err)
		}
		if got := s.EvictHalt(); got != EvictHaltSweep {
			t.Fatalf("halt = %q while a pack is held, want %q", got, EvictHaltSweep)
		}
		locs, err = s.EvictLocate(ctx, hashes)
		if err != nil {
			t.Fatal(err)
		}
		for i, loc := range locs {
			if loc.ID != "" {
				t.Fatalf("located block %d in quarantined pack %s", i, loc.ID)
			}
		}
		values, err := s.VerifyPack(ctx, id, size)
		var verr *PackVerifyError
		if !errors.As(err, &verr) || verr.Reason != VerifyUnregistered || values != nil {
			t.Fatalf("verify of a quarantined pack = %d values, %v; want refused unregistered", len(values), err)
		}

		// Lifted, it counts and verifies again.
		if err := s.Lift(ctx, []string{id}); err != nil {
			t.Fatal(err)
		}
		if got := s.EvictHalt(); got != "" {
			t.Fatalf("halt = %q with no pack held", got)
		}
		locs, err = s.EvictLocate(ctx, hashes)
		if err != nil || locs[0].ID != id {
			t.Fatalf("locate after the lift = %+v, %v", locs, err)
		}
		values, err = s.VerifyPack(ctx, id, size)
		if err != nil || len(values) != len(refs) {
			t.Fatalf("verify after the lift = %d values, %v", len(values), err)
		}
	})
}
