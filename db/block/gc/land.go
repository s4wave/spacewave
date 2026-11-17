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
	// Mode is what Land does when a move that requires its blocks held finds
	// a block that is not, as requireTree describes.
	Mode HeldMode
	// StageHeld reports whether an open stage holds ref, read in the Land's
	// transaction, for the moves that require their blocks held. Nil reports
	// that none does.
	StageHeld func(ctx context.Context, ref *block.BlockRef) (bool, error)
	// Observed receives, in HeldObserve, the count of blocks not held by
	// reason when any is nonzero, or no counts and the error when the check
	// failed or was cancelled, before the Land's transaction commits, so it
	// also counts a Land that later fails. Nil drops them.
	Observed func(notHeld [NotHeldChild + 1]uint64, err error)
}

// HeldMode is what Land does with a move that requires its blocks held when
// a block it reaches is not.
type HeldMode uint8

const (
	// HeldEnforce refuses the Land with block.ErrRootNotHeld.
	HeldEnforce HeldMode = iota
	// HeldObserve lands the move and reports the blocks to Observed.
	HeldObserve
	// HeldOff checks nothing.
	HeldOff
)

// NotHeldReason says which block of a move that requires its blocks held is
// not held.
type NotHeldReason uint8

const (
	// NotHeldRoot is the block the root moves onto, which the batch does not
	// store: a repeat that stores the tree again in its batch lands.
	NotHeldRoot NotHeldReason = iota
	// NotHeldChild is a child outside the batch of a block the batch stores:
	// one the writer stored in an earlier batch, or never did. Land cannot
	// tell which, so a repeat lands only if it stores the child again.
	NotHeldChild
)

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
// holder. A put of an unrecorded hole counts it as a new block the batch
// gives no holder, so it keeps the block only through an invariant the
// adoption sets: a block without a child list counts one holder more than
// its namers. Turning frees on recounts (SetFrees), which restores that for
// every hole, so a hole lost while frees were off survives its put. A block
// lost with its list after that recount and put again before any audit
// records it is the residual window: the put leaves it one holder short, the
// collector frees it once the put ages past the grace period while its
// parents still name it, and an audit before then reports the undercount. A
// restore of lost blocks therefore audits first, as the payload restore
// does, or opens the Volume once with frees off, so frees turn on again
// through the recount.
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
// that the counts owe a recount, which the flip reads. It refuses a hold onto
// a block the free parked with an AbsentError, and a put of one gives birth
// to it again, as Records.repark describes. While Sweeping, Land
// refuses a hold onto an unmarked block with an AbsentError, and a put of
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
// one, and per listed block it holds or puts, its count record and, for a
// zero one, its parked key; while Sweeping, one run entry read per
// unverified list it meets.
// When it holds a stored block, a root moving onto one or a put of one
// again, it also seeks the cascade records and reads each such block's count
// record until one is at zero, to set the hold mark Collector describes. It
// writes each born block's list, one journal record and the state record. The
// pairs are eager, as Journal.tla models them: the collector nets each later,
// and a shared child's up cancels the down its old parent's cascade gives it
// when both net in one slice. A move that requires its blocks held adds the
// reads requireTree describes.
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
		touched:  make(map[string][]*block.BlockRef),
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

	// Require the trees the strict moves reach held.
	for i, move := range moves {
		if err := l.requireMoveHeld(ctx, move, held[i]); err != nil {
			return nil, err
		}
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
	// repark reports that the free parked the block, so the put gives birth
	// to it again.
	repark bool
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
	// touched maps the block keys of the entries already stored, whose put
	// the journal record's again list names, to the refs each put names.
	touched map[string][]*block.BlockRef
	// stored are the first entries of the blocks stored with a child list,
	// whose lists markStored checks.
	stored []*block.PutBatchEntry
	// onStored are the block keys of the stored blocks a root moves onto,
	// and, while Open, of the unverified stored blocks a born block holds:
	// the front may pop one at zero on the free proof before the hold nets,
	// so the hold mark covers it as it covers a root. With the touched
	// blocks, they are the blocks the Land holds that the collector may have
	// at zero, which decide the hold mark.
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

// stageHeld is BucketRoots.StageHeld.
type stageHeld = func(ctx context.Context, ref *block.BlockRef) (bool, error)

// checkRoots refuses a root named twice, or one whose Previous differs from
// the block it holds, and returns the block each root holds.
func (l *lander) checkRoots(ctx context.Context, bucketID string, roots []block.NamedRoot) ([]*block.BlockRef, error) {
	// A change that moves one root name twice is refused whole, before
	// anything lands; the name set keeps the check linear in the roots.
	held := make([]*block.BlockRef, len(roots))
	names := make(map[string]struct{}, len(roots))
	for i, root := range roots {
		// Refuse a name the change moves twice.
		if _, ok := names[root.Name]; ok {
			return nil, errors.Errorf("root %q moves twice in one change", root.Name)
		}
		names[root.Name] = struct{}{}

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
			l.touched[key] = entry.Refs
		case putStored:
			l.stored = append(l.stored, entry)
			l.touched[key] = entry.Refs
		default:
			b := &bornBlock{ref: entry.Ref, children: normalizeChildren(entry.Refs), reborn: kind == putReborn, repark: kind == putRepark}
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
		b.reput = i < len(existed) && existed[i] && !b.reborn && !b.repark
		if b.repark {
			released, err := l.rec.repark(ctx, l.state, BlockKey(b.ref))
			if err != nil {
				return nil, err
			}
			if l.journal {
				l.down = append(l.down, released...)
			}
		}
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
	// putRepark is a block the free parked while Open, or one without a
	// list left with a zero count: the put gives birth to it again, as
	// Records.repark describes.
	putRepark
)

// classifyPuts checks and names each put entry, building an empty ref from
// its data, and calls fn with the first entry of each block the entries
// put and what the records hold for it, skipping nil entries and
// tombstones. It reads whether the Volume records any hole once, at the
// first block without a child list, and a block's hole record only when one
// does, so a put into a Volume without a hole, the usual case, reads no hole
// record per block. Until the generation is Settled it reads whether each
// listed block is hidden, as Records.Hidden describes.
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
			kind, err := listedPut(ctx, rec, entry.Ref)
			if err != nil {
				return err
			}
			fn(entry, kind)
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
			continue
		}
		kind, err := unlistedPut(ctx, rec, key)
		if err != nil {
			return err
		}
		fn(entry, kind)
	}
	return nil
}

// unlistedPut returns what a put of the block at key, which has neither a
// child list nor a hole record, does. It keeps this invariant: a put leaves
// no record that lets the collector free or sweep the block it stores
// without a later release. A zero count is such a record, left when the
// store lost a Dead block's list while its count, queue entry, front entry,
// parked key and run entry outlived it. The put reparks such a block, as
// Records.repark describes, or, when it is unmarked while Sweeping, gives
// birth to it again; either way it takes no extra holder for bytes stored
// without a list, since its count recorded that nothing else holds it. It
// reads the count record, an indexed point read, and only for a zero one
// the generation and, while Sweeping, the run entry.
func unlistedPut(ctx context.Context, rec *Records, key []byte) (putKind, error) {
	// Read the count; a block with none or with holders is born.
	value, found, err := rec.tx.Get(ctx, rec.key(kindCount, key))
	if err != nil || !found {
		return putBorn, err
	}
	holders, _, err := decodeHolders(value)
	if err != nil || holders != 0 {
		return putBorn, err
	}

	// An unmarked block is born again; any other leaves its zero count.
	unmarked, err := rec.Unmarked(ctx, key)
	if err == nil && unmarked {
		return putReborn, nil
	}
	return putRepark, err
}

// listedPut returns what a put of ref, a block with a child list, does: a
// hidden block is born again, unmarked while Sweeping or parked while Open,
// and any other is stored already.
func listedPut(ctx context.Context, rec *Records, ref *block.BlockRef) (putKind, error) {
	// A block that reads present is stored already.
	hidden, err := rec.Hidden(ctx, ref)
	if err != nil || !hidden {
		return putStored, err
	}

	// A hidden one is parked while Open and unmarked otherwise.
	gen, err := rec.generation(ctx)
	if err == nil && gen == Generation_GENERATION_OPEN {
		return putRepark, nil
	}
	return putReborn, err
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
// reports whether the store lists it: it refuses a hidden one with an
// AbsentError, as Records.Hidden describes, and while Open it pushes an
// unverified ref onto the front. While Settled it reads only whether the
// list exists.
func (l *lander) holdStored(ctx context.Context, ref *block.BlockRef) (bool, error) {
	// Read the list, and while Settled no more.
	key := BlockKey(ref)
	if l.gen == Generation_GENERATION_SETTLED {
		return l.rec.HasChildren(ctx, ref)
	}
	listed, marked, err := l.rec.listState(ctx, key)
	if err != nil || !listed {
		return listed, err
	}

	// Refuse a hidden block. A marked block may die and park, and the flip
	// unmarks a parked one.
	hidden, err := l.rec.Hidden(ctx, ref)
	if err != nil {
		return true, err
	}
	if hidden {
		return true, &AbsentError{Ref: ref}
	}

	// Push an unverified block while Open, which the hold mark covers.
	if marked || l.gen != Generation_GENERATION_OPEN {
		return true, nil
	}
	l.onStored = append(l.onStored, key)
	return true, l.rec.tx.Set(ctx, l.rec.key(kindFront, key), nil)
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

// requireMoveHeld requires held, as move.Mode says, the tree each root of
// move reaches that requires its blocks held and moves onto a new block. held
// are the blocks the roots held before.
func (l *lander) requireMoveHeld(ctx context.Context, move BucketRoots, held []*block.BlockRef) error {
	// Off checks nothing.
	if move.Mode == HeldOff {
		return nil
	}

	// Walk each strict root that moves onto a new block, refusing the first
	// block not held, or counting each one while observing, until a read
	// fails.
	var counts [NotHeldChild + 1]uint64
	notHeld := func(reason NotHeldReason, ref *block.BlockRef) error {
		if move.Mode == HeldEnforce {
			return errors.Wrap(block.ErrRootNotHeld, ref.MarshalString())
		}
		counts[reason]++
		return nil
	}
	var err error
	for i, root := range move.Roots {
		if root.RequireHeld && !root.Ref.GetEmpty() && !SameRef(root.Ref, held[i]) {
			if err = l.requireTree(ctx, root.Ref, move.StageHeld, notHeld); err != nil {
				break
			}
		}
	}

	// Enforcing refuses on the error. Observing lands as with the check off,
	// and reports what it counted, or only the error that stopped it.
	if move.Mode == HeldEnforce {
		return err
	}
	if err != nil {
		counts = [NotHeldChild + 1]uint64{}
	}
	if move.Observed != nil && (err != nil || counts != [NotHeldChild + 1]uint64{}) {
		move.Observed(counts, err)
	}
	return nil
}

// requireTree requires the tree a strict root on ref reaches held, as far as
// the batch carries it, and passes each block that is not to notHeld. A
// root on a block the batch does not store requires that block held. From a
// block the batch stores, born or put again, the walk visits each child: a
// child the batch stores is walked the same way, and any other, a boundary
// child, must be held. A block is held when the records hold it, or staged
// reports that an open stage does.
//
// Invariants. The check keeps an accepted root from naming a block the
// collector freed:
//
//  1. It reads Records.Held, never GetHolders: GetHolders reports one holder
//     for a block with no count record, missing ones included. Held refuses
//     a missing block, one without a child list, and one at zero holders,
//     Dying or Dead, whose children release at zero may already have freed
//     (Collector, invariant 2). Under HoldUntilFree a stored Dead block
//     still holds its children, so a boundary child under it reads Held and
//     the move lands where release at zero refuses it; once the collector
//     frees the block, and then its children, none has a list and the move
//     is refused. A block with a list and no count record carries its
//     implied holder: netting writes a zero record whenever a block reaches
//     zero, and a free deletes both the list and the record, so neither
//     reads Held. A Volume never adopted, such as one of format 7 or 8 whose
//     collector does not free, nets nothing and has no zero records: there
//     the check refuses only a missing block or one with no list.
//  2. A held block holds its whole tree only by Collector invariant 1: a
//     block with no holders holds nothing. The walk stops at a held block,
//     so it does not cover a block another hold revived from Dead over a
//     child the collector freed before the revival: that block reads Held.
//     Held at the boundary is trusted, and every hold that does not require
//     its blocks held can break that trust: any root move without
//     RequireHeld, a stage root, or a put of a block naming the Dead block as
//     a child can land on a stored Dead block whose child was freed, which
//     happens only with frees on. Today only a Space's admit, moving its
//     payloads root, sets RequireHeld; every other hold is of this kind, such
//     as replay's payload moves, the payload reset and the World head move.
//  3. A put proves the block's own bytes, never its children: a block the
//     batch stores, born or put again, may have been Dead with its children
//     freed. So the requirement follows the batch down to its boundary, and
//     the walk reads nothing for a block the batch stores. A boundary block
//     may read at zero, since netting adds the holders a revival gives it
//     only once its record is Grace old; an open stage then proves it, as
//     every stage put lands a block's children before the block, with its
//     stage roots, so a block a stage root holds is stored whole. Any open
//     stage of the process is proof, not only the writer's. staged reads the
//     stage's root in this transaction, and a stage released before it holds
//     nothing. The stage is proof only at the instant of this Land; from it
//     on the move's own holders keep the block, so a later release of the
//     stage changes nothing. A stage holds a block only while it is a stage
//     root: a later batch of the stage that names it as a child releases
//     that root, so an interior block of a staged tree at netted zero is
//     refused unless its own stage root remains.
//  4. A root that keeps its block checks nothing: the name holds the block.
//  5. Observing never fails the Land: the check only reads, and a check that
//     errors or is cancelled counts nothing, and the Land proceeds as with
//     the check off; only HeldEnforce returns the error.
//
// Cost: nothing for a block the batch stores; one child list and one count
// record read for each distinct boundary block, and staged's reads for one
// not held.
func (l *lander) requireTree(ctx context.Context, ref *block.BlockRef, staged stageHeld, notHeld func(NotHeldReason, *block.BlockRef) error) error {
	// A root the batch does not store must be held itself.
	if _, ok := l.carried(ref); !ok {
		return l.requireHeld(ctx, ref, staged, NotHeldRoot, notHeld)
	}

	// Walk the blocks the batch stores, requiring each boundary child held.
	walk := []*block.BlockRef{ref}
	seen := map[string]struct{}{string(BlockKey(ref)): {}}
	for len(walk) != 0 {
		children, _ := l.carried(walk[len(walk)-1])
		walk = walk[:len(walk)-1]
		for _, child := range children {
			if child.GetEmpty() {
				continue
			}
			key := string(BlockKey(child))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			if _, ok := l.carried(child); ok {
				walk = append(walk, child)
			} else if err := l.requireHeld(ctx, child, staged, NotHeldChild, notHeld); err != nil {
				return err
			}
		}
	}
	return nil
}

// carried returns the children of ref when the batch stores it, born or put
// again.
func (l *lander) carried(ref *block.BlockRef) ([]*block.BlockRef, bool) {
	key := BlockKey(ref)
	if b, ok := l.born[string(key)]; ok {
		return b.children, true
	}
	children, ok := l.touched[string(key)]
	return children, ok
}

// requireHeld passes ref to notHeld for reason unless the records hold it or
// staged reports that an open stage does.
func (l *lander) requireHeld(ctx context.Context, ref *block.BlockRef, staged stageHeld, reason NotHeldReason, notHeld func(NotHeldReason, *block.BlockRef) error) error {
	held, err := l.rec.Held(ctx, ref)
	if err != nil || held {
		return err
	}
	if staged != nil {
		if held, err = staged(ctx, ref); err != nil || held {
			return err
		}
	}
	return notHeld(reason, ref)
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
	// Under HoldUntilFree a free deletes its block as its paged release
	// starts, so a put of that block during the release is born and holds no
	// stored block, and the children the release took to zero regain it only
	// once this record's up pairs net. Mark a Land with up pairs while a
	// release is in progress, as a put again of the Dying block marks under
	// release at zero. It reads nothing: the state is already read.
	if len(l.up) != 0 && state.GetRelease() != nil && state.GetHoldRule() == HoldRule_HOLD_RULE_HOLD_UNTIL_FREE {
		return true, nil
	}

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
