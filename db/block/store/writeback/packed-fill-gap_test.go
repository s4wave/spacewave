package block_store_writeback

import (
	"fmt"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// TestPackedFillMarkerGap checks that a cache fill through a Store over a
// PackedStore gets an upload marker whether or not the PackedStore diverts,
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
	for _, divert := range []bool{false, true} {
		s, err := NewStoreWithPacks(ctx, NewPackedStore(log, ownedStore{env.vol}, divert), env.vol, env.markers(t), testMarkersID, true)
		if err != nil {
			t.Fatal(err)
		}

		// A cache fill lands in the Volume with its marker.
		data := []byte(fmt.Sprintf("peer fill, divert=%v", divert))
		ref, _, err := s.PutBlock(ctx, data, &block.PutOpts{RefsKnown: true, CacheFill: true})
		if err != nil {
			t.Fatal(err)
		}
		if _, found, _ := env.vol.GetBlock(ctx, ref); !found {
			t.Errorf("divert=%v: cache fill not in the Volume", divert)
		}
		if !env.isMarked(t, ref) {
			t.Errorf("divert=%v: cache fill has no upload marker", divert)
		}

		// A new value is marked only when it is not diverted.
		data = []byte(fmt.Sprintf("new value, divert=%v", divert))
		ref, _, err = s.PutBlock(ctx, data, &block.PutOpts{RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
		if marked := env.isMarked(t, ref); marked == divert {
			t.Errorf("divert=%v: new value marked %v", divert, marked)
		}
	}
}
