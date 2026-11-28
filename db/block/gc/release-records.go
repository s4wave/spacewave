package block_gc

import (
	"bytes"
	"context"
	"encoding/binary"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// releasePageKey returns the key of release page page.
func (r *Records) releasePageKey(page uint64) []byte {
	return r.key(kindReleasePage, binary.BigEndian.AppendUint64(nil, page))
}

// putReleasePages writes children, sorted and distinct as a child list
// holds them, to release pages of releasePageChildren each.
func (r *Records) putReleasePages(ctx context.Context, children []*block.BlockRef) error {
	var page uint64
	for start := 0; start < len(children); start += releasePageChildren {
		end := min(start+releasePageChildren, len(children))
		if err := r.tx.Set(ctx, r.releasePageKey(page), EncodeChildren(children[start:end])); err != nil {
			return err
		}
		page++
	}
	return nil
}

// getReleasePage returns the children release page page holds, none when it
// is missing.
func (r *Records) getReleasePage(ctx context.Context, page uint64) ([]*block.BlockRef, error) {
	value, found, err := r.tx.Get(ctx, r.releasePageKey(page))
	if err != nil || !found {
		return nil, err
	}
	return DecodeChildren(value)
}

// deleteReleasePage deletes release page page.
func (r *Records) deleteReleasePage(ctx context.Context, page uint64) error {
	return r.tx.Delete(ctx, r.releasePageKey(page))
}

// scanReleasePending calls fn with each child of rel not moved yet, in block
// key order, from the release page holding its Next. Such a child still
// counts the freed block as a holder, as continueRelease describes.
func (r *Records) scanReleasePending(ctx context.Context, rel *Release, fn func(child *block.BlockRef) error) error {
	cur := newReleaseCursor(r, rel)
	for {
		child, err := cur.child(ctx)
		if err != nil || child == nil {
			return err
		}
		if err := fn(child); err != nil {
			return err
		}
		cur.next++
	}
}

// releaseCursor walks the children of a paged release not moved yet in block
// key order, the order of its block's child list, holding one release page
// at a time.
//
// Invariant: rel and the pages the cursor reads come from rec's one
// transaction snapshot, so the pages hold exactly rel's Children and no
// release step moves Next under the cursor.
type releaseCursor struct {
	// rec are the liveness records it reads the pages from.
	rec *Records
	// rel is the release, nil when none is in progress.
	rel *Release
	// next is the position of the child at the cursor.
	next uint64
	// page holds the children of release page pageNo once read.
	page []*block.BlockRef
	// pageNo is the page page holds.
	pageNo uint64
}

// newReleaseCursor returns a cursor at the first child of rel not moved
// yet.
func newReleaseCursor(rec *Records, rel *Release) *releaseCursor {
	return &releaseCursor{rec: rec, rel: rel, next: rel.GetNext()}
}

// child returns the child at the cursor, or nil past the last one.
func (c *releaseCursor) child(ctx context.Context) (*block.BlockRef, error) {
	// Past the last child there is none.
	if c.next >= c.rel.GetChildren() {
		return nil, nil
	}

	// Read the page holding it, unless the cursor holds it already.
	no := c.next / releasePageChildren
	if c.page == nil || c.pageNo != no {
		page, err := c.rec.getReleasePage(ctx, no)
		if err != nil {
			return nil, err
		}
		c.page, c.pageNo = page, no
	}
	within := c.next % releasePageChildren
	if within >= uint64(len(c.page)) {
		return nil, errReleasePageMissing(c.rel, no)
	}
	return c.page[within], nil
}

// seek moves the cursor from its position to the first child whose key is
// not below key. Children are in block key order, as EncodeChildren and
// mergeList keep them, so a binary search over the children from the cursor
// finds it, reading only the pages it probes. The cursor only reads release
// pages: it never moves the release's Next, which only the release step
// that commits a page of moves writes.
func (c *releaseCursor) seek(ctx context.Context, key []byte) error {
	lo, hi := c.next, c.rel.GetChildren()
	for lo < hi {
		c.next = lo + (hi-lo)/2
		child, err := c.child(ctx)
		if err != nil {
			return err
		}
		if bytes.Compare(BlockKey(child), key) < 0 {
			lo = c.next + 1
		} else {
			hi = c.next
		}
	}
	c.next = lo
	return nil
}

// skipBelow moves the cursor past the children below key and returns the
// child at it, or nil past the last one.
func (c *releaseCursor) skipBelow(ctx context.Context, key []byte) (*block.BlockRef, error) {
	for {
		child, err := c.child(ctx)
		if err != nil || child == nil || bytes.Compare(BlockKey(child), key) >= 0 {
			return child, err
		}
		c.next++
	}
}

// has reports whether the release references the block at key, moving the
// cursor past the children below it. Calls pass keys in ascending order, as
// ScanChildren yields them.
func (c *releaseCursor) has(ctx context.Context, key []byte) (bool, error) {
	child, err := c.skipBelow(ctx, key)
	if err != nil || child == nil {
		return false, err
	}
	return bytes.Equal(BlockKey(child), key), nil
}

// count moves the cursor past every child not above key and returns how
// many equal it, or zero with an error. A child list holds each child once,
// as normalizeChildren and unionChildren keep it, so the count is
// zero or one. Calls pass keys in ascending order.
func (c *releaseCursor) count(ctx context.Context, key []byte) (int64, error) {
	var n int64
	for {
		child, err := c.skipBelow(ctx, key)
		if err != nil {
			return 0, err
		}
		if child == nil || !bytes.Equal(BlockKey(child), key) {
			return n, nil
		}
		n++
		c.next++
	}
}

// errReleasePageMissing reports that release page page of rel is missing or
// ends before the release's Next, naming its block, so an operator can find
// the damaged release. No child moves until a repair.
func errReleasePageMissing(rel *Release, page uint64) error {
	name := "a block of unknown key"
	if ref, _, err := ParseBlockKey(rel.GetBlock()); err == nil {
		name = ref.MarshalString()
	}
	return errors.Errorf("release of %s: page %d of its %d children is missing", name, page, rel.GetChildren())
}
