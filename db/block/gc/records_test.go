package block_gc

import (
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// TestNormalizeChildrenAllocs checks that normalizing a page's child list
// allocates its output once, at 32 and 128 children, rather than a block key
// per comparison inside the writer.
func TestNormalizeChildrenAllocs(t *testing.T) {
	for _, n := range []int{32, 128} {
		// Build n children and repeat the first.
		children := make([]*block.BlockRef, 0, n+1)
		for i := range n {
			ref, err := block.BuildBlockRef([]byte("child "+strconv.Itoa(i)), nil)
			if err != nil {
				t.Fatal(err)
			}
			children = append(children, ref)
		}
		children = append(children, children[0])

		// Normalize them and count the allocations.
		got := normalizeChildren(children)
		if len(got) != n {
			t.Fatalf("%d children normalized to %d", n, len(got))
		}
		if allocs := testing.AllocsPerRun(20, func() { normalizeChildren(children) }); allocs > 1 {
			t.Errorf("normalizing %d children made %v allocations, want 1", n, allocs)
		}
	}
}
