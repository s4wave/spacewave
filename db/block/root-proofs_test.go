package block

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/net/hash"
)

// proofStore records completion proofs by ref key.
type proofStore struct {
	*pinStore

	// proofs holds the proven ref keys.
	proofs map[string]bool
}

func (s *proofStore) MarkRootsComplete(_ context.Context, roots []*BlockRef) error {
	for _, root := range roots {
		key, _ := marshalRefKey(root)
		s.proofs[key] = true
	}
	return nil
}

func (s *proofStore) RootsComplete(_ context.Context, refs []*BlockRef) ([]bool, error) {
	complete := make([]bool, len(refs))
	for i, ref := range refs {
		key, _ := marshalRefKey(ref)
		complete[i] = s.proofs[key]
	}
	return complete, nil
}

func TestDerivedRootProofFollowsBase(t *testing.T) {
	// Write a base World and a root over it.
	ctx := t.Context()
	pins := &pinStore{countStore: newCountStore(hash.HashType_HashType_BLAKE3), pins: make(map[string]int)}
	store := &proofStore{pinStore: pins, proofs: make(map[string]bool)}
	base, _, err := store.PutBlock(ctx, []byte("base"), nil)
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := store.PutBlock(ctx, []byte("derived"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// A base read on demand leaves the derived root unproven.
	if err := MarkDerivedRootsComplete(ctx, store, base, root); err != nil {
		t.Fatal(err)
	}
	if complete, _ := RootComplete(ctx, store, root); complete {
		t.Fatal("root over an unproven base is proven")
	}

	// A proven base, or none, proves the derived root.
	for _, base := range []*BlockRef{base, nil} {
		store.proofs = map[string]bool{}
		if err := store.MarkRootsComplete(ctx, []*BlockRef{base}); err != nil {
			t.Fatal(err)
		}
		if err := MarkDerivedRootsComplete(ctx, store, base, root); err != nil {
			t.Fatal(err)
		}
		if complete, _ := RootComplete(ctx, store, root); !complete {
			t.Fatalf("root over base %v is unproven", base)
		}
	}
}
