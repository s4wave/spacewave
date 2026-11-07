package packsweep

import (
	"maps"
	"slices"
	"testing"
	"time"
)

// TestHeadRow round-trips a header with every field set and one with none,
// and refuses each truncation of the full row.
func TestHeadRow(t *testing.T) {
	full := &Head{
		Cycle:         3,
		SnapshotAt:    time.Unix(1_800_000_000, 123),
		SnapshotPacks: 31250,
		Catalogued:    true,
		Roots:         []string{"rootA", "rootB"},
		LogPosition:   "00000042",
		Walked:        true,
		Live:          830_000,
		Holes:         map[string]uint64{"missing": 2, "unparsed": 1},
		JudgedAt:      time.Unix(1_800_000_600, 0),
	}
	for _, want := range []*Head{full, {}} {
		var got Head
		if err := got.unmarshal(want.marshal()); err != nil {
			t.Fatal(err)
		}
		if got.Cycle != want.Cycle || !got.SnapshotAt.Equal(want.SnapshotAt) ||
			got.SnapshotPacks != want.SnapshotPacks || got.Catalogued != want.Catalogued ||
			got.Seeded != want.Seeded || got.Walked != want.Walked ||
			!slices.Equal(got.Roots, want.Roots) || got.LogPosition != want.LogPosition ||
			got.Live != want.Live || !maps.Equal(got.Holes, want.Holes) ||
			!got.JudgedAt.Equal(want.JudgedAt) {
			t.Fatalf("round trip = %+v, want %+v", got, *want)
		}
	}
	row := full.marshal()
	for n := range len(row) {
		var got Head
		if err := got.unmarshal(row[:n]); err == nil {
			t.Fatalf("truncated row of %d bytes decoded", n)
		}
	}
}
