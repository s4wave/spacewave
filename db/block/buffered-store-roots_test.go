package block

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/net/hash"
)

// pinStore counts the reader pins it holds. A pin requires the root's bytes.
type pinStore struct {
	*countStore

	// pins counts the held pins by ref key.
	pins map[string]int
}

func (s *pinStore) SupportsRootRetention() bool { return true }

func (s *pinStore) SetRetainedRoot(context.Context, string, *BlockRef) error { return nil }

func (s *pinStore) PinRoot(ctx context.Context, ref *BlockRef) (func(), error) {
	if found, err := s.GetBlockExists(ctx, ref); err != nil || !found {
		return nil, ErrNotFound
	}
	key, _ := marshalRefKey(ref)
	s.pins[key]++
	return func() { s.pins[key]-- }, nil
}

func (s *pinStore) OpenStage(context.Context) (StoreOps, func(), error) { return s, func() {}, nil }

func (s *pinStore) ReleaseRoots(context.Context, []*BlockRef) error { return nil }

func (s *pinStore) MarkRootsComplete(context.Context, []*BlockRef) error { return nil }

func (s *pinStore) RootComplete(context.Context, *BlockRef) (bool, error) { return false, nil }

// held returns the number of pins the store holds on ref.
func (s *pinStore) held(t *testing.T, ref *BlockRef) int {
	key, err := marshalRefKey(ref)
	if err != nil {
		t.Fatal(err)
	}
	return s.pins[key]
}

func TestBufferedStorePinsPendingRootWithoutDraining(t *testing.T) {
	// Buffer two roots.
	ctx := t.Context()
	inner := &pinStore{countStore: newCountStore(hash.HashType_HashType_BLAKE3), pins: make(map[string]int)}
	store := NewBufferedStore(ctx, inner)
	kept, _, err := store.PutBlock(ctx, []byte("kept"), nil)
	if err != nil {
		t.Fatal(err)
	}
	dropped, _, err := store.PutBlock(ctx, []byte("dropped"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Pinning a pending root writes nothing.
	releaseKept, err := PinRoot(ctx, store, kept)
	if err != nil {
		t.Fatal(err)
	}
	releaseDropped, err := PinRoot(ctx, store, dropped)
	if err != nil {
		t.Fatal(err)
	}
	if inner.batchCalls != 0 {
		t.Fatalf("pin drained the buffer: batches=%d", inner.batchCalls)
	}

	// A pin released before the drain never reaches the inner store; one
	// still held moves there with its root.
	releaseDropped()
	if _, err := store.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if got := inner.held(t, dropped); got != 0 {
		t.Fatalf("released pin reached the inner store: %d", got)
	}
	if got := inner.held(t, kept); got != 1 {
		t.Fatalf("held pin after drain: %d want 1", got)
	}

	// Releasing the moved pin releases the inner pin once.
	releaseKept()
	releaseKept()
	if got := inner.held(t, kept); got != 0 {
		t.Fatalf("pin after release: %d want 0", got)
	}
}
