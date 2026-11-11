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
// blocks, appending one journal record for all of them, in pages of
// journalPagePairs entries when it has more. It returns,
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
// block. Every root moving off a block is a down pair, and so is the implied
// holder of a born block no holder in the batch names: such a put is garbage
// once its record ages past the grace period, unless a later Land holds the
// block first, which a writer must do within the grace period.
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
// counts the block as already stored: it stores the bytes, names the put in
// the record's again list, which restarts the block's grace, and adds the
// holders the batch gives it. A block the store lost together with its list is
// a hole whose parents' lists still name it and whose children's counts still
// include it. The audit records each hole it finds under a live root and
// restores its count from the holders it found, and a put of a recorded hole
// counts it as stored too: Land writes its list back, and its children gain no
// holder. The one gap is a block lost with its list and put again before any
// audit records it: the put counts it as a new block the batch gives no
// holder, which the collector frees once the put ages past the grace period
// while its parents still name it, and an audit before then reports the
// undercount. A restore of lost blocks therefore audits first, as the payload
// restore does.
//
// While the Volume's old generation is Open, Land keeps the settle's front
// invariant, as settle.go describes: it pushes onto the front each child a
// born block names outside the batch, and each block a root moves onto, when
// its list is unverified. A put of a stored block whose list is unverified
// writes the union of the list and the put's refs, marked, since a list may
// lack a child and naming one more only keeps a block longer, takes the block
// off the run and the front, and pushes its unverified children; a refilled
// hole does the same with the list it writes back. A list that gains a child
// journals no holder for it, so on a Volume that keeps a journal Land records
// that the counts owe a recount, which the flip reads. While Sweeping, Land
// refuses a hold onto an unmarked block with an UnmarkedError, and a put of
// one gives birth to it again, as Records.rebirth describes: its old list's
// children outside the run lose the holder it gave them, through down pairs.
// Journal.tla models this with its FrontPop, Stop and RootWalk constants.
//
// A Volume that is not adopted keeps no journal: Land writes the child lists
// and roots and appends nothing, and an adoption counts every holder from
// them. An adopting or adopted Volume refuses a block that names a child whose
// bytes it holds without a child list, and a root onto such a block, because
// the children that block names would hold nothing. While an adoption counts
// the Volume, Land also marks each block it gives birth to and records the
// block each root it moves held when the adoption began, as Adopt reads.
//
// Cost per call: the state record and, when an entry has no list, whether the
// Volume records any hole; per entry, its child list record and, for one
// without a list in a Volume that records a hole, its hole record; per child a born block names outside
// its batch, its child list record and, for an unlisted child, a block
// existence read, with one up pair appended for each, a blind append on reads
// it already makes; per moved root, its entry and its lease index record.
// While the generation is not Settled, each child list record it reads
// returns its value, and per stored block it puts while Open, its list
// record, which it may rewrite, with one list read per child of a rewritten
// one; while Sweeping, one run entry read per unverified list it meets.
// When it holds a stored block, a root moving onto one or a put of one
// again, it also seeks the cascade records and reads each such block's count
// record until one is at zero, to set the hold mark Collector describes. It
// writes each born block's list, one journal record and the state record. The
// pairs are eager, as Journal.tla models them: the collector nets each later,
// and a shared child's up cancels the down its old parent's cascade gives it
// when both net in one slice.
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
		state:    state,
		journal:  state.GetAdoption() != Adoption_ADOPTION_UNADOPTED,
		adopting: state.GetAdoption() == Adoption_ADOPTION_ADOPTING,
		gen:      state.GetGeneration(),
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

	// Mark the unverified lists of the stored blocks it puts again.
	if l.gen == Generation_GENERATION_OPEN {
		for _, entry := range l.stored {
			if err := l.markStored(ctx, entry); err != nil {
				return nil, err
			}
		}
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

	// Append the journal record, then write the state if anything changed it.
	if err := l.appendJournal(ctx, now); err != nil {
		return nil, err
	}
	if l.dirty {
		if err := rec.SetState(ctx, state); err != nil {
			return nil, err
		}
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
	// reborn reports that the block was unmarked, so the put gives birth to
	// it again.
	reborn bool
	// written reports that its child list was written.
	written bool
}

// lander is the state of one Land call.
type lander struct {
	// rec are the liveness records.
	rec *Records
	// blocks stores the block bytes.
	blocks block.StoreOps
	// state is the state record, which Land writes when dirty is set.
	state *LivenessState
	// dirty reports that the Land changed state.
	dirty bool
	// journal reports that the Volume is adopting or adopted.
	journal bool
	// adopting reports that an adoption is counting the Volume, so Land marks
	// the blocks it gives birth to and the roots it moves, as Adopt reads.
	adopting bool
	// gen is the Volume's generation, which decides the settle's barrier.
	gen Generation
	// touched holds the block keys of the entries already stored, whose put
	// the journal record's again list names.
	touched map[string]struct{}
	// stored are the first entries of the blocks stored with a child list,
	// whose lists markStored checks.
	stored []*block.PutBatchEntry
	// onStored are the block keys of the stored blocks a root moves onto,
	// and, while Open, of the unverified stored blocks a born block holds:
	// the front may pop one at zero on the free proof before the hold nets,
	// so the hold mark covers it as it covers a root.
	// With the touched blocks, they are the blocks the Land holds that the
	// collector may have at zero, which decide the hold mark.
	onStored [][]byte
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
	// Name each entry and sort its block: stored, a refilled hole, or born.
	err := classifyPuts(ctx, l.rec, l.blocks, entries, func(entry *block.PutBatchEntry, kind putKind) {
		key := string(BlockKey(entry.Ref))
		switch kind {
		case putRefill:
			l.refills = append(l.refills, entry)
			l.touched[key] = struct{}{}
		case putStored:
			l.stored = append(l.stored, entry)
			l.touched[key] = struct{}{}
		default:
			b := &bornBlock{ref: entry.Ref, children: normalizeChildren(entry.Refs), reborn: kind == putReborn}
			l.born[key] = b
			l.order = append(l.order, b)
		}
	})
	if err != nil {
		return nil, err
	}

	// Store the bytes, which also evicts the tombstones' bytes, then write the
	// child lists of the refilled holes back.
	existed, err := l.blocks.PutBlockBatch(ctx, entries)
	if err != nil {
		return nil, err
	}
	for _, entry := range l.refills {
		if err := l.rec.refill(ctx, entry); err != nil {
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
		b.reput = i < len(existed) && existed[i] && !b.reborn
		if b.reborn {
			released, err := l.rec.rebirth(ctx, l.state, b.ref)
			if err != nil {
				return nil, err
			}
			l.dirty = true
			if l.journal {
				l.down = append(l.down, released...)
			}
		}
		if err := l.rec.PutChildren(ctx, b.ref, b.children); err != nil {
			return nil, err
		}
		if l.adopting {
			if err := l.rec.tx.Set(ctx, l.rec.key(kindAdoptBorn, BlockKey(b.ref)), nil); err != nil {
				return nil, err
			}
		}
	}
	return existed, nil
}

// putKind is what the liveness records hold for a block a put names.
type putKind int

const (
	// putBorn is a block without a child list: the put gives birth to it.
	putBorn putKind = iota
	// putStored is a block with a child list, stored already.
	putStored
	// putRefill is a hole the audit recorded, stored too: the put writes its
	// child list back.
	putRefill
	// putReborn is a block the settle unmarked: the put gives birth to it
	// again, as Records.rebirth describes.
	putReborn
)

// classifyPuts checks and names each put entry, building an empty ref from
// its data, and calls fn with the first entry of each block the entries
// put and what the records hold for it, skipping nil entries and
// tombstones. It reads whether the Volume records any hole once, at the
// first block without a child list, and a block's hole record only when one
// does, so a put into a Volume without a hole, the usual case, reads no hole
// record per block. While the generation is Sweeping it reads each listed
// block's run entry.
func classifyPuts(ctx context.Context, rec *Records, blocks block.StoreOps, entries []*block.PutBatchEntry, fn func(entry *block.PutBatchEntry, kind putKind)) error {
	seen := make(map[string]struct{}, len(entries))
	var holesRead, holes bool
	for _, entry := range entries {
		// Check and name the entry, once per block.
		if entry == nil || entry.Tombstone {
			continue
		}
		if len(entry.Data) == 0 {
			return block.ErrEmptyBlock
		}
		if !entry.RefsKnown {
			return errors.Wrap(block.ErrRefsUnknown, entry.Ref.MarshalString())
		}
		if entry.Ref.GetEmpty() {
			ref, err := block.BuildBlockRef(entry.Data, &block.PutOpts{HashType: blocks.GetHashType()})
			if err != nil {
				return err
			}
			entry.Ref = ref
		}
		key := BlockKey(entry.Ref)
		if _, ok := seen[string(key)]; ok {
			continue
		}
		seen[string(key)] = struct{}{}

		// Read its child list, then whether it is a recorded hole.
		listed, err := rec.HasChildren(ctx, entry.Ref)
		if err != nil {
			return err
		}
		if listed {
			unmarked, err := rec.Unmarked(ctx, key)
			if err != nil {
				return err
			}
			if unmarked {
				fn(entry, putReborn)
			} else {
				fn(entry, putStored)
			}
			continue
		}
		if !holesRead {
			if holes, err = rec.anyKey(ctx, rec.key(kindHole)); err != nil {
				return err
			}
			holesRead = true
		}
		hole := false
		if holes {
			if hole, err = rec.tx.Exists(ctx, rec.key(kindHole, key)); err != nil {
				return err
			}
		}
		if hole {
			fn(entry, putRefill)
		} else {
			fn(entry, putBorn)
		}
	}
	return nil
}

// refill writes back the child list of the recorded hole entry puts, from
// its content, and deletes its hole record. While the generation is Open,
// the block leaves the run and the front, and its unverified children join
// the front.
func (r *Records) refill(ctx context.Context, entry *block.PutBatchEntry) error {
	// Write the list back and drop the hole record.
	key := BlockKey(entry.Ref)
	children := normalizeChildren(entry.Refs)
	if err := r.PutChildren(ctx, entry.Ref, children); err != nil {
		return err
	}
	if err := r.tx.Delete(ctx, r.key(kindHole, key)); err != nil {
		return err
	}
	gen, err := r.generation(ctx)
	if err != nil || gen != Generation_GENERATION_OPEN {
		return err
	}
	return r.markedYoung(ctx, key, EncodeChildren(children), nil)
}

// markStored marks the unverified list of the stored block entry puts
// again, while the generation is Open, as Land describes.
func (l *lander) markStored(ctx context.Context, entry *block.PutBatchEntry) error {
	// A marked list was written from content.
	key := BlockKey(entry.Ref)
	value, listed, err := l.rec.tx.Get(ctx, l.rec.key(kindChildren, key))
	if err != nil || !listed || listMarked(value) {
		return err
	}

	// Write the union, marked, and keep the front invariant.
	packed, gained, _, err := l.rec.mergeList(ctx, key, value, entry.Refs, false)
	if err != nil {
		return err
	}
	if gained && l.journal {
		if err := l.rec.noteRewrite(ctx, l.state); err != nil {
			return err
		}
	}
	return l.rec.markedYoung(ctx, key, packed, nil)
}

// holdStored checks a hold onto ref, a block the batch does not store, and
// reports whether the store lists it: while Open it pushes an unverified
// ref onto the front, and while Sweeping it refuses an unmarked one with an
// UnmarkedError. While Settled it reads only whether the list exists.
func (l *lander) holdStored(ctx context.Context, ref *block.BlockRef) (bool, error) {
	// Read the list, and while Settled no more.
	key := BlockKey(ref)
	if l.gen == Generation_GENERATION_SETTLED {
		return l.rec.HasChildren(ctx, ref)
	}
	listed, marked, err := l.rec.listState(ctx, key)
	if err != nil || !listed || marked {
		return listed, err
	}

	// Push an unverified block, which the hold mark covers, or refuse an
	// unmarked one.
	if l.gen == Generation_GENERATION_OPEN {
		l.onStored = append(l.onStored, key)
		return true, l.rec.tx.Set(ctx, l.rec.key(kindFront, key), nil)
	}
	unmarked, err := l.rec.Unmarked(ctx, key)
	if err == nil && unmarked {
		err = &UnmarkedError{Ref: ref}
	}
	return true, err
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

			// Any other child passes the settle's check, and gains a holder
			// unless the Volume keeps no journal.
			listed, err := l.holdStored(ctx, child)
			if err != nil {
				return err
			}
			if !l.journal {
				continue
			}
			if !listed {
				if err := l.requireListed(ctx, child); err != nil {
					return err
				}
			}
			l.up = append(l.up, BlockKey(child))
		}
	}
	return nil
}

// requireListed refuses ref, a block without a child list, when the store
// holds its bytes: the children it names would hold nothing. A block the
// store lacks, such as a child stored after its parent, passes; its holders
// count from its birth.
func (l *lander) requireListed(ctx context.Context, ref *block.BlockRef) error {
	found, err := l.blocks.GetBlockExists(ctx, ref)
	if err == nil && found {
		err = errors.Wrap(block.ErrRefsUnknown, ref.MarshalString())
	}
	return err
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
	listed, err := l.holdStored(ctx, root.Ref)
	if err != nil {
		return err
	}
	if listed {
		if l.journal {
			l.up = append(l.up, BlockKey(root.Ref))
			l.onStored = append(l.onStored, BlockKey(root.Ref))
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
func (l *lander) appendJournal(ctx context.Context, now time.Time) error {
	// Without a journal there is nothing to append.
	state := l.state
	if !l.journal {
		return nil
	}

	// Count each born block's holders beyond its implied one. A block whose
	// bytes were stored without a child list may have holders the store
	// cannot see, so it keeps one more and never frees on a guess. A block
	// the batch gives no holder takes its implied one back with a down pair,
	// so an unheld put is garbage once the record ages past the grace period,
	// unless a parent or a root holds it first.
	for _, b := range l.order {
		extra := b.holders - 1
		if b.reput {
			extra++
		}
		for range max(extra, 0) {
			l.up = append(l.up, BlockKey(b.ref))
		}
		if extra < 0 {
			l.down = append(l.down, BlockKey(b.ref))
		}
	}

	// Name each put of a block already stored in the again list. It changes
	// no holder, but a block it finds at zero takes the journal's end as its
	// ripe sequence again once the record nets, so a writer that puts a tree
	// across several batches restarts the grace of each block it puts again.
	again := make([][]byte, 0, len(l.touched))
	for _, key := range slices.Sorted(maps.Keys(l.touched)) {
		again = append(again, []byte(key))
	}
	if len(l.up) == 0 && len(l.down) == 0 && len(again) == 0 {
		return nil
	}

	// Write the record at the next sequence numbers, stamped with the clock,
	// in pages of at most journalPagePairs entries, the up pairs first, then
	// the down pairs, then the again list: the collector nets the pages in
	// order, as it nets one record's entries in that order, and a slice
	// decodes only the pages it works on.
	at := now.UnixMilli()
	held := slices.Concat(l.onStored, again)
	for ups, downs := l.up, l.down; len(ups)+len(downs)+len(again) != 0; state.NextSeq++ {
		page := &JournalRecord{AtUnixMilli: at}
		n := min(len(ups), journalPagePairs)
		page.Up, ups = ups[:n], ups[n:]
		n = min(len(downs), journalPagePairs-len(page.Up))
		page.Down, downs = downs[:n], downs[n:]
		n = min(len(again), journalPagePairs-len(page.Up)-len(page.Down))
		page.Again, again = again[:n], again[n:]
		if err := l.rec.PutJournal(ctx, state.GetNextSeq(), page); err != nil {
			return err
		}
	}

	// Set the hold mark past the record when it holds a block the collector
	// may have at zero.
	mark, err := l.holdsZero(ctx, state, held)
	if err != nil {
		return err
	}
	if mark {
		state.HoldMark = state.GetNextSeq()
	}
	l.dirty = true
	return nil
}

// holdsZero reports whether the Land holds a stored block, of held, that the
// collector has at zero holders, or may have through a parent: a root moves
// onto it or the batch puts it again, and either a cascade is due or in
// progress, so a block below a block with holders may have none, or the
// block's own count record is a zero record. Such a block's children lost
// its holder when it reached zero, and get it back only through the re-hold
// that runs once this record nets, which may come after a child's own ripe
// sequence; the hold mark makes the free wait for it.
//
// Cost: nothing for a Land that holds no stored block, the usual commit of
// a born tree; otherwise one count record read per block until one is at
// zero, since the state it already read counts the cascade records.
func (l *lander) holdsZero(ctx context.Context, state *LivenessState, held [][]byte) (bool, error) {
	// A Land that holds no stored block sets no mark.
	if len(held) == 0 {
		return false, nil
	}

	// Mark while a cascade is due or in progress. The free also waits while
	// one is, and a block a cascade brings to zero after this record lands
	// ripens past it, so no known path leaves this mark the only guard; it
	// stays as defense in depth, as the model's hold mark does.
	settled, err := l.rec.settled(ctx, state)
	if err != nil || !settled {
		return !settled, err
	}

	// Mark when a held block is at zero.
	for _, key := range held {
		ref, _, err := ParseBlockKey(key)
		if err != nil {
			return false, err
		}
		holders, _, err := l.rec.GetHolders(ctx, ref)
		if err != nil || holders == 0 {
			return holders == 0, err
		}
	}
	return false, nil
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
