package sobject

import (
	"bytes"
	"iter"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
)

// TestSOOperationSetOrder checks that the replay order is a function of the
// set: parents come first, concurrent operations sort by hash, and an
// operation naming one the set lacks waits with its descendants.
func TestSOOperationSetOrder(t *testing.T) {
	// Derive one key per author.
	privA, _ := vectorKey(t, "order a")
	privB, _ := vectorKey(t, "order b")
	privC, _ := vectorKey(t, "order c")

	// Build a DAG: a1 <- a2, b1 <- b2 also naming a1, and c2 whose previous
	// operation c1 is never delivered.
	build := func(priv crypto.PrivKey, link *SOOperationLink) *SOOperation {
		// Sign the operation under one config.
		t.Helper()
		link.ConfigHash = bytes.Repeat([]byte{3}, 32)
		op, err := BuildSOOperation(vectorObjectID, priv, []byte("data"), link, NewSOOperationLocalID())
		if err != nil {
			t.Fatal(err)
		}

		return op
	}
	a1 := build(privA, &SOOperationLink{Nonce: 1})
	a2 := build(privA, &SOOperationLink{Nonce: 2, PrevOpHash: a1.Hash()})
	b1 := build(privB, &SOOperationLink{Nonce: 1})
	b2 := build(privB, &SOOperationLink{Nonce: 2, PrevOpHash: b1.Hash(), ParentHashes: [][]byte{a1.Hash()}})
	c1 := build(privC, &SOOperationLink{Nonce: 1})
	c2 := build(privC, &SOOperationLink{Nonce: 2, PrevOpHash: c1.Hash()})
	delivered := []*SOOperation{a1, a2, b1, b2, c2}

	// Order the set built in the delivered order and in reverse.
	var want [][]byte
	for _, ops := range []iter.Seq2[int, *SOOperation]{slices.All(delivered), slices.Backward(delivered)} {
		set := NewSOOperationSet(vectorObjectID)
		for _, op := range ops {
			if _, err := set.Add(op); err != nil {
				t.Fatal(err)
			}
		}
		got := set.Order()
		if want == nil {
			want = got
			continue
		}
		if !slices.EqualFunc(got, want, bytes.Equal) {
			t.Fatalf("order depends on arrival:\n  %x\n  %x", want, got)
		}
	}

	// Every placed operation follows its links, ties sort by hash, and c2 waits.
	if len(want) != 4 {
		t.Fatalf("placed %d operations; want 4", len(want))
	}
	index := func(op *SOOperation) int {
		return slices.IndexFunc(want, func(h []byte) bool { return bytes.Equal(h, op.Hash()) })
	}
	if index(c2) != -1 {
		t.Fatal("c2 was placed without its previous operation")
	}
	if index(a1) > index(a2) || index(a1) > index(b2) || index(b1) > index(b2) {
		t.Fatalf("an operation precedes one it names: %x", want)
	}
	roots := [][]byte{a1.Hash(), b1.Hash()}
	slices.SortFunc(roots, bytes.Compare)
	if !bytes.Equal(want[0], roots[0]) {
		t.Fatal("concurrent roots are not in hash order")
	}
}
