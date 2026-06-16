package world_block

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// countingSelfBufferedStore is a self-buffered StoreOps that records whether
// writes arrived on the foreground PutBlock or background PutBlockBackground
// intake, and whether the GC defer-flush scope and Sync barrier were forwarded.
type countingSelfBufferedStore struct {
	block.NopStoreOps
	fgPuts     int
	bgPuts     int
	lastBgOpts *block.PutOpts
	syncs      int
	deferBegin int
	deferEnd   int
	ret        *block.BlockRef
}

func (s *countingSelfBufferedStore) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeatureSelfBuffered
}

func (s *countingSelfBufferedStore) PutBlock(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	s.fgPuts++
	return s.ret, false, nil
}

func (s *countingSelfBufferedStore) PutBlockBackground(_ context.Context, _ []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.bgPuts++
	s.lastBgOpts = opts
	return s.ret, false, nil
}

func (s *countingSelfBufferedStore) Sync(context.Context) (bool, error) {
	s.syncs++
	return true, nil
}

func (s *countingSelfBufferedStore) BeginDeferFlush() { s.deferBegin++ }

func (s *countingSelfBufferedStore) EndDeferFlush(context.Context) error {
	s.deferEnd++
	return nil
}

// TestBackgroundWriteStoreRoutesPutToBackground proves the deferred self-buffered
// adapter sends commit-path PutBlock writes to the inner store's background
// intake (carrying the original opts) while forwarding the Sync barrier and the
// GC defer-flush scope unchanged.
func TestBackgroundWriteStoreRoutesPutToBackground(t *testing.T) {
	ctx := context.Background()
	wantRef, err := block.BuildBlockRef([]byte("payload"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	inner := &countingSelfBufferedStore{ret: wantRef}
	store := newBackgroundWriteStore(inner)

	// the adapter must still advertise the self-buffered feature it wraps.
	if store.GetSupportedFeatures()&block.StoreFeatureSelfBuffered == 0 {
		t.Fatal("adapter must forward the self-buffered feature bit")
	}

	// commit-path PutBlock routes to the background intake, not foreground, and
	// returns exactly the inner ref so commit-time ref validation still holds.
	opts := &block.PutOpts{ForceBlockRef: wantRef.Clone()}
	gotRef, _, err := store.PutBlock(ctx, []byte("payload"), opts)
	if err != nil {
		t.Fatal(err.Error())
	}
	if inner.fgPuts != 0 {
		t.Fatalf("PutBlock must not reach the foreground intake, got %d", inner.fgPuts)
	}
	if inner.bgPuts != 1 {
		t.Fatalf("PutBlock must route to the background intake, got %d", inner.bgPuts)
	}
	if inner.lastBgOpts != opts {
		t.Fatal("background put must receive the original put opts")
	}
	if !gotRef.EqualsRef(wantRef) {
		t.Fatal("adapter must return the inner background-put ref")
	}

	// the GC defer-flush scope the world write path opens on the write store must
	// reach the inner store through the adapter.
	block.BeginDeferFlush(store)
	if err := block.EndDeferFlush(ctx, store); err != nil {
		t.Fatal(err.Error())
	}
	if inner.deferBegin != 1 || inner.deferEnd != 1 {
		t.Fatalf("defer-flush scope must forward to inner, got begin=%d end=%d", inner.deferBegin, inner.deferEnd)
	}

	// the Sync barrier forwards to the inner self-buffered fence.
	fenced, err := store.Sync(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !fenced || inner.syncs != 1 {
		t.Fatalf("Sync must forward to the inner fence, fenced=%v syncs=%d", fenced, inner.syncs)
	}
}
