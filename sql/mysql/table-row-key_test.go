package mysql

import (
	"bytes"
	"math/rand"
	"sort"
	"testing"
)

// TestTableRowKeySortable ensures that the sorting is as we expect.
func TestTableRowKeySortable(t *testing.T) {
	ordering := []uint64{2, 4, 6, 342, 135135, 342515135}
	vals := make([][]byte, len(ordering))
	for i := range vals {
		vals[i] = MarshalTableRowKey(ordering[i])
	}
	// copy the slice
	valsExpected := make([][]byte, len(vals))
	copy(valsExpected, vals)
	// shuffle the slice
	rand.Shuffle(len(vals), func(i, j int) {
		k := vals[j]
		vals[j] = vals[i]
		vals[i] = k
	})
	// sort again
	sort.Slice(vals, func(i, j int) bool {
		return bytes.Compare(vals[i], vals[j]) < 0
	})
	t.Log("expected", valsExpected)
	t.Log("actual", vals)
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
