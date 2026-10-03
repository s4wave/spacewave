package mysql

import (
	"bytes"
	"math/rand/v2"
	"slices"
	"testing"
)

// TestTableRowKeySortable ensures that the sorting is as we expect.
func TestTableRowKeySortable(t *testing.T) {
	// Encode ordered row nonces into their sortable table row keys.
	ordering := []uint64{2, 4, 6, 342, 135135, 342515135}
	vals := make([][]byte, len(ordering))
	for i := range vals {
		vals[i] = MarshalTableRowKey(ordering[i])
	}

	// Retain the expected key order before shuffling the encoded keys.
	// copy the slice
	valsExpected := make([][]byte, len(vals))
	copy(valsExpected, vals)

	// Shuffle the encoded row keys to test their sort order.
	// shuffle the slice
	rand.Shuffle(len(vals), func(i, j int) {
		k := vals[j]
		vals[j] = vals[i]
		vals[i] = k
	})

	// Sort the encoded keys and record their expected and actual order.
	// sort again
	slices.SortFunc(vals, func(a, b []byte) int {
		return bytes.Compare(a, b)
	})
	t.Log("expected", valsExpected)
	t.Log("actual", vals)

	// Decode the sorted row keys and verify their original nonce order.
	// unmarshal
	out := make([]uint64, len(ordering))
	var err error
	for i, v := range vals {
		out[i], err = UnmarshalTableRowKey(v)
		if err != nil {
			t.Fatal(err.Error())
		}
		if out[i] != ordering[i] {
			t.Fatalf("expected at index %d value %d but got %d", i, ordering[i], out[i])
		}
	}
}
