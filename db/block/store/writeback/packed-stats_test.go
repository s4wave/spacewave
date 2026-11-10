package block_store_writeback

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// TestPackedStats counts a diverting PackedStore's appended values, a value
// the Log held already, and a cache fill, through a read scope too, and
// counts nothing while not diverting.
func TestPackedStats(t *testing.T) {
	ctx := t.Context()
	env := newTestEnv(t)
	log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	// Divert two values, one twice, and a cache fill.
	s := NewPackedStore(log, ownedStore{env.vol}, true)
	for _, data := range []string{"one", "two", "one"} {
		if _, _, err := s.PutBlock(ctx, []byte(data), &block.PutOpts{RefsKnown: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := s.PutBlock(ctx, []byte("fill"), &block.PutOpts{RefsKnown: true, CacheFill: true}); err != nil {
		t.Fatal(err)
	}
	st := s.GetStats()
	if !s.GetDivert() || st.Diverted != 2 || st.Held != 1 || st.Fills != 1 || st.DivertedBytes == 0 {
		t.Fatalf("diverting: %+v; want 2 diverted with bytes, 1 held, 1 fill", st)
	}
	scope, release, err := s.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := scope.(*PackedStore).GetStats()
	release()
	if got != st {
		t.Fatalf("read scope counts %+v, want %+v", got, st)
	}

	// A store that does not divert counts nothing.
	plain := NewPackedStore(log, ownedStore{env.vol}, false)
	if _, _, err := plain.PutBlock(ctx, []byte("three"), &block.PutOpts{RefsKnown: true}); err != nil {
		t.Fatal(err)
	}
	if st := plain.GetStats(); st != (PackedStats{}) {
		t.Fatalf("not diverting: %+v; want no counts", st)
	}
}
