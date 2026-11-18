package packsweep

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	"github.com/s4wave/spacewave/db/kvtx"
	"golang.org/x/sync/errgroup"
)

// Check reasons, beside packcopy's and ReasonCandidate.
const (
	// ReasonUncatalogued names a candidate pack the cycle's catalog lacks, or
	// a key in a candidate's index the catalog does not list for that pack:
	// the mark never saw it, so it cannot have judged it dead.
	ReasonUncatalogued = "uncatalogued"
	// ReasonKeyCount names a candidate whose index holds another count of
	// keys than the catalog lists for it.
	ReasonKeyCount = "key-count"
	// ReasonRefsDiffer names a sampled block whose stored refs differ from
	// the catalog's, which the walk follows.
	ReasonRefsDiffer = "refs-differ"
)

// checkSet is the set of blocks a check has reached, with its frontier, in
// the check's own scratch store.
var checkSet = walkSet{
	visited:  checkVisitedPrefix,
	frontier: checkFrontierPrefix,
}

// CheckOptions configures Check.
type CheckOptions struct {
	// Cycle is the judged cycle's scratch store: its catalog gives each
	// pack's key count and members and each key's refs, and a pass writes
	// the cycle's receipt into it.
	Cycle kvtx.Store
	// Store is an empty scratch store of the check's own, holding the
	// candidates' keys, the blocks reached and the frontier.
	Store kvtx.Store
	// Candidates are the packs a sweep would remove, Mutant among them when
	// set.
	Candidates []string
	// Mutant is a pack the caller added to the candidates to test the check,
	// one some root reaches, or empty. A run with a mutant writes no receipt.
	Mutant string
	// PackKeys calls fn with each key of a candidate's own index, read from
	// the bucket rather than the catalog.
	PackKeys func(ctx context.Context, id string, fn func(key []byte) error) error
	// Source serves the blocks with every candidate left out.
	Source packcopy.Source
	// Refs serves the refs of a block neither the catalog nor Source holds,
	// such as one waiting to upload, or is nil.
	Refs RefSource
	// Roots are the roots walked.
	Roots []block.NamedRoot
	// CycleRootsOnly is set when Roots are the cycle's own, with no root
	// read fresh when the check began; the receipt records it, and a hide
	// refuses such a receipt.
	CycleRootsOnly bool
	// Accepted are missing blocks, as b58 refs, accepted as lost: the walk
	// counts them and goes on.
	Accepted map[string]bool
	// Sample is the fraction of the other reached blocks read by value and
	// checked against the catalog's refs, from 0 for none to 1 for all.
	Sample float64
	// SampleSeed chooses which blocks the sample takes.
	SampleSeed uint64
	// Batch is the frontier keys per transaction; zero uses
	// DefaultVerifyBatch.
	Batch int
	// Reads is the reads at once; zero uses DefaultVerifyReads.
	Reads int
	// MaxBad is the bad blocks the result lists; zero uses
	// DefaultVerifyMaxBad. Every bad block is counted.
	MaxBad int
	// Progress observes the check after each batch, nil for none.
	Progress func(*CheckResult)
	// Now reads the clock the receipt's time comes from; nil uses time.Now.
	Now func() time.Time
}

// Check proves that removing the candidate packs loses no block the roots
// reach, reading each candidate's index and the values of only the few blocks
// that need it.
//
// It reads every candidate's keys from the pack's own index in the bucket,
// and fails a candidate whose keys differ from the catalog's: the mark judged
// the catalog's keys, so a key only the index holds was never judged. It then
// walks the roots in its own scratch store, taking each block's children from
// the catalog. A reached block a candidate holds must read, hash-checked, from
// the packs left after the candidates; one that does not is named
// ReasonCandidate. A block the catalog cannot walk under is read from Source
// or asked of Refs, and fails unless one serves its refs or it is an accepted
// missing block. A sample of the other blocks is read by value and its refs
// compared with the catalog's.
//
// Invariant: the check passes only when no candidate holds the sole readable
// copy of a block reached from the roots, as far as the catalog's child lists
// are true; the sample tests those lists. Unsure fails: a candidate whose
// index cannot be read, a hole not accepted, and a block whose refs are
// unknown each count bad. An error is a failed scratch store or a stop; a bad
// block or pack is a result.
//
// A pass with no mutant, over candidates that are the cycle's TrashSet when
// the check began, writes the cycle's Receipt for them, which a hide needs:
// see Trash. It fails when the cycle's judgment changed while the check ran.
func Check(ctx context.Context, opts *CheckOptions) (*CheckResult, error) {
	// Apply the defaults.
	c := &checker{opts: opts, res: &CheckResult{
		Candidates: len(opts.Candidates),
		BadReasons: make(map[string]uint64),
		BadPacks:   make(map[string]uint64),
		LivePacks:  make(map[string]uint64),
	}}
	c.batch, c.reads, c.maxBad = opts.Batch, opts.Reads, opts.MaxBad
	if c.batch <= 0 {
		c.batch = DefaultVerifyBatch
	}
	if c.reads <= 0 {
		c.reads = DefaultVerifyReads
	}
	if c.maxBad <= 0 {
		c.maxBad = DefaultVerifyMaxBad
	}

	// Read the judgment the check proves, then record every candidate's
	// keys and seed the roots.
	head, err := ReadHead(ctx, opts.Cycle)
	if err != nil {
		return nil, err
	}
	proves, err := c.provesTrashSet(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.loadKeys(ctx); err != nil {
		return nil, err
	}
	if err := c.seed(ctx); err != nil {
		return nil, err
	}

	// Walk a batch at a time until the frontier is empty.
	for {
		done, err := c.walkBatch(ctx)
		if err != nil {
			return c.res, err
		}
		if done {
			break
		}
		if opts.Progress != nil {
			opts.Progress(c.res)
		}
	}

	// Give the cycle its receipt when the check passed over the trash set.
	if c.res.Bad != 0 || !proves {
		return c.res, nil
	}
	return c.res, c.writeReceipt(ctx, head)
}

// provesTrashSet reports whether a pass would prove the cycle's TrashSet: no
// mutant, and the candidates are that set exactly.
func (c *checker) provesTrashSet(ctx context.Context) (bool, error) {
	// A mutant run proves the check, not the set.
	if c.opts.Mutant != "" {
		return false, nil
	}

	// Compare the candidates with the set, which TrashSet gives in id order.
	d := newLineDigest()
	err := TrashSet(ctx, c.opts.Cycle, func(j *Judgment) error {
		d.add(j.ID)
		return nil
	})
	if err != nil {
		return false, err
	}
	n, digest := d.sum()
	m, want := digestLines(c.opts.Candidates)
	return n == m && digest == want, nil
}

// writeReceipt writes the receipt of the check's pass over the judgment of
// head.
func (c *checker) writeReceipt(ctx context.Context, head *Head) error {
	// Take the time, the digests and the sample.
	now := time.Now
	if c.opts.Now != nil {
		now = c.opts.Now
	}
	rec := &Receipt{
		Cycle:          head.Cycle,
		JudgedAt:       head.JudgedAt,
		CheckedAt:      now(),
		Sample:         c.opts.Sample,
		CycleRootsOnly: c.opts.CycleRootsOnly,
	}
	rec.Packs, rec.PacksDigest = digestLines(c.opts.Candidates)
	rec.Roots, rec.RootsDigest = digestLines(rootRefs(c.opts.Roots))

	// Write it with a row for each pack proved.
	if err := writeReceipt(ctx, c.opts.Cycle, rec, c.opts.Candidates); err != nil {
		return err
	}
	c.res.Receipt = rec
	return nil
}

// checker is one check.
type checker struct {
	// opts are the options.
	opts *CheckOptions
	// batch, reads and maxBad are the defaulted sizes.
	batch, reads, maxBad int
	// res is the result so far.
	res *CheckResult
}

// candidateKeys are one candidate's keys, read from its index.
type candidateKeys struct {
	// id is the pack.
	id string
	// tails are its keys' tails.
	tails [][]byte
	// reason is why the keys could not be read, empty when they were.
	reason string
}

// loadKeys reads the candidates' indexes, c.reads at once, and records each
// key with the candidates holding it in one writer.
func (c *checker) loadKeys(ctx context.Context) error {
	// Read the indexes, handing each pack's keys to the writer.
	read := make(chan *candidateKeys, c.reads)
	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		defer close(read)
		return c.readIndexes(egCtx, read)
	})

	// Record each pack's keys as it arrives.
	eg.Go(func() error {
		for ck := range read {
			if err := c.recordKeys(egCtx, ck); err != nil {
				return err
			}
		}
		return nil
	})
	return eg.Wait()
}

// readIndexes reads each candidate's keys into read, c.reads at once. A pack
// whose index cannot be read is sent with the reason, as a bad pack.
func (c *checker) readIndexes(ctx context.Context, read chan<- *candidateKeys) error {
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(c.reads)
	for _, id := range c.opts.Candidates {
		eg.Go(func() error {
			// Read the keys, keeping a failure as the pack's reason.
			ck := &candidateKeys{id: id}
			err := c.opts.PackKeys(egCtx, id, func(key []byte) error {
				ck.tails = append(ck.tails, bytes.Clone(Tail(key)))
				return nil
			})
			if err != nil {
				if egCtx.Err() != nil {
					return context.Cause(egCtx)
				}
				ck.tails, ck.reason = nil, packcopy.ReasonError
			}

			// Hand them on.
			select {
			case read <- ck:
				return nil
			case <-egCtx.Done():
				return context.Cause(egCtx)
			}
		})
	}
	return eg.Wait()
}

// recordKeys checks one candidate's keys against the catalog and records
// each with the candidates holding it. It runs in the one writer, which alone
// changes the result while the keys load.
func (c *checker) recordKeys(ctx context.Context, ck *candidateKeys) error {
	// Count a pack whose index could not be read, or that the catalog lacks.
	if ck.reason != "" {
		c.badPack(ck.id, ck.reason)
		return nil
	}
	cat, found, err := c.catalogPack(ctx, ck.id)
	if err != nil {
		return err
	}
	if !found {
		c.badPack(ck.id, ReasonUncatalogued)
		return nil
	}

	// Fail a key the catalog does not list for the pack, and another count.
	if err := c.checkMembers(ctx, ck); err != nil {
		return err
	}
	if uint64(len(ck.tails)) != cat.blocks {
		c.badPack(ck.id, ReasonKeyCount)
	}
	c.res.CandidateKeys += uint64(len(ck.tails))

	// Record each key with the candidates holding it.
	tx, err := c.opts.Store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	for _, tail := range ck.tails {
		if err := addHolder(ctx, tx, tail, ck.id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// catalogPack reads the cycle's catalog row of a pack.
func (c *checker) catalogPack(ctx context.Context, id string) (catalogPack, bool, error) {
	// Read the row.
	tx, err := c.opts.Cycle.NewTransaction(ctx, false)
	if err != nil {
		return catalogPack{}, false, err
	}
	defer tx.Discard()
	value, found, err := tx.Get(ctx, []byte(catalogPackPrefix+id))
	if err != nil || !found {
		return catalogPack{}, false, err
	}

	// Decode it.
	cat, err := unmarshalCatalogPack(value)
	return cat, err == nil, err
}

// checkMembers counts the pack bad when its index holds a key the catalog
// does not list for it.
func (c *checker) checkMembers(ctx context.Context, ck *candidateKeys) error {
	// Open a read of the catalog.
	tx, err := c.opts.Cycle.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Look up each key's membership row.
	id := []byte(ck.id + "/")
	for _, tail := range ck.tails {
		found, err := tx.Exists(ctx, prefixed(catalogMemberPrefix, id, tail))
		if err != nil {
			return err
		}
		if !found {
			c.badPack(ck.id, ReasonUncatalogued)
			return nil
		}
	}
	return nil
}

// addHolder adds a candidate to the ones a key's row names, one id a line.
func addHolder(ctx context.Context, tx kvtx.TxOps, tail []byte, id string) error {
	// Read the ids already named, then append this one.
	key := prefixed(checkKeyPrefix, tail)
	prev, _, err := tx.Get(ctx, key)
	if err != nil {
		return err
	}
	value := append(slices.Clip(prev), id...)
	return tx.Set(ctx, key, append(value, '\n'))
}

// seed adds every root to the set and its frontier.
func (c *checker) seed(ctx context.Context) error {
	// Visit the roots in one transaction.
	tx, err := c.opts.Store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	for _, root := range c.opts.Roots {
		if root.Ref.GetEmpty() {
			continue
		}
		if _, err := visit(ctx, tx, checkSet, root.Ref); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// checkKey is one frontier key and what the check found of it.
type checkKey struct {
	// frontierKey holds the tail, the ref, and the catalog's refs or hole
	// reason.
	frontierKey
	// holders are the candidates holding the key, none for most.
	holders []string
	// sampled is set for a key the sample reads.
	sampled bool
	// read is set once the key was read from Source.
	read bool
	// stored is the copy read, when readReason is empty.
	stored *block.StoredBlock
	// readReason is why the read failed, empty when it served the key.
	readReason string
	// served are the refs Refs served, when servedOK.
	served []*block.BlockRef
	// servedOK is set when Refs served the key's refs.
	servedOK bool
}

// needsRead reports whether the key is read from Source: a candidate holds
// it, the catalog cannot walk under it, or the sample takes it.
func (k *checkKey) needsRead() bool {
	return len(k.holders) != 0 || k.reason != "" || k.sampled
}

// walkBatch takes up to c.batch frontier keys, reads the ones that need it,
// and commits their children in place of the keys. It reports done when the
// frontier was empty.
func (c *checker) walkBatch(ctx context.Context) (bool, error) {
	// Take the batch, and read and ask what it needs.
	keys, err := c.readFrontier(ctx)
	if err != nil || len(keys) == 0 {
		return len(keys) == 0, err
	}
	if err := c.readBlocks(ctx, keys); err != nil {
		return false, err
	}
	if err := c.askRefs(ctx, keys); err != nil {
		return false, err
	}

	// Replace each key with its children, counting what it showed.
	tx, err := c.opts.Store.NewTransaction(ctx, true)
	if err != nil {
		return false, err
	}
	defer tx.Discard()
	for _, k := range keys {
		if err := c.take(ctx, tx, k); err != nil {
			return false, err
		}
	}
	return false, tx.Commit(ctx)
}

// readFrontier reads up to c.batch frontier keys with their holders, their
// catalog refs and whether the sample takes them.
func (c *checker) readFrontier(ctx context.Context) ([]*checkKey, error) {
	// Open a read of the check's store and of the catalog.
	tx, err := c.opts.Store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	cycle, err := c.opts.Cycle.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer cycle.Discard()

	// Take the first keys.
	var keys []*checkKey
	errFull := errors.New("full")
	prefix := []byte(checkSet.frontier)
	err = tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		// Decode the row's ref and keep the key, stopping at a full batch.
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(value); err != nil {
			return errors.Wrap(err, "frontier row")
		}
		keys = append(keys, &checkKey{frontierKey: frontierKey{tail: bytes.Clone(key[len(prefix):]), ref: ref}})
		if len(keys) == c.batch {
			return errFull
		}
		return nil
	})
	if err != nil && err != errFull {
		return nil, err
	}

	// Read each key's holders and catalog refs, and choose the sample.
	for _, k := range keys {
		holders, _, err := tx.Get(ctx, prefixed(checkKeyPrefix, k.tail))
		if err != nil {
			return nil, err
		}
		k.holders = splitHolders(holders)
		if err := readCatalogRefs(ctx, cycle, &k.frontierKey); err != nil {
			return nil, err
		}
		k.sampled = k.reason == "" && len(k.holders) == 0 && sampled(k.tail, c.opts.SampleSeed, c.opts.Sample)
	}
	return keys, nil
}

// splitHolders returns the ids of a key's holder row.
func splitHolders(row []byte) []string {
	var out []string
	for line := range bytes.Lines(row) {
		out = append(out, string(bytes.TrimSuffix(line, []byte{'\n'})))
	}
	return out
}

// sampled reports whether the sample takes the key: its tail, already a hash,
// mixed with the seed, falls in the fraction p of the range.
func sampled(tail []byte, seed uint64, p float64) bool {
	// Take none or all at the ends.
	if p <= 0 {
		return false
	}
	if p >= 1 {
		return true
	}

	// Mix the tail's first word with the seed (splitmix64's finalizer).
	var word [8]byte
	copy(word[:], tail)
	x := binary.BigEndian.Uint64(word[:]) ^ seed
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	x ^= x >> 31
	return float64(x) < p*math.MaxUint64
}

// readBlocks reads the keys that need it from Source, c.reads at once,
// checking each against its hash.
func (c *checker) readBlocks(ctx context.Context, keys []*checkKey) error {
	// Read each key that needs it, keeping the first error.
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(c.reads)
	for _, k := range keys {
		if !k.needsRead() {
			continue
		}
		eg.Go(func() error {
			stored, reason, err := packcopy.ReadVerified(egCtx, c.opts.Source, k.ref)
			k.read, k.stored, k.readReason = true, stored, reason
			return err
		})
	}
	return eg.Wait()
}

// askRefs asks Refs for the refs of each key that neither the catalog nor
// the copy read can walk under.
func (c *checker) askRefs(ctx context.Context, keys []*checkKey) error {
	// Gather the keys still without refs.
	if c.opts.Refs == nil {
		return nil
	}
	var ask []*checkKey
	var refs []*block.BlockRef
	for _, k := range keys {
		if k.reason != "" && !k.readRefs() {
			ask, refs = append(ask, k), append(refs, k.ref)
		}
	}
	if len(ask) == 0 {
		return nil
	}

	// Ask for them in one call.
	results, err := c.opts.Refs.BlockRefs(ctx, refs)
	if err != nil {
		return err
	}
	for i, k := range ask {
		if i < len(results) && results[i].Reason == "" {
			k.served, k.servedOK = results[i].Refs, true
		}
	}
	return nil
}

// readRefs reports whether the copy read serves the key's refs.
func (k *checkKey) readRefs() bool {
	return k.read && k.readReason == "" && k.stored.RefsKnown
}

// take replaces a key on the frontier with its children, counting what it
// showed.
func (c *checker) take(ctx context.Context, tx kvtx.TxOps, k *checkKey) error {
	// Take it off the frontier and count it.
	if err := tx.Delete(ctx, prefixed(checkSet.frontier, k.tail)); err != nil {
		return err
	}
	c.res.Reached++

	// A key a candidate holds must read from the other packs.
	if len(k.holders) != 0 {
		c.res.InCandidates++
		if k.readReason == "" {
			c.res.Shared++
		} else {
			c.bad(k, ReasonCandidate)
		}
	}

	// Find its children, or count it bad.
	children, ok := c.children(k)
	if !ok {
		return nil
	}

	// Add the children.
	for _, child := range children {
		if child.GetEmpty() {
			continue
		}
		if _, err := visit(ctx, tx, checkSet, child); err != nil {
			return err
		}
	}
	return nil
}

// children returns a key's children: the catalog's, checked against any copy
// read, or else the copy's or the ones Refs served. It counts a key with none
// bad, unless it is an accepted missing block.
func (c *checker) children(k *checkKey) ([]*block.BlockRef, bool) {
	// Take the catalog's refs, checking against them a sampled copy and any
	// other copy read that knows its refs, since the walk follows the
	// catalog's.
	if k.reason == "" {
		if k.sampled {
			c.res.Sampled++
		}
		switch {
		case k.sampled && k.readReason != "":
			c.bad(k, k.readReason)
		case k.read && k.readReason == "" && (k.sampled || k.stored.RefsKnown) && !sameRefs(nonEmpty(k.stored.Refs), k.refs):
			c.bad(k, ReasonRefsDiffer)
		}
		return k.refs, true
	}

	// Take the refs of the copy read, or the ones Refs served.
	switch {
	case k.readRefs():
		return nonEmpty(k.stored.Refs), true
	case k.servedOK:
		return k.served, true
	}

	// Count an accepted missing block, and any other bad.
	if k.reason == packcopy.ReasonMissing && k.readReason == packcopy.ReasonMissing && c.opts.Accepted[k.ref.MarshalString()] {
		c.res.Accepted++
		return nil, false
	}
	reason := k.reason
	switch {
	case k.read && k.readReason == "":
		reason = packcopy.ReasonRefsUnknown
	case k.read:
		reason = packcopy.WorseReason(k.readReason, reason)
	}
	if len(k.holders) == 0 {
		c.bad(k, reason)
	}
	return nil, false
}

// sameRefs reports whether two child lists name the same refs in order.
func sameRefs(a, b []*block.BlockRef) bool {
	return slices.EqualFunc(a, b, func(x, y *block.BlockRef) bool {
		return x.EqualVT(y)
	})
}

// bad counts a bad block, against each candidate holding it, and lists it
// while the list has room.
func (c *checker) bad(k *checkKey, reason string) {
	c.res.Bad++
	c.res.BadReasons[reason]++
	if reason == ReasonCandidate {
		for _, id := range k.holders {
			c.res.BadPacks[id]++
			c.res.LivePacks[id]++
		}
	}
	if len(c.res.BadBlocks) < c.maxBad {
		c.res.BadBlocks = append(c.res.BadBlocks, CheckBad{Ref: k.ref, Packs: k.holders, Reason: reason})
	}
}

// badPack counts a candidate whose keys cannot be proven, and lists it while
// the list has room.
func (c *checker) badPack(id, reason string) {
	c.res.Bad++
	c.res.BadReasons[reason]++
	c.res.BadPacks[id]++
	if len(c.res.BadBlocks) < c.maxBad {
		c.res.BadBlocks = append(c.res.BadBlocks, CheckBad{Packs: []string{id}, Reason: reason})
	}
}
