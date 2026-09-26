package kvtx_block_okra

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// maxRootLevel bounds the root page level. Every other page sits below it.
const maxRootLevel = 254

// treeBuilder packs sorted entries into pages as their page boundaries arrive.
// Each finished page is written at once and referenced by ref, so a build
// holds only the open page of each level.
type treeBuilder struct {
	// writePage stores a finished page and returns its reference.
	writePage func(*Page) (*block.BlockRef, error)
	// levels holds the open page of each level, indexed by page level.
	levels []treeLevel
}

// treeLevel is the open page of one level.
type treeLevel struct {
	// entries are the staged entries of the open page. The builder owns them
	// until their page takes them.
	entries []*Entry
	// built reports whether the level already built a page.
	built bool
}

// newTreeBuilder starts a tree whose first leaf is the anchor entry.
func newTreeBuilder(writePage func(*Page) (*block.BlockRef, error)) *treeBuilder {
	anchor := &Entry{Anchor: true, Hash: mustAnchorHash()}
	return &treeBuilder{
		writePage: writePage,
		levels:    []treeLevel{{entries: []*Entry{anchor}}},
	}
}

// add stages an entry on the level, first building the open page when the
// entry starts a new one.
func (b *treeBuilder) add(level uint32, entry *Entry) error {
	if level == uint32(len(b.levels)) { //nolint:gosec // levels are bounded by maxRootLevel.
		b.levels = append(b.levels, treeLevel{})
	}
	if len(b.levels[level].entries) != 0 && isBoundary(entry.GetHash()) {
		if err := b.buildPage(level, entry.GetKey()); err != nil {
			return err
		}
	}
	b.levels[level].entries = append(b.levels[level].entries, entry)
	return nil
}

// buildPage writes the level's open page with the upper bound and stages the
// entry referencing it on the next level.
func (b *treeBuilder) buildPage(level uint32, upper []byte) error {
	if level >= maxRootLevel {
		return errors.New("okra tree exceeded maximum height")
	}
	entries := b.levels[level].entries
	page, ref, err := b.createPage(level, entries, upper)
	if err != nil {
		return err
	}
	parentHash, err := hashEntryRange(page.GetEntries())
	if err != nil {
		return err
	}
	parent := &Entry{
		Anchor:   page.GetStartsAtAnchor(),
		Key:      slices.Clone(entries[0].GetKey()),
		Hash:     parentHash,
		Size:     page.GetSize(),
		ChildRef: ref,
	}
	clear(entries)
	b.levels[level].entries = entries[:0]
	b.levels[level].built = true
	return b.add(level+1, parent)
}

// finish writes the open pages bottom-up. It returns the root page reference,
// the entry describing the root page, and the root page level. The first
// level above the leaves to receive a single entry holds the root page.
func (b *treeBuilder) finish() (*block.BlockRef, *Entry, uint32, error) {
	for level := uint32(0); ; level++ {
		open := b.levels[level]
		if level != 0 && !open.built && len(open.entries) == 1 {
			_, ref, err := b.createPage(level, open.entries, nil)
			return ref, open.entries[0], level, err
		}
		if err := b.buildPage(level, nil); err != nil {
			return nil, nil, 0, err
		}
	}
}

// createPage writes one page over the staged entries. The page takes the
// entries without copying them.
func (b *treeBuilder) createPage(level uint32, entries []*Entry, upper []byte) (*Page, *block.BlockRef, error) {
	page := &Page{
		Level:          level,
		UpperBound:     slices.Clone(upper),
		StartsAtAnchor: entries[0].GetAnchor(),
		Entries:        slices.Clone(entries),
	}
	for _, entry := range entries {
		page.Size += entry.GetSize()
		if !entry.GetAnchor() && len(page.LowerBound) == 0 {
			page.LowerBound = slices.Clone(entry.GetKey())
		}
	}
	pageHash, err := hashPage(page)
	if err != nil {
		return nil, nil, err
	}
	page.PageHash = pageHash
	ref, err := b.writePage(page)
	if err != nil {
		return nil, nil, err
	}
	return page, ref, nil
}

// writeStagedPage writes a finished page into the staging store of the tree
// cursor's transaction and returns its reference.
func writeStagedPage(ctx context.Context, tree *block.Cursor, page *Page) (*block.BlockRef, error) {
	cursor := stagedCursor(ctx, tree)
	cursor.SetBlock(page, true)
	ref, _, err := cursor.GetTransaction().WriteAtRoot(ctx, false, nil)
	return ref, err
}

// mustAnchorHash returns the anchor entry's hash, panicking only if the
// package's own digest fails, which prevents the package from operating.
func mustAnchorHash() []byte {
	hash, err := okraDigest(nil)
	if err != nil {
		panic(err)
	}
	return hash
}
