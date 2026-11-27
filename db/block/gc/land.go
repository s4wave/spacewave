package block_gc

import (
	"bytes"
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
	Observed func(notHeld [NotHeldPut + 1]uint64, err error)
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
	// store, or stores only through a copied entry: a repeat that stores the
	// tree again in its batch, from content, lands.
	NotHeldRoot NotHeldReason = iota
	// NotHeldChild is a child outside the batch of a block the batch stores:
	// one the writer stored in an earlier batch, or never did, or one the
	// batch stores only through a copied entry. Land cannot tell which, so a
	// repeat lands only if it stores the child again, from content.
	NotHeldChild
	// NotHeldPut is a block the batch gives birth to and gives no holder:
	// no root moves onto it, no block the batch stores references it, and
	// its count records no holder from before.
	NotHeldPut
)

// Land stores entries and moves the roots of each bucket in moves in one
// write transaction, with the liveness records in rec and the block bytes in
// blocks, appending the holders it adds to the up log and the ones it
// removes to the down log, in pages of logPageKeys keys. It returns, for
// each entry, whether blocks held its bytes before. It is the one write path
// that changes holders.
//
// Invariant: rec and blocks are views of one transaction, so the block bytes
// Land puts and the liveness records it writes commit or discard together.
// A Land that fails leaves neither: the caller discards the transaction, and
// the result it returns with the error.
//
// A block is stored, in the liveness sense, while its child list exists. An
// entry whose block has no child list is born: Land writes its list. A born
// block has one holder implied by its record, which is the first holder this
// transaction gives it: a born parent in the batch that references it, or a
// root that moves onto it. Every further holder is an up: a child of a born
// block that the batch does not store, a root moving onto a block stored
// before, and the second and later holders of a born block. Every root moving
// off a block is a down, and so is the implied holder of a born block no holder
// in the batch references: such a put is garbage once its down ages past the
// grace period, unless a later Land holds the block first, which a writer must
// do within the grace period.
//
// A put of a stored block, one whose list exists, holds it as a born put
// does: an up now and a down for that implied holder, so the block lives at
// least one grace period past the put, and a writer that puts a tree across
// several batches keeps each block it puts again.
//
// Land dequeues each stored block it holds: a root moving onto it, a born
// block or a refilled hole naming it, or a put of it again. It reads their
// count records in the batch that reads the puts' counts and deletes the
// queue entry of each at zero, so the free cannot take a block whose up has
// not netted yet. It dequeues only a block it also gives an up, which nets
// later and lifts the count off zero.
//
// A root's Previous must match the block the name holds, or Land writes nothing
// and returns block.ErrRootMoved. A root must reference a block the store holds
// or the batch stores. A root that references a lease and a block moves only
// while the lease is live at now, or Land writes nothing and returns
// ErrLeaseLapsed, so no root joins a lease the collector may be releasing.
//
// A tombstone evicts a block's bytes and its proof but keeps its child list
// and count, so a cache that evicts a live block leaves a hole the collector
// steps over, and only the collector frees.
//
// A block's place in the graph is its child list record, not its bytes. A put
// that finds the list without the bytes, such as a block a cache evicted,
// counts the block as already stored: it stores the bytes and holds it as
// above. A block the store lost together with its list is a hole whose parents'
// lists still reference it and whose children's counts still include it. The
// audit records each hole it finds under a live root and restores its count
// from the holders it found, and a put of a recorded hole counts it as stored
// too: Land writes its list back, and its children gain no holder. A put of an
// unrecorded hole counts it as a new block the batch gives no holder, so it
// keeps the block only through an invariant the adoption sets: a block without
// a child list counts one holder more than its namers. Turning frees on
// recounts (SetFrees), which restores that for every hole, so a hole lost while
// frees were off survives its put. A block lost with its list after that
// recount and put again before any audit records it is the residual window: the
// put leaves it one holder short, the collector frees it once the put ages past
// the grace period while its parents still reference it, and an audit before
// then reports the undercount. A restore of lost blocks therefore audits first,
// as the payload restore does, or opens the Volume once with frees off, so
// frees turn on again through the recount.
//
// While the Volume's old generation is Open, Land keeps the settle's front
// invariant, as settle.go describes: it pushes onto the front each child a born
// block references outside the batch, and each block a root moves onto, when
// its list is unverified. A put of a stored block whose list is unverified
// writes the union of the list and the put's refs, marked, since a list may
// lack a child and referencing one more only keeps a block longer, takes the
// block off the run and the front, and pushes its unverified children. A
// refilled hole does the same with the list it writes back. A list that gains a
// child logs no holder for it, so on an adopted Volume Land records that the
// counts owe a recount, which the flip reads. It refuses a hold onto a block
// the free parked with an AbsentError, and a put of one gives birth to it
// again, as Records.repark describes. While Sweeping, Land refuses a hold onto
// an unmarked block with an AbsentError, and a put of one gives birth to it
// again, as Records.rebirth describes: its old list's children outside the run
// lose the holder it gave them, through downs.
//
// A list is marked only from refs the writer read from content: an entry
// with RefsKnown. Another entry is copied: its refs came from a source's
// stored list, which may lack a child. Land keeps these invariants for one:
//
//  1. A copied entry never writes or marks a list. Of a stored block it
//     stores the bytes and holds the block as a put again; its list stays as
//     it was, and markStored skips it, so its refs never join the list.
//  2. While Open, a copied entry of a block without a list, or with a zero
//     count and none, is born with its refs as an unverified list, through
//     Records.putCopied, which enters it in the run and on the front in the
//     same transaction. It joins the front at its birth whatever holds it,
//     a root, a parent in the batch or a parent outside it, so the walk
//     decodes it and pushes any child the copied refs lack before the flip.
//  3. While Open, a copied entry of a parked block keeps its stored list,
//     through Records.keepParked, and the block gains no child. A park that
//     kept its holds leaves the holders the list gives as they were, so
//     repark's releases are not logged. A released park's list holds
//     nothing, so the put gives that list's ups as a born block's, and
//     refuses a hidden child as one does. Its count is then absent, the
//     implied holder; with no holder in the batch, the down appendLogs logs
//     nets it to zero, which queues it.
//  4. A copied entry of a recorded hole refuses the Land with
//     block.ErrRefsUnknown, since only content refills a hole. While
//     Sweeping, a copied entry of a block hidden or without a list refuses
//     it with ErrSweeping, since no walk is left to check a copied list.
//  5. While Settled, a copied entry of a block without a list moves the
//     generation back to Open in the same transaction, through
//     Records.reopen, and is then born as in invariant 2: from the commit
//     on every free parks until a walk decodes the block.
//
// A Volume that is not adopted keeps no logs: Land writes the child lists and
// roots and appends nothing, and an adoption counts every holder from them. An
// adopting or adopted Volume refuses a block that references a child whose
// bytes it holds without a child list, and a root onto such a block, because
// the children that block references would hold nothing. While an adoption
// counts the Volume, Land also marks each block it gives birth to and records
// the block each root it moves held when the adoption began, as Adopt reads.
//
// Cost per call: the state record and, when an entry has no list, the
// holes-clear mark, one point read; one batched read of the entries' child list
// records and one of the count records of those without a list and, on a
// Volume that keeps the logs, of the stored blocks it holds, and, for
// one without a list while that mark is absent, its hole record; one
// batched read of the child list records of the distinct children the born
// blocks name outside the batch and, when any is unlisted, one batched block
// existence read, with one up per naming; per moved root, its entry and its
// lease index record, and per distinct lease the roots name, its record.
// While the generation is not Settled, each child list record it reads
// returns its value, and per stored block it puts while
// Open, its list record, which it may rewrite, with one list read per child
// of a rewritten one, and per listed block it holds or puts, its count
// record and, for a zero one, its parked key; while Sweeping, one run entry
// read per unverified list it meets. On an adopted Volume, one queue delete
// per stored block it holds at zero, then one up page and one down page per
// logPageKeys keys, and the state record. While Open, a call that pushes a
// block onto the front reads and writes the add revision once. Nothing in it
// scans: every read is a point read or a batch.
// A move that requires its blocks held adds the reads requireTree describes.
func Land(
	ctx context.Context,
	rec *Records,
	blocks block.StoreOps,
	now time.Time,
	entries []*block.PutBatchEntry,
	puts PutHold,
	moves ...BucketRoots,
) ([]bool, error) {
	// Read the log positions and adoption.
	state, err := rec.GetState(ctx)
	if err != nil {
		return nil, err
	}
	l := &lander{
		rec:      rec,
		blocks:   blocks,
		state:    state,
		logs:     state.GetAdoption() != Adoption_ADOPTION_UNADOPTED,
		partial:  puts.Partial,
		adopting: state.GetAdoption() == Adoption_ADOPTION_ADOPTING,
		gen:      state.GetGeneration(),
		now:      now.UnixMilli(),
		born:     make(map[string]*bornBlock),
		touched:  make(map[string]*block.PutBatchEntry),
	}

	// Check every root before writing anything.
	held := make([][]*block.BlockRef, len(moves))
	for i, move := range moves {
		if held[i], err = l.checkRoots(ctx, move.BucketID, move.Roots); err != nil {
			return nil, err
		}
	}

	// Record the blocks the roots move onto, whose counts the puts' count
	// batch reads, then store the entries and find the blocks they give
	// birth to.
	l.holdRoots(moves)
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

	// Mark the unverified lists of the stored blocks the Land puts again.
	if err := l.markStoredLists(ctx); err != nil {
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

	// Refuse or count the born blocks nothing holds.
	if err := l.requirePutsHeld(puts); err != nil {
		return nil, err
	}

	// Dequeue the stored blocks it holds and append the logs, then write the
	// state if anything changed it.
	if err := l.appendLogs(ctx, now); err != nil {
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
	// counted reports that its count records holders from before the put.
	counted bool
	// reput reports that the store held its bytes without a child list, so
	// holders the store cannot see may reference it.
	reput bool
	// reborn reports that the block was unmarked, so the put gives birth to
	// it again.
	reborn bool
	// repark reports that the free parked the block, so the put gives birth
	// to it again.
	repark bool
	// copied reports that the put named children it copied rather than
	// read from content, so Land writes them as an unverified list the
	// settle checks, as Land describes.
	copied bool
	// keep reports a copied put of a parked block, which keeps its stored
	// list: children is nil, and the list gives its children no new holder.
	keep bool
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
	// logs reports that the Volume is adopting or adopted, so it keeps the
	// up and down logs.
	logs bool
	// partial lets a born block hold a hidden child, as PutHold.Partial
	// describes.
	partial bool
	// adopting reports that an adoption is counting the Volume, so Land marks
	// the blocks it gives birth to and the roots it moves, as Adopt reads.
	adopting bool
	// gen is the Volume's generation, which decides the settle's barrier.
	gen Generation
	// now is the Land's time in Unix milliseconds, against which a root's
	// lease must be live.
	now int64
	// leases caches whether each lease a root references is live at now.
	leases map[string]bool
	// touched maps the block keys of the entries already stored, each held
	// as a born put is, to the last entry that put it.
	touched map[string]*block.PutBatchEntry
	// stored are the first entries of the blocks stored with a child list,
	// whose lists markStored checks.
	stored []*block.PutBatchEntry
	// holds are the root targets and the zero counts classifyPuts reads for
	// the stored blocks the Land holds, which it dequeues.
	holds zeroHolds
	// refills are the entries that put a recorded hole, whose child lists
	// Land writes back.
	refills []*block.PutBatchEntry
	// born are the born blocks by block key.
	born map[string]*bornBlock
	// order are the born blocks in entry order.
	order []*bornBlock
	// up and down are the block keys gaining and losing a holder.
	up, down [][]byte
}

// stageHeld is BucketRoots.StageHeld.
type stageHeld = func(ctx context.Context, ref *block.BlockRef) (bool, error)

// checkRoots refuses a root named twice, or one whose Previous differs from
// the block it holds, and returns the block each root holds.
func (l *lander) checkRoots(ctx context.Context, bucketID string, roots []block.NamedRoot) ([]*block.BlockRef, error) {
	// A change that moves one root name twice is refused whole, before
	// anything lands. The name set keeps the check linear in the roots.
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

		// Refuse a root onto a lease that is not live.
		if root.Lease != "" && !root.Ref.GetEmpty() {
			if err := l.checkLease(ctx, root.Lease); err != nil {
				return nil, err
			}
		}
	}
	return held, nil
}

// checkLease returns ErrLeaseLapsed unless the lease id is live at the Land's
// time, reading its record once per Land.
func (l *lander) checkLease(ctx context.Context, id string) error {
	// Read the lease once.
	live, ok := l.leases[id]
	if !ok {
		var err error
		if live, err = l.rec.LeaseLive(ctx, id, l.now); err != nil {
			return err
		}
		if l.leases == nil {
			l.leases = make(map[string]bool)
		}
		l.leases[id] = live
	}

	// Refuse a lease that is not live.
	if !live {
		return errors.Wrap(ErrLeaseLapsed, id)
	}
	return nil
}

// ErrSweeping is returned by Land for a copied put of a block the Volume
// does not store while it sweeps: no walk is left to check the copied list
// before the flip, and a reopen then could undo a sweep in progress. It wraps
// block.ErrRefsUnknown; the writer retries once the generation moves, which
// a Volume does for a background writer and leaves to a foreground one.
var ErrSweeping = errors.Wrap(block.ErrRefsUnknown, "the volume is sweeping")

// putEntries stores the entries' bytes and the child lists of the blocks
// they give birth to, and evicts the tombstones' bytes.
func (l *lander) putEntries(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Build each entry's ref and sort its block: stored, a refilled hole,
	// or born.
	var holds *zeroHolds
	if l.logs {
		holds = &l.holds
	}
	err := classifyPuts(ctx, l.rec, l.blocks, entries, holds, func(entry *block.PutBatchEntry, kind putKind) error {
		key := string(BlockKey(entry.Ref))
		copied := !entry.RefsKnown
		switch {
		case kind == putRefill && copied:
			return errors.Wrapf(block.ErrRefsUnknown, "copied put of the hole %s", entry.Ref.MarshalString())
		case kind == putRefill:
			l.refills = append(l.refills, entry)
			l.touched[key] = entry
		case kind == putStored:
			if !copied {
				l.stored = append(l.stored, entry)
			}
			l.touched[key] = entry
		case copied && kind == putParked:
			b := &bornBlock{ref: entry.Ref, repark: true, keep: true}
			l.born[key] = b
			l.order = append(l.order, b)
		case copied && l.gen == Generation_GENERATION_SWEEPING:
			return errors.Wrapf(ErrSweeping, "copied put of %s", entry.Ref.MarshalString())
		case copied && l.gen == Generation_GENERATION_SETTLED:
			// Reopen the Volume, then store the block as an Open Volume does.
			// The reopen keeps these invariants, which Records.reopen
			// details:
			//  - No free deletes a block a holder reaches through a copied
			//    list: the generation is Open from this transaction's commit,
			//    so every free parks, and the Volume flips only after a walk
			//    decodes this block from the front.
			//  - A Settled Volume has no run, front, park or unmarked count,
			//    so the reopen writes only the generation and the run's
			//    completeness, and the block's birth through putCopied
			//    writes the rest exactly as it would on a Volume Open all
			//    along.
			//  - A child the copied list lacks, freed under Settled, is the
			//    one state an Open Volume never reaches: the walk finds it
			//    absent and keeps it on the front, and the flip waits on it
			//    as on any block a read may still fill.
			if err := l.rec.reopen(ctx, l.state); err != nil {
				return err
			}
			l.gen = Generation_GENERATION_OPEN
			l.dirty = true
			fallthrough
		default:
			b := &bornBlock{
				ref:      entry.Ref,
				children: normalizeChildren(entry.Refs),
				counted:  kind == putCounted,
				reborn:   kind == putReborn,
				repark:   kind == putRepark || kind == putParked,
				copied:   copied,
			}
			l.born[key] = b
			l.order = append(l.order, b)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Store the bytes, which also evicts the tombstones' bytes, then write the
	// child lists of the refilled holes back. A list gives its children
	// holders their counts lack, outside an adoption or where its count
	// already passed it, so it logs them as a birth does, and dequeues each
	// at zero: the free trusts a queue entry to prove no namer since the
	// block ripened, and the up nets only after it. It holds each as a born
	// block's child too, so a hidden one refuses the Land.
	existed, err := l.blocks.PutBlockBatch(ctx, entries)
	if err != nil {
		return nil, err
	}
	for _, entry := range l.refills {
		children, err := l.rec.refill(ctx, entry)
		if err != nil {
			return nil, err
		}
		if err := l.holdRefilled(ctx, children); err != nil {
			return nil, err
		}
		if l.logs && (!l.adopting || adoptCounted(l.state, BlockKey(entry.Ref))) {
			for _, child := range children {
				key := BlockKey(child)
				l.up = append(l.up, key)
				if err := l.dequeue(ctx, key); err != nil {
					return nil, err
				}
			}
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

		// A block parked or unmarked again releases the holders its old list
		// gave. While adopting, the count skips a born block, so the tally
		// holds those holders only if the count passed its list already.
		release := l.logs && (!l.adopting || adoptCounted(l.state, BlockKey(b.ref)))
		if b.repark {
			children, holds, err := l.rec.repark(ctx, l.state, BlockKey(b.ref))
			if err != nil {
				return nil, err
			}
			switch {
			case holds && release && !b.keep:
				l.down = append(l.down, children...)
			case !holds && b.keep:
				// A released park's list holds nothing, so the kept list
				// gives its ups as a born block's does, and holdChildren
				// refuses a hidden child.
				if b.children, _, err = l.rec.GetChildren(ctx, b.ref); err != nil {
					return nil, err
				}
			}
		}
		if b.reborn {
			released, err := l.rec.rebirth(ctx, b.ref)
			if err != nil {
				return nil, err
			}
			l.dirty = true
			if release {
				l.down = append(l.down, released...)
			}
		}
		switch {
		case b.keep:
			// The stored list stays. Kept holds stay with it, so the put
			// logs neither the downs repark released nor ups; released
			// ones come back through the ups of b.children.
			err = l.rec.keepParked(ctx, BlockKey(b.ref))
		case b.copied:
			err = l.rec.putCopied(ctx, b.ref, b.children)
		default:
			err = l.rec.PutChildren(ctx, b.ref, b.children)
		}
		if err != nil {
			return nil, err
		}
		// A block born from content while Open leaves the front, where a
		// walk that found it absent pushed it: the walk skips a marked
		// list, and the pop never proves a block its holders hold, so the
		// entry would hold the flip for good. holdOutside pushed its
		// unverified children.
		if !b.copied && !b.keep && l.gen == Generation_GENERATION_OPEN {
			if err := l.rec.tx.Delete(ctx, l.rec.key(kindFront, BlockKey(b.ref))); err != nil {
				return nil, err
			}
		}
		if l.adopting && !b.keep {
			if err := l.rec.tx.Set(ctx, l.rec.key(kindAdoptBorn, BlockKey(b.ref)), nil); err != nil {
				return nil, err
			}
		}
	}
	return existed, nil
}

// putKind is what the liveness records hold for a block a put references.
type putKind int

const (
	// putBorn is a block without a child list: the put gives birth to it.
	putBorn putKind = iota
	// putCounted is a block without a child list whose count outlived it
	// with holders, as when the store lost its bytes and list while a
	// parent still references it: the put gives birth to it, and the count
	// holds it.
	putCounted
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
	// putParked is a block with a child list the free parked while Open:
	// the put gives birth to it again, as Records.repark describes, and a
	// copied put keeps its list.
	putParked
)

// classifyPuts checks each put entry and fills an empty ref from its data, and
// calls fn, in entry order, with the first entry of each block the entries put
// and what the records hold for it, skipping nil entries and tombstones. It
// reads the blocks' child lists in one batch, whether the Volume records any
// hole once, at the first block without a child list, and a block's hole record
// only when one does, so a put into a Volume without a hole, the usual case,
// reads no hole record per block, then the counts of the blocks without a list
// in one batch. Until the generation is Settled it reads whether each listed
// block is hidden, as Records.Hidden describes.
//
// When holds is set, the same count batch reads the count of each block outside
// the batch a born block or a refilled hole references or a root moves onto,
// and of each block the batch stores again, and holds.zeros records each at
// zero, so Land dequeues them with no read of its own.
func classifyPuts(
	ctx context.Context,
	rec *Records,
	blocks block.StoreOps,
	entries []*block.PutBatchEntry,
	holds *zeroHolds,
	fn func(entry *block.PutBatchEntry, kind putKind) error,
) error {
	// Check each entry and build its ref, once per block.
	seen := make(map[string]struct{}, len(entries))
	puts := make([]*block.PutBatchEntry, 0, len(entries))
	keys := make([][]byte, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.Tombstone {
			continue
		}
		if len(entry.Data) == 0 {
			return block.ErrEmptyBlock
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
		puts = append(puts, entry)
		keys = append(keys, key)
	}

	// A Land that only moves roots still reads its roots' counts below, so
	// it dequeues a block at zero that a root now holds. The free proves no
	// late holder by that queue entry.
	if len(puts) == 0 && (holds == nil || len(holds.roots) == 0) {
		return nil
	}

	// Read the child lists in one batch.
	var listed []bool
	var err error
	if len(keys) != 0 {
		if listed, err = rec.listedBatch(ctx, keys); err != nil {
			return err
		}
	}

	// Sort each block by its list, then by whether it is a recorded hole or
	// unmarked, leaving the rest to their counts. A block with no list reads
	// its hole key only while the holes-clear mark is absent, so a put of
	// new blocks reads one key for the batch, and no seek under the writer.
	kinds := make([]putKind, len(puts))
	counted := make([][]byte, 0, len(keys))
	var countedAt []int
	var holesRead, holes bool
	for i, key := range keys {
		if listed[i] {
			if kinds[i], err = listedPut(ctx, rec, puts[i].Ref); err != nil {
				return err
			}
			continue
		}
		if !holesRead {
			if holes, err = rec.holesOpen(ctx); err != nil {
				return err
			}
			holesRead = true
		}
		hole := false
		if holes {
			if hole, err = rec.hasHole(ctx, key); err != nil {
				return err
			}
		}
		if hole {
			kinds[i] = putRefill
			continue
		}
		unmarked, err := rec.Unmarked(ctx, key)
		if err != nil {
			return err
		}
		if unmarked {
			kinds[i] = putReborn
			continue
		}
		counted = append(counted, rec.key(kindCount, key))
		countedAt = append(countedAt, i)
	}

	// Read the counts in one batch, with those of the blocks holds asks for.
	held := holds.gather(rec, puts, keys, kinds, seen)
	counted = append(counted, held...)
	if len(counted) != 0 {
		values, found, err := rec.countsBatch(ctx, counted)
		if err != nil {
			return err
		}
		for j, i := range countedAt {
			if kinds[i], err = unlistedPut(values[j], found[j]); err != nil {
				return err
			}
		}
		if err := holds.note(rec, held, values[len(countedAt):], found[len(countedAt):]); err != nil {
			return err
		}
	}
	for i, entry := range puts {
		if err := fn(entry, kinds[i]); err != nil {
			return err
		}
	}
	return nil
}

// zeroHolds gathers the count records a Land reads in classifyPuts' count
// batch, so it can dequeue each stored block it holds at zero with no read
// of its own. A nil zeroHolds, as RefillBlocks and a Volume without the logs
// pass, reads nothing more.
type zeroHolds struct {
	// roots are the block keys the moved roots move onto.
	roots [][]byte
	// zeros maps the block key of each gathered block at zero holders to the
	// ripe sequence its queue entry is keyed by.
	zeros map[string]uint64
}

// gather returns the count record keys the Land reads for holds: of each child
// outside the batch a born put or a refill references, each root target outside
// the batch, and each block the batch stores again, a refilled hole among them.
// kinds holds each put's kind, with the blocks whose counts are not read yet
// still putBorn, which is born like every kind their counts can give. A key
// named twice is read twice, which costs less than a map to find it. The keys
// share one allocation, built from each child's hash with no block key of its
// own.
func (h *zeroHolds) gather(rec *Records, puts []*block.PutBatchEntry, keys [][]byte, kinds []putKind, seen map[string]struct{}) [][]byte {
	// Without holds, nothing.
	if h == nil {
		return nil
	}

	// Size one buffer for every key.
	head := len(rec.prefix) + 1
	size, n := 0, len(h.roots)
	for _, key := range h.roots {
		size += head + len(key)
	}
	for i, entry := range puts {
		if kinds[i] == putStored || kinds[i] == putRefill {
			size, n = size+head+len(keys[i]), n+1
		}
		if kinds[i] == putStored {
			continue
		}
		for _, child := range entry.Refs {
			size, n = size+head+1+len(child.GetHash().GetHash()), n+1
		}
	}

	// Append each key's block key after the prefix and the kind, dropping
	// an outside block the batch stores.
	buf := make([]byte, 0, size)
	held := make([][]byte, 0, n)
	add := func(start int, outside bool) {
		if outside {
			if _, ok := seen[string(buf[start+head:])]; ok {
				buf = buf[:start]
				return
			}
		}
		held = append(held, buf[start:len(buf):len(buf)])
	}
	for i, entry := range puts {
		if kinds[i] == putStored || kinds[i] == putRefill {
			start := len(buf)
			buf = append(append(append(buf, rec.prefix...), kindCount), keys[i]...)
			add(start, false)
		}
		if kinds[i] == putStored {
			continue
		}
		for _, child := range entry.Refs {
			if child.GetEmpty() {
				continue
			}
			start, hash := len(buf), child.GetHash()
			buf = append(append(buf, rec.prefix...), kindCount, byte(hash.GetHashType())) //nolint:gosec // hash types fit in a byte.
			buf = append(buf, hash.GetHash()...)
			add(start, true)
		}
	}
	for _, key := range h.roots {
		start := len(buf)
		buf = append(append(append(buf, rec.prefix...), kindCount), key...)
		add(start, true)
	}
	return held
}

// note records, for each count key of held read with its value and whether
// it was found, the ripe sequence of each block it finds at zero holders.
func (h *zeroHolds) note(rec *Records, held, values [][]byte, found []bool) error {
	head := len(rec.prefix) + 1
	for i, key := range held {
		if !found[i] {
			continue
		}
		holders, ripe, err := decodeHolders(values[i])
		if err != nil {
			return err
		}
		if holders != 0 {
			continue
		}
		if h.zeros == nil {
			h.zeros = make(map[string]uint64)
		}
		h.zeros[string(key[head:])] = ripe
	}
	return nil
}

// unlistedPut returns what a put of a block with neither a child list, a hole
// record nor an empty run entry does, from its count record value. It keeps
// this invariant: a put leaves no record that lets the collector free or sweep
// the block it stores without a later release. Two records can outlive a lost
// list and do so. An empty run entry while Sweeping is one, since the sweep
// deletes every block it references whatever the block's list or count, so
// classifyPuts gives an unmarked block birth again, as Records.rebirth
// describes, whatever its count. A zero count is the other, left when the store
// lost a Dead block's list while its count, queue entry, front entry, parked
// key and run entry outlived it, so the put reparks such a block, as
// Records.repark describes. Neither takes an extra holder for bytes stored
// without a list, since the block's count recorded what holds it.
func unlistedPut(count []byte, found bool) (putKind, error) {
	// A block with no count is born, and one with holders is born held.
	if !found {
		return putBorn, nil
	}
	holders, _, err := decodeHolders(count)
	if err != nil {
		return putBorn, err
	}
	if holders != 0 {
		return putCounted, nil
	}
	return putRepark, nil
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
		return putParked, nil
	}
	return putReborn, err
}

// refill writes back the child list of the recorded hole entry puts, from
// its content, deletes its hole record and returns its children, whose
// counts lack the hole's holder, as kindHole describes. While the
// generation is Open, the block leaves the run and the front, and its
// unverified children join the front.
func (r *Records) refill(ctx context.Context, entry *block.PutBatchEntry) ([]*block.BlockRef, error) {
	// Write the list back and drop the hole record.
	key := BlockKey(entry.Ref)
	children := normalizeChildren(entry.Refs)
	if err := r.PutChildren(ctx, entry.Ref, children); err != nil {
		return nil, err
	}
	if err := r.tx.Delete(ctx, r.key(kindHole, key)); err != nil {
		return nil, err
	}

	// Keep the front invariant while the generation is Open.
	gen, err := r.generation(ctx)
	if err != nil || gen != Generation_GENERATION_OPEN {
		return children, err
	}
	return children, r.markedYoung(ctx, key, packChildren(nil, children), nil)
}

// adoptCounted reports whether the adoption that state records has counted the
// child list of the block at key: the count passes the lists in key order after
// the roots, and its cursor moves in the transaction that counts them, so a
// Land's transaction reads exactly which lists it holds.
func adoptCounted(state *LivenessState, key []byte) bool {
	cur := state.GetAdoptCursor()
	switch cur.GetPhase() {
	case AdoptPhase_ADOPT_PHASE_CHILDREN:
		return bytes.Compare(key, cur.GetAfter()) <= 0
	case AdoptPhase_ADOPT_PHASE_SETTLE:
		return true
	default:
		return false
	}
}

// holdRoots records the blocks moves will root, when the Volume keeps the
// logs. It must run before putEntries, since classifyPuts reads holds.roots
// in the puts' count batch to dequeue a stored block a root now holds at
// zero; recorded after it, the Land would leave on the free queue a block it
// is about to root.
func (l *lander) holdRoots(moves []BucketRoots) {
	if !l.logs {
		return
	}
	for _, move := range moves {
		for _, root := range move.Roots {
			if !root.Ref.GetEmpty() {
				l.holds.roots = append(l.holds.roots, BlockKey(root.Ref))
			}
		}
	}
}

// markStoredLists marks the unverified list of each stored block the Land
// puts again, in entry order, while the generation is Open, as markStored
// describes. It runs after putEntries, which fills stored.
func (l *lander) markStoredLists(ctx context.Context) error {
	if l.gen != Generation_GENERATION_OPEN {
		return nil
	}
	for _, entry := range l.stored {
		if err := l.markStored(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

// markStored marks the unverified list of the stored block entry puts
// again, while the generation is Open, as Land describes.
func (l *lander) markStored(ctx context.Context, entry *block.PutBatchEntry) error {
	// A marked list was written from content.
	key := BlockKey(entry.Ref)
	value, listed, err := l.rec.childList(ctx, key)
	if err != nil || !listed || listMarked(value) {
		return err
	}

	// Write the union, marked, and keep the front invariant. An adoption
	// compares the union with the old list after the write.
	var old []byte
	if l.adopting {
		old = bytes.Clone(listKeys(value))
	}
	packed, gained, _, err := l.rec.mergeList(ctx, key, value, entry.Refs, false)
	if err != nil {
		return err
	}

	// A gained child lacks the holder the list now gives it. While
	// adopting, the count reads the union where it has not passed the list,
	// and the put logs the holder where it has. Otherwise the counts owe a
	// recount.
	switch {
	case !gained || !l.logs:
	case !l.adopting:
		if err := l.rec.noteRewrite(ctx, l.state); err != nil {
			return err
		}
	case adoptCounted(l.state, key):
		l.up = appendGained(l.up, old, packed)
	}
	return l.rec.markedYoung(ctx, key, packed, nil)
}

// holdChildren counts the holder each born block gives each child: an
// implied holder of a child the batch stores too, and an up for any other.
// It reads the lists of the other children in one batch, and, on a Volume
// that keeps the logs, whether the store holds the bytes of those without a
// list in one more.
func (l *lander) holdChildren(ctx context.Context) error {
	// Give each child the batch stores its batch holder, and gather each
	// naming of the others: each gains its child a holder, one up, when the
	// Volume keeps the logs. A child two blocks reference is read twice in
	// the batch, which costs less than a map to find it. The keys share one
	// buffer: a born child's key is dropped from it.
	n, size := 0, 0
	for _, b := range l.order {
		n += len(b.children)
		for _, child := range b.children {
			size += blockKeyLen(child)
		}
	}
	outside := make([]*block.BlockRef, 0, n)
	keys := make([][]byte, 0, n)
	buf := make([]byte, 0, size)
	for _, b := range l.order {
		for _, child := range b.children {
			start := len(buf)
			buf = AppendBlockKey(buf, child)
			key := buf[start:len(buf):len(buf)]
			if c, ok := l.born[string(key)]; ok {
				c.holders++
				buf = buf[:start]
				continue
			}
			outside, keys = append(outside, child), append(keys, key)
		}
	}
	if l.logs {
		l.up = append(l.up, keys...)
	}
	if len(outside) == 0 {
		return nil
	}

	// Pass each the settle's check, dequeue the listed ones, and require the
	// unlisted ones absent.
	listed, err := l.holdOutside(ctx, outside, keys)
	if err != nil || !l.logs {
		return err
	}
	for i, ok := range listed {
		if ok {
			if err := l.dequeue(ctx, keys[i]); err != nil {
				return err
			}
		}
	}
	return l.requireListed(ctx, outside, listed)
}

// holdRefilled checks the holds a refilled hole gives children, its child list,
// as holdOutside checks a born block's: a child outside the batch that the
// generation hides refuses the Land with an AbsentError, so the writer puts it
// again. The hole's holder is one the child's count lacked, so the free may
// have parked the child while Open, and the flip unmarks a parked block before
// the up this Land logs can net. A child the flip left unmarked is the sweep's.
// A child the batch gives birth to is stored by this Land. A Settled Volume
// hides nothing and a partial Land counts a hidden child as an absent one, as
// PutHold.Partial describes, so holdRefilled reads nothing. Otherwise
// holdRefilled reads the children's lists in one batch.
func (l *lander) holdRefilled(ctx context.Context, children []*block.BlockRef) error {
	// Nothing is hidden while Settled, and a partial Land refuses no hidden
	// child.
	if l.partial || l.gen == Generation_GENERATION_SETTLED {
		return nil
	}

	// Check the children the batch does not give birth to.
	outside := make([]*block.BlockRef, 0, len(children))
	keys := make([][]byte, 0, len(children))
	for _, child := range children {
		key := BlockKey(child)
		if _, ok := l.born[string(key)]; !ok {
			outside, keys = append(outside, child), append(keys, key)
		}
	}
	if len(outside) == 0 {
		return nil
	}
	_, err := l.holdOutside(ctx, outside, keys)
	return err
}

// holdOutside checks the holds the born blocks give refs, children outside
// the batch whose block keys are keys, and reports whether the store lists
// each: one batched existence read while Settled, and otherwise one batched
// list read and holdStored's checks for each listed block.
func (l *lander) holdOutside(ctx context.Context, refs []*block.BlockRef, keys [][]byte) ([]bool, error) {
	// Read the lists in one batch.
	if l.gen == Generation_GENERATION_SETTLED {
		return l.rec.listedBatch(ctx, keys)
	}
	values, listed, err := l.rec.childListsBatch(ctx, keys)
	if err != nil {
		return nil, err
	}

	// Pass each listed block the settle's check.
	for i, ok := range listed {
		if !ok {
			continue
		}
		if err := l.holdListed(ctx, refs[i], keys[i], listMarked(values[i]), !l.partial); err != nil {
			return nil, err
		}
	}
	return listed, nil
}

// holdStored checks a hold onto ref, a block the batch does not store, and
// reports whether the store lists it, as holdListed checks it. While Settled
// it reads only whether the list exists.
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
	return true, l.holdListed(ctx, ref, key, marked, true)
}

// holdListed checks a hold onto ref, a listed block at key the batch does not
// store, while the generation is not Settled: when refuseHidden is set it
// refuses a hidden one with an AbsentError, as Records.Hidden describes, and
// while Open it pushes an unverified one onto the front.
func (l *lander) holdListed(ctx context.Context, ref *block.BlockRef, key []byte, marked, refuseHidden bool) error {
	// Refuse a hidden block. A marked block may die and park, and the flip
	// unmarks a parked one.
	if refuseHidden {
		hidden, err := l.rec.Hidden(ctx, ref)
		if err != nil {
			return err
		}
		if hidden {
			return &AbsentError{Ref: ref}
		}
	}

	// Push an unverified block while Open.
	if marked || l.gen != Generation_GENERATION_OPEN {
		return nil
	}
	return l.rec.addFront(ctx, key)
}

// requireListed refuses the first of refs listed reports without a child list
// whose bytes the store holds: the children it references would hold nothing. A
// block the store lacks, such as a child stored after its parent, passes. Its
// holders count from its birth. It reads the unlisted blocks' bytes in one
// batch, and nothing when every one is listed.
func (l *lander) requireListed(ctx context.Context, refs []*block.BlockRef, listed []bool) error {
	// Gather the unlisted blocks, and read their bytes in one batch.
	var unlisted []*block.BlockRef
	for i, ok := range listed {
		if !ok {
			unlisted = append(unlisted, refs[i])
		}
	}
	if len(unlisted) == 0 {
		return nil
	}
	found, err := l.blocks.GetBlockExistsBatch(ctx, unlisted)
	if err != nil {
		return err
	}
	for i, ok := range found {
		if ok {
			return errors.Wrap(block.ErrRefsUnknown, unlisted[i].MarshalString())
		}
	}
	return nil
}

// moveRoot moves one named root from held to root.Ref, counting the holder it
// adds and the one it removes.
func (l *lander) moveRoot(ctx context.Context, bucketID string, root block.NamedRoot, held *block.BlockRef) error {
	// A root that keeps its block changes no holder and only moves its
	// lease.
	entry := &RootEntry{Ref: BlockKey(root.Ref), Lease: root.Lease}
	if root.Ref.GetEmpty() {
		entry.Lease = ""
	}
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
	if !held.GetEmpty() && l.logs {
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
		if l.logs {
			key := BlockKey(root.Ref)
			l.up = append(l.up, key)
			return l.dequeue(ctx, key)
		}
		return nil
	}

	// A block stored without a child list may be referenced only while the
	// Volume keeps no logs. A block the store lacks may never be
	// referenced.
	found, err := l.blocks.GetBlockExists(ctx, root.Ref)
	if err != nil {
		return err
	}
	switch {
	case !found:
		return errors.Wrapf(block.NotFoundError(root.Ref), "named root %q", root.Name)
	case l.logs:
		return errors.Wrapf(block.ErrRefsUnknown, "named root %q on %s", root.Name, root.Ref.MarshalString())
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
	var counts [NotHeldPut + 1]uint64
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
		counts = [NotHeldPut + 1]uint64{}
	}
	if move.Observed != nil && (err != nil || counts != [NotHeldPut + 1]uint64{}) {
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
//     for a block with no count record, missing ones included. Held refuses a
//     missing block, one without a child list, and one at zero holders. A
//     stored block at zero still holds its children until it is freed, so a
//     boundary child under it reads Held; once the collector frees the
//     block, and then its children, none has a list and the move is
//     refused. A block with a list and no count record carries its
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
//     the walk reads nothing for a block the batch stores. It walks the
//     entries' refs as their children, which only RefsKnown proves, so a
//     block the batch stores only through a copied entry is not held: its
//     refs prove nothing of its children, and its stored list may be one
//     the settle has yet to check. The admit's batch holds only its
//     HeldRoots block, encoded from content with RefsKnown. A boundary
//     block may read at zero, since netting adds the holders a revival gives
//     it only once its record is Grace old; an open stage then proves it, as
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
//     errors or is cancelled counts nothing, and the Land proceeds as with the
//     check off. Only HeldEnforce returns the error.
//
// Cost: nothing for a block the batch stores; one child list and one count
// record read for each distinct boundary block, and staged's reads for one
// not held.
func (l *lander) requireTree(ctx context.Context, ref *block.BlockRef, staged stageHeld, notHeld func(NotHeldReason, *block.BlockRef) error) error {
	// A root the batch does not store must be held itself, and one it stores
	// through a copied entry is not.
	switch _, c := l.carried(ref); c {
	case carryNone:
		return l.requireHeld(ctx, ref, staged, NotHeldRoot, notHeld)
	case carryCopied:
		return notHeld(NotHeldRoot, ref)
	}

	// Walk the blocks the batch stores from content, requiring each boundary
	// child held.
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
			var err error
			switch _, c := l.carried(child); c {
			case carryKnown:
				walk = append(walk, child)
			case carryCopied:
				err = notHeld(NotHeldChild, child)
			default:
				err = l.requireHeld(ctx, child, staged, NotHeldChild, notHeld)
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// carry is how a batch stores a block, as the held check reads it.
type carry uint8

const (
	// carryNone is a block the batch does not store, or a copied put of a
	// parked block, which keeps its stored list.
	carryNone carry = iota
	// carryKnown is a block the batch stores, born or put again, through an
	// entry whose refs it read from content.
	carryKnown
	// carryCopied is a block the batch stores through a copied entry, whose
	// refs prove nothing of its children.
	carryCopied
)

// carried returns how the batch stores ref, and its children when it stores
// it from content.
func (l *lander) carried(ref *block.BlockRef) ([]*block.BlockRef, carry) {
	// A born block carries its list unless the put keeps the stored one or
	// copied it.
	key := BlockKey(ref)
	if b, ok := l.born[string(key)]; ok {
		switch {
		case b.keep:
			return nil, carryNone
		case b.copied:
			return nil, carryCopied
		}
		return b.children, carryKnown
	}

	// A block put again carries the refs of its last entry, if known.
	entry, ok := l.touched[string(key)]
	switch {
	case !ok:
		return nil, carryNone
	case !entry.RefsKnown:
		return nil, carryCopied
	}
	return entry.Refs, carryKnown
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

// appendLogs holds each born block beyond its implied holder and each block
// put again, dequeues the stored blocks the Land holds, and appends the ups
// and downs, on a Volume that keeps the logs.
func (l *lander) appendLogs(ctx context.Context, now time.Time) error {
	// Without the logs there is nothing to append.
	if !l.logs {
		return nil
	}

	// Count each born block's holders beyond its implied one. A block whose
	// bytes were stored without a child list may have holders the store
	// cannot see, so it keeps one more and never frees on a guess. A block
	// the batch gives no holder takes its implied one back with a down, so
	// an unheld put is garbage once the down ages past the grace period,
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

	// Hold each block put again as a born put is held: an up now, which
	// dequeues it, and a down for the same holder one grace period on.
	for _, key := range slices.Sorted(maps.Keys(l.touched)) {
		l.up = append(l.up, []byte(key))
		l.down = append(l.down, []byte(key))
		if err := l.dequeue(ctx, []byte(key)); err != nil {
			return err
		}
	}

	// Append the logs.
	if len(l.up) == 0 && len(l.down) == 0 {
		return nil
	}
	l.dirty = true
	return l.rec.appendLogs(ctx, l.state, now, l.up, l.down)
}

// dequeue deletes the queue entry of key, a stored block the Land holds,
// when classifyPuts read its count at zero. Each such block also has an up
// in this Land, which lifts its count once it nets, so the block leaves the
// queue for the window between the hold and its net, when its count still
// reads zero. A block at zero keeps its queue entry at the ripe sequence its
// zero record holds, so the count read finds the entry without a queue scan.
// Nothing the Land writes before it changes such a block's count, so the
// read stays current.
//
// Cost: one delete per block at zero, and no read.
func (l *lander) dequeue(ctx context.Context, key []byte) error {
	// A block read at zero leaves the queue once.
	ripe, ok := l.holds.zeros[string(key)]
	if !ok {
		return nil
	}
	delete(l.holds.zeros, string(key))
	return l.rec.tx.Delete(ctx, l.rec.queueKey(ripe, key))
}

// SameRef reports whether a and b reference the same block, or are both empty.
func SameRef(a, b *block.BlockRef) bool {
	if a.GetEmpty() || b.GetEmpty() {
		return a.GetEmpty() == b.GetEmpty()
	}
	return a.EqualsRef(b)
}
