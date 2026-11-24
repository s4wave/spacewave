package block_store_fillcache

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

func TestCacheResidentListYieldsWhatTheCacheLacks(t *testing.T) {
	// Fill two of four blocks into a cache, and save all four as one scope's
	// list, the first one twice, as a walk over two block types yields it.
	path := filepath.Join(t.TempDir(), "fill.db")
	c := openTest(t, path, 1<<30)
	src, refs := testBlocks(t, 4)
	fillRead(t, c, src, refs[0])
	fillRead(t, c, src, refs[1])
	keys := make([]string, 0, len(refs)+1)
	for _, ref := range append([]*block.BlockRef{refs[0]}, refs...) {
		key, err := ref.MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, string(key))
	}
	if err := c.SaveResident(t.Context(), "store/a", slices.Values(keys)); err != nil {
		t.Fatal(err)
	}

	// Read the list back: only the two blocks the cache lacks, once each.
	var listed int
	missing := func(scope string) []*block.BlockRef {
		var got []*block.BlockRef
		var err error
		listed, err = c.MissingResident(t.Context(), scope, func(refs []*block.BlockRef) error {
			got = append(got, refs...)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	got := missing("store/a")
	if listed != 4 || len(got) != 2 || !slices.ContainsFunc(got, refs[2].EqualsRef) || !slices.ContainsFunc(got, refs[3].EqualsRef) {
		t.Fatalf("listed %d, missing %v, want 4 listed and refs 2 and 3 missing", listed, got)
	}

	// Another scope has no list, and a damaged list ends at its damage.
	if got := missing("store/b"); len(got) != 0 {
		t.Fatalf("unsaved scope yielded %v", got)
	}
	list := c.residentPath("store/a")
	info, err := os.Stat(list)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(list, info.Size()-1); err != nil {
		t.Fatal(err)
	}
	if got := missing("store/a"); len(got) != 1 {
		t.Fatalf("truncated list yielded %v, want the one whole missing ref", got)
	}
	if err := os.WriteFile(list, []byte("not a list"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := missing("store/a"); len(got) != 0 {
		t.Fatalf("foreign file yielded %v", got)
	}
}
