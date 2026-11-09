package block_store_writeback

import (
	"fmt"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// plainStore hides the atomic publication of the store it wraps, so a Store
// over it marks after each write instead of in the storing transaction.
type plainStore struct {
	block.StoreOps
}

// TestPackedFillMarkerGap checks that a cache fill through a Store over a
// PackedStore gets an upload marker whether or not the PackedStore diverts,
// and whether the marker commits with the write or after it,
// as a peer fill with PEER_FILLS_UPLOAD=on needs one: the remote lacks the
// block. A value the PackedStore diverts to its Log gets none, since its pack
// is its upload queue entry.
func TestPackedFillMarkerGap(t *testing.T) {
	ctx := t.Context()
	env := newTestEnv(t)
	log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	for _, tc := range []struct{ divert, atomic bool }{{false, true}, {true, true}, {false, false}, {true, false}} {
		divert := tc.divert
		var local block.StoreOps = ownedStore{env.vol}
		if !tc.atomic {
			local = plainStore{local}
		}
		s, err := NewStoreWithPacks(ctx, NewPackedStore(log, local, divert), env.vol, env.markers(t), testMarkersID, true)
		if err != nil {
			t.Fatal(err)
		}
		if got := supportsAtomicPublication(local); got != tc.atomic {
			t.Fatalf("atomic publication %v, want %v", got, tc.atomic)
		}

		// A cache fill lands in the Volume with its marker.
		data := []byte(fmt.Sprintf("peer fill, divert=%v atomic=%v", divert, tc.atomic))
		ref, _, err := s.PutBlock(ctx, data, &block.PutOpts{RefsKnown: true, CacheFill: true})
		if err != nil {
			t.Fatal(err)
		}
		if _, found, _ := env.vol.GetBlock(ctx, ref); !found {
			t.Errorf("%+v: cache fill not in the Volume", tc)
		}
		if !env.isMarked(t, ref) {
			t.Errorf("%+v: cache fill has no upload marker", tc)
		}

		// A new value is marked only when it is not diverted.
		data = []byte(fmt.Sprintf("new value, divert=%v atomic=%v", divert, tc.atomic))
		ref, _, err = s.PutBlock(ctx, data, &block.PutOpts{RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
		if marked := env.isMarked(t, ref); marked == divert {
			t.Errorf("%+v: new value marked %v", tc, marked)
		}
	}
}
