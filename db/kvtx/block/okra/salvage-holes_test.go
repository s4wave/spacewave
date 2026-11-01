package kvtx_block_okra

import (
	"bytes"
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// TestSalvageHolesMatchesOrderedWalk removes leaf pages from a tree and
// requires the parallel hole walk to find the same holes and readable keys
// as the ordered Salvage walk, over the whole tree and over a bounded range.
func TestSalvageHolesMatchesOrderedWalk(t *testing.T) {
	ctx := context.Background()
	store := newOkraTestStore()
	fixture := newOkraFixture(t, ctx, store, 9999)
	rootRef, _ := writeOkraFixture(t, ctx, store, fixture)

	// Remove every 7th leaf page of the tree.
	tx := openOkraRoot(t, ctx, store, rootRef, false)
	page, cursor, err := tx.getRootPage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if page.GetLevel() < 1 {
		t.Fatalf("root level %d, want a tree at least 2 levels tall", page.GetLevel())
	}
	var leaves []*block.BlockRef
	var collect func(p *Page, c *block.Cursor)
	collect = func(p *Page, c *block.Cursor) {
		for i := range p.GetEntries() {
			if p.GetLevel() == 1 {
				leaves = append(leaves, p.GetEntries()[i].GetChildRef())
				continue
			}
			cc := p.FollowChild(c, i)
			cp, err := loadPage(ctx, cc)
			if err != nil {
				t.Fatal(err)
			}
			collect(cp, cc)
		}
	}
	collect(page, cursor)
	tx.Discard()
	removed := 0
	for i := 3; i < len(leaves); i += 7 {
		if err := store.RmBlock(ctx, leaves[i]); err != nil {
			t.Fatal(err)
		}
		removed++
	}

	// Compare both walks over the whole tree and over a middle range.
	for _, span := range [][2][]byte{{nil, nil}, {[]byte("key-2000"), []byte("key-8000")}} {
		ordered := openOkraRoot(t, ctx, store, rootRef, false)
		var wantKeys uint64
		var wantHoles []SalvageHole
		err := ordered.Salvage(ctx, span[0], span[1], func(BuildEntry) bool {
			wantKeys++
			return true
		}, func(h SalvageHole) bool {
			wantHoles = append(wantHoles, h)
			return true
		})
		ordered.Discard()
		if err != nil {
			t.Fatal(err)
		}

		parallel := openOkraRoot(t, ctx, store, rootRef, false)
		keys, holes, err := parallel.SalvageHoles(ctx, span[0], span[1])
		parallel.Discard()
		if err != nil {
			t.Fatal(err)
		}
		if keys != wantKeys || len(holes) != len(wantHoles) {
			t.Fatalf("span %q: keys %d holes %d, want keys %d holes %d", span, keys, len(holes), wantKeys, len(wantHoles))
		}
		for i := range holes {
			if !holes[i].Ref.EqualsRef(wantHoles[i].Ref) || !bytes.Equal(holes[i].Low, wantHoles[i].Low) ||
				!bytes.Equal(holes[i].High, wantHoles[i].High) || holes[i].Keys != wantHoles[i].Keys || holes[i].Level != wantHoles[i].Level {
				t.Fatalf("span %q hole %d differs", span, i)
			}
		}
		if span[0] == nil && len(holes) != removed {
			t.Fatalf("found %d holes, removed %d pages", len(holes), removed)
		}
	}
}
