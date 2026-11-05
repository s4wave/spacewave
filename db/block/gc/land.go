package block_gc

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// BucketRoots are the root moves of one bucket a Land makes.
type BucketRoots struct {
	// BucketID is the bucket.
	BucketID string
	// Roots are the moves.
	Roots []block.NamedRoot
}

// Land stores entries and moves the roots of each bucket in moves in one
// write transaction, with the liveness records in rec and the block bytes in
// blocks, appending one journal record for all of them. It returns,
// for each entry, whether blocks held its bytes before. It is the one write
// path that changes holders, so Journal.tla's Land action is this function.
//
// A block is stored, in the liveness sense, while its child list exists. An
// entry whose block has no child list is born: Land writes its list. A born
// block has one holder implied by its record, which is the first holder this
// transaction gives it: a born parent in the batch that names it, or a root
// that moves onto it. Every further holder is an up pair in the journal
// record: a child of a born block that the batch does not store, a root moving
// onto a block stored before, and the second and later holders of a born
// block. Every root moving off a block is a down pair. A born block that no
// holder in the batch names keeps its implied holder until the audit finds it,
// so a put outside a batch that names it leaks and never frees.
//
// A root's Previous must match the block the name holds, or Land writes
// nothing and returns block.ErrRootMoved. A root must name a block the store
// holds or the batch stores.
//
// A tombstone evicts a block's bytes and its proof but keeps its child list
// and count, so a cache that evicts a live block leaves a hole the collector
// steps over, and only the collector frees.
//
// A block's place in the graph is its child list record, not its bytes. A
// put that finds the list without the bytes, such as a block a cache evicted,
// counts the block as already stored: it stores the bytes, journals the put
// as an up and a down pair, which restarts the block's grace, and adds the
// holders the batch gives it. A block the store lost together with its list is
// a hole whose parents' lists still name it and whose children's counts still
// include it. The audit records each hole it finds under a live root and
// restores its count from the holders it found, and a put of a recorded hole
// counts it as stored too: Land writes its list back, and its children gain no
// holder. The one gap is a block lost with its list and put again before any
// audit records it: the put counts it as a new block with one implied holder,
// fewer than its parents give it, and the next audit reports the undercount.
//
// A Volume that is not adopted keeps no journal: Land writes the child lists
// and roots and appends nothing, and an adoption counts every holder from
// them. An adopting or adopted Volume refuses a block that names a child whose
// bytes it holds without a child list, and a root onto such a block, because
// the children that block names would hold nothing. While an adoption counts
// the Volume, Land also marks each block it gives birth to and records the
// block each root it moves held when the adoption began, as Adopt reads.
//
// Cost per call: the state record; per entry, its child list record and, for
// one without a list, its hole record; per child a born block names outside
// its batch, its child list record and, for an unlisted child, a block
// existence read, with one up pair appended for each, a blind append on reads
// it already makes; per moved root, its entry and its lease index record. It
// writes each born block's list, one journal record and the state record. The
// pairs are eager, as Journal.tla models them: the collector nets each later,
// writing the shared child's count, and decrements it again in the cascade
// that frees the old parent.
func Land(
	ctx context.Context,
	rec *Records,
	blocks block.StoreOps,
	now time.Time,
	entries []*block.PutBatchEntry,
	moves ...BucketRoots,
) ([]bool, error) {
	// Read the journal position and adoption.
	state, err := rec.GetState(ctx)
	if err != nil {
		return nil, err
	}
	l := &lander{
		rec:      rec,
		blocks:   blocks,
		journal:  state.GetAdoption() != Adoption_ADOPTION_UNADOPTED,
		adopting: state.GetAdoption() == Adoption_ADOPTION_ADOPTING,
		born:     make(map[string]*bornBlock),
		touched:  make(map[string]struct{}),
	}

	// Check every root before writing anything.
	held := make([][]*block.BlockRef, len(moves))
	for i, move := range moves {
		if held[i], err = l.checkRoots(ctx, move.BucketID, move.Roots); err != nil {
			return nil, err
		}
	}

	// Store the entries and find the blocks they give birth to.
	existed, err := l.putEntries(ctx, entries)
	if err != nil {
		return nil, err
	}

	// Count the holders the born blocks give their children.
	if err := l.holdChildren(ctx); err != nil {
		return nil, err
	}

	// Move the roots.
	for i, move := range moves {
		for j, root := range move.Roots {
			if err := l.moveRoot(ctx, move.BucketID, root, held[i][j]); err != nil {
				return nil, err
			}
		}
	}

	// Append the journal record.
	if err := l.appendJournal(ctx, state, now); err != nil {
		return nil, err
	}
	return existed, nil
}

// bornBlock is a block a Land stores for the first time.
type bornBlock struct {
	// ref is the block.
	ref *block.BlockRef
	// children is its child list.
	children []*block.BlockRef
	// holders counts the holders the batch gives it.
	holders int
	// reput reports that the store held its bytes without a child list, so
	// holders the store cannot see may name it.
	reput bool
	// written reports that its child list was written.
	written bool
}

// lander is the state of one Land call.
type lander struct {
	// rec are the liveness records.
	rec *Records
	// blocks stores the block bytes.
	blocks block.StoreOps
	// journal reports that the Volume is adopting or adopted.
	journal bool
	// adopting reports that an adoption is counting the Volume, so Land marks
	// the blocks it gives birth to and the roots it moves, as Adopt reads.
	adopting bool
	// touched holds the block keys of the entries already stored, whose put
	// the journal records as an up and a down pair.
	touched map[string]struct{}
	// refills are the entries that put a recorded hole, whose child lists
	// Land writes back.
	refills []*block.PutBatchEntry
	// born are the born blocks by block key.
	born map[string]*bornBlock
	// order are the born blocks in entry order.
	order []*bornBlock
	// up and down are the journal record's pairs.
	up, down [][]byte
}

// checkRoots refuses a root named twice, or one whose Previous differs from
// the block it holds, and returns the block each root holds.
func (l *lander) checkRoots(ctx context.Context, bucketID string, roots []block.NamedRoot) ([]*block.BlockRef, error) {
	held := make([]*block.BlockRef, len(roots))
	for i, root := range roots {
		// Refuse a name the change moves twice.
		for _, prior := range roots[:i] {
			if prior.Name == root.Name {
				return nil, errors.Errorf("root %q moves twice in one change", root.Name)
			}
		}

		// Compare the held block with the writer's Previous.
		entry, err := l.rec.GetRoot(ctx, bucketID, root.Name)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			if held[i], _, err = ParseBlockKey(entry.GetRef()); err != nil {
				return nil, err
			}
		}
		if !SameRef(held[i], root.Previous) {
			return nil, block.RootMoved(root.Name)
		}
	}
	return held, nil
}

// putEntries stores the entries' bytes and the child lists of the blocks
// they give birth to, and evicts the tombstones' bytes.
func (l *lander) putEntries(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Name each entry and find the blocks without a child list.
	for _, entry := range entries {
		if entry == nil || entry.Tombstone {
			continue
		}
		if len(entry.Data) == 0 {
			return nil, block.ErrEmptyBlock
		}
		if !entry.RefsKnown {
			return nil, errors.Wrap(block.ErrRefsUnknown, entry.Ref.MarshalString())
		}
		if entry.Ref.GetEmpty() {
			ref, err := block.BuildBlockRef(entry.Data, &block.PutOpts{HashType: l.blocks.GetHashType()})
			if err != nil {
				return nil, err
			}
			entry.Ref = ref
		}
		key := string(BlockKey(entry.Ref))
		if _, ok := l.born[key]; ok {
			continue
		}
		stored, err := l.rec.HasChildren(ctx, entry.Ref)
		if err != nil {
			return nil, err
		}
		if stored {
			l.touched[key] = struct{}{}
			continue
		}

		// A recorded hole is stored too: the put refills it.
		hole, err := l.rec.IsHole(ctx, entry.Ref)
		if err != nil {
			return nil, err
		}
		if hole {
			l.touched[key] = struct{}{}
			l.refills = append(l.refills, entry)
			continue
		}
		b := &bornBlock{ref: entry.Ref, children: normalizeChildren(entry.Refs)}
		l.born[key] = b
		l.order = append(l.order, b)
	}

	// Store the bytes, which also evicts the tombstones' bytes, then write the
	// child lists of the refilled holes back.
	existed, err := l.blocks.PutBlockBatch(ctx, entries)
	if err != nil {
		return nil, err
	}
	for _, entry := range l.refills {
		if err := l.rec.PutChildren(ctx, entry.Ref, entry.Refs); err != nil {
			return nil, err
		}
		if err := l.rec.DeleteHole(ctx, entry.Ref); err != nil {
			return nil, err
		}
	}

	// Write each born block's child list once, and drop the evicted blocks'
	// proofs. A born block whose bytes were stored already is a reput.
	for i, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.Tombstone {
			if err := l.rec.DeleteProof(ctx, entry.Ref); err != nil {
				return nil, err
			}
			continue
		}
		b, ok := l.born[string(BlockKey(entry.Ref))]
		if !ok || b.written {
			continue
		}
		b.written = true
		b.reput = i < len(existed) && existed[i]
		if err := l.rec.PutChildren(ctx, b.ref, b.children); err != nil {
			return nil, err
		}
		if l.adopting {
			if err := l.rec.MarkAdoptBorn(ctx, b.ref); err != nil {
				return nil, err
			}
		}
	}
	return existed, nil
}

// holdChildren counts the holder each born block gives each child: an
// implied holder of a child the batch stores too, and an up pair for any
// other.
func (l *lander) holdChildren(ctx context.Context) error {
	for _, b := range l.order {
		for _, child := range b.children {
			// A child the batch stores takes the parent as a batch holder.
			if c, ok := l.born[string(BlockKey(child))]; ok {
				c.holders++
				continue
			}

			// Any other child gains a holder, unless the Volume keeps no
			// journal.
			if !l.journal {
				continue
			}
			if err := l.requireListed(ctx, child); err != nil {
				return err
			}
			l.up = append(l.up, BlockKey(child))
		}
	}
	return nil
}

// requireListed refuses a block whose bytes the store holds without a child
// list: the children it names would hold nothing. A block the store lacks,
// such as a child stored after its parent, passes; its holders count from its
// birth.
func (l *lander) requireListed(ctx context.Context, ref *block.BlockRef) error {
	// A block with a child list passes.
	listed, err := l.rec.HasChildren(ctx, ref)
	if err != nil || listed {
		return err
	}

	// Refuse one whose bytes are stored without it.
	found, err := l.blocks.GetBlockExists(ctx, ref)
	if err != nil {
		return err
	}
	if found {
		return errors.Wrap(block.ErrRefsUnknown, ref.MarshalString())
	}
	return nil
}

// moveRoot moves one named root from held to root.Ref, counting the holder it
// adds and the one it removes.
func (l *lander) moveRoot(ctx context.Context, bucketID string, root block.NamedRoot, held *block.BlockRef) error {
	// A root that keeps its block changes no holder; it only renews its
	// expiry.
	entry := &RootEntry{Ref: BlockKey(root.Ref), ExpiresUnixMilli: unixMilli(root.Expires)}
	if !root.Ref.GetEmpty() && SameRef(root.Ref, held) {
		return l.rec.SetRoot(ctx, bucketID, root.Name, entry)
	}

	// Note the block the name held when a running adoption began.
	if l.adopting {
		if err := l.rec.NoteAdoptRoot(ctx, bucketID, root.Name, held); err != nil {
			return err
		}
	}

	// Count the holder the root adds to its new block.
	if !root.Ref.GetEmpty() {
		if err := l.holdRoot(ctx, root); err != nil {
			return err
		}
	}

	// Count the holder it takes from its old block.
	if !held.GetEmpty() && l.journal {
		l.down = append(l.down, BlockKey(held))
	}

	// Write or free the name.
	if root.Ref.GetEmpty() {
		return l.rec.DeleteRoot(ctx, bucketID, root.Name)
	}
	return l.rec.SetRoot(ctx, bucketID, root.Name, entry)
}

// holdRoot counts the holder a root adds to the block it moves onto, which
// must be stored or born in the batch.
func (l *lander) holdRoot(ctx context.Context, root block.NamedRoot) error {
	// A block the batch stores takes the root as a batch holder.
	if b, ok := l.born[string(BlockKey(root.Ref))]; ok {
		b.holders++
		return nil
	}

	// A stored block gains a holder.
	listed, err := l.rec.HasChildren(ctx, root.Ref)
	if err != nil {
		return err
	}
	if listed {
		if l.journal {
			l.up = append(l.up, BlockKey(root.Ref))
		}
		return nil
	}

	// A block stored without a child list may be named only while the Volume
	// keeps no journal; a block the store lacks never.
	found, err := l.blocks.GetBlockExists(ctx, root.Ref)
	if err != nil {
		return err
	}
	switch {
	case !found:
		return errors.Wrapf(block.NotFoundError(root.Ref), "named root %s", root.Name)
	case l.journal:
		return errors.Wrapf(block.ErrRefsUnknown, "named root %s on %s", root.Name, root.Ref.MarshalString())
	}
	return nil
}

// appendJournal appends the record of the holders the Land added beyond each
// born block's implied one, and the ones it removed.
func (l *lander) appendJournal(ctx context.Context, state *LivenessState, now time.Time) error {
	// Without a journal there is nothing to append.
	if !l.journal {
		return nil
	}

	// Count each born block's holders beyond its implied one. A block whose
	// bytes were stored without a child list may have holders the store
	// cannot see, so it keeps one more and never frees on a guess.
	for _, b := range l.order {
		extra := b.holders - 1
		if b.reput {
			extra++
		}
		for range max(extra, 0) {
			l.up = append(l.up, BlockKey(b.ref))
		}
	}

	// Record each put of a block already stored as an up and a down pair. It
	// changes no count, but the collector's tail check keeps the block until
	// the record ages past the grace period, so a writer that puts a tree
	// across several batches restarts the grace of each block it puts again.
	for _, key := range slices.Sorted(maps.Keys(l.touched)) {
		l.up = append(l.up, []byte(key))
		l.down = append(l.down, []byte(key))
	}
	if len(l.up) == 0 && len(l.down) == 0 {
		return nil
	}

	// Write the record at the next sequence number, stamped with the clock.
	rec := &JournalRecord{AtUnixMilli: now.UnixMilli(), Up: l.up, Down: l.down}
	if err := l.rec.PutJournal(ctx, state.GetNextSeq(), rec); err != nil {
		return err
	}
	state.NextSeq++
	return l.rec.SetState(ctx, state)
}

// SameRef reports whether a and b name the same block, or are both empty.
func SameRef(a, b *block.BlockRef) bool {
	if a.GetEmpty() || b.GetEmpty() {
		return a.GetEmpty() == b.GetEmpty()
	}
	return a.EqualsRef(b)
}

// unixMilli returns t in Unix milliseconds, or zero for the zero time.
func unixMilli(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}
