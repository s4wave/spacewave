package viewcache

import (
	"context"
	"errors"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// previousValue is a value a catch-up dropped, with the newest revision it is
// known to hold at: the revision the cache stood at when that catch-up
// started, not the one whose change dropped it. It holds at every revision
// from entry.at through through.
type previousValue struct {
	entry   *revisionEntry
	through uint64
}

// cacheAdvance is the changes one catch-up passed.
type cacheAdvance struct {
	// from and through bound the revisions the changes cover: after from,
	// through through.
	from, through uint64
	// changes merges the changes.
	changes *world.ChangeSet
}

// cacheHistoryLimit bounds the catch-ups the cache keeps changes for. A value
// that took longer to compute than they cover is not kept for later readers.
const cacheHistoryLimit = 512

// Advance brings the cache to seqno or later, dropping each value a change
// since the cached revision may have altered.
func (c *Cache) Advance(ctx context.Context, seqno uint64) error {
	for {
		// Wait for a running catch-up, or start one.
		c.mtx.Lock()
		if seqno <= c.seqno.Load() {
			c.mtx.Unlock()
			return nil
		}
		if len(c.entries) == 0 {
			// The jump reads no changes, so no older value can be checked
			// across it.
			c.seqno.Store(seqno)
			c.history = nil
			c.movedLocked()
			c.mtx.Unlock()
			return nil
		}
		if wait := c.advancing; wait != nil {
			c.mtx.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		advancing := make(chan struct{})
		c.advancing = advancing
		from := c.seqno.Load()
		c.mtx.Unlock()

		// Read every change from the cached revision through seqno at once.
		through, changes, err := c.changesThrough(ctx, from, seqno)
		var next map[*revisionEntry]*revisionEntry
		if err == nil {
			next = c.refreshFacets(ctx, changes, through)
		}

		// Keep the values the changes left alone; one still computing keeps
		// the changes to check once it ends.
		c.mtx.Lock()
		c.advancing = nil
		close(advancing)
		if err == nil {
			c.dropTouchedLocked(changes, next, from)
			if through > c.seqno.Load() {
				c.history = append(c.history, cacheAdvance{from: from, through: through, changes: changes})
				if len(c.history) > cacheHistoryLimit {
					c.history = slices.Delete(c.history, 0, len(c.history)-cacheHistoryLimit)
					c.prunePreviousLocked(c.history[0].from)
				}
			}
			if through > c.seqno.Load() {
				c.seqno.Store(through)
				c.movedLocked()
			}
		}
		c.mtx.Unlock()
		if err != nil {
			return err
		}
	}
}

// refreshFacets computes again each facet part the changes touched, which
// some kept value reads by facet, and returns the new values by the ones they
// replace. A part that fails to compute is left out, so its readers drop as
// for a plain part. It holds no lock while computing.
func (c *Cache) refreshFacets(ctx context.Context, changes *world.ChangeSet, through uint64) map[*revisionEntry]*revisionEntry {
	// Find the touched facet parts the cache still keeps.
	type refresh struct {
		part    *revisionEntry
		compute func(context.Context, world.WorldState) (any, error)
	}
	var wanted []refresh
	c.mtx.Lock()
	seen := make(map[*revisionEntry]bool)
	for entry := range c.faceted {
		for _, facet := range entry.facets {
			part := facet.part
			if seen[part] || c.entries[part.name] != part {
				continue
			}
			seen[part] = true
			if part.touched(changes, nil) {
				wanted = append(wanted, refresh{part, facet.compute})
			}
		}
	}
	c.mtx.Unlock()

	// Compute them again.
	var next map[*revisionEntry]*revisionEntry
	for _, w := range wanted {
		replaced, err := c.recompute(ctx, w.part.name, through, w.compute)
		if err != nil {
			continue
		}
		if next == nil {
			next = make(map[*revisionEntry]*revisionEntry)
		}
		next[w.part] = replaced
	}
	return next
}

// recompute computes the named value at the World's newest revision, labeled
// with the revision through, and returns it as a finished entry the cache does
// not hold yet.
func (c *Cache) recompute(ctx context.Context, name string, through uint64, compute func(context.Context, world.WorldState) (any, error)) (*revisionEntry, error) {
	// Open a fresh snapshot for the facet computation.
	tx, err := c.engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Compute and track this value apart from the retained entry.
	entry := &revisionEntry{name: name, done: make(chan struct{}), at: through, reads: world.NewReadSet(), blocks: block.NewRefHold()}
	child, finish := world.WithViewReads(block.WithRefHold(ctx, entry.blocks), entry.reads)
	child, parts := c.withParts(context.WithValue(child, computingKey{}, true))
	entry.value, entry.err = c.observe(name, "shared", func() (any, error) {
		return compute(child, entry.reads.Track(tx))
	})
	entry.wake = finish()
	if entry.err != nil {
		return nil, entry.err
	}

	// Finish the entry before exposing its value to dependent projections.
	entry.parts, entry.facets = parts.collected()
	entry.finished = true
	close(entry.done)
	return entry, nil
}

// dropTouchedLocked drops each finished value whose reads changes, the
// changes after revision from, touched, checking only the values the index
// names: one it leaves out read nothing the changes name. It puts in the cache
// the facet parts computed again, in place of the ones they replace, and points
// the values reading them at the new ones. The caller holds mtx.
func (c *Cache) dropTouchedLocked(changes *world.ChangeSet, next map[*revisionEntry]*revisionEntry, from uint64) {
	// Find the values the changes may touch, before the facet parts computed
	// again join the index: they hold at the new revision already.
	if c.index == nil {
		return
	}
	accepted, tested := c.index.Candidates(changes)

	// Put the facet parts computed again in place of the ones they replace.
	for old, replaced := range next {
		if c.entries[old.name] != old {
			delete(next, old)
			continue
		}
		c.keepLocked(replaced)
	}

	// Drop each value the changes touched. The index accepted a value
	// because a changed key names one of the read sets it checks, through a
	// map lookup per changed key, and a change to a key a set names always
	// touches it; so an accepted value is dropped without scanning the
	// changes again, unless it reaches a part read by facet, which may leave
	// it alone. A value the index names only by a quad field, or every value
	// for changes that cannot say what changed, is tested.
	facets := make(map[*revisionEntry]bool, len(c.faceted))
	for entry := range c.faceted {
		facets[entry] = true
	}
	for _, entry := range accepted {
		if c.entries[entry.name] != entry || (reaches(entry, facets) && !entry.touched(changes, next)) {
			continue
		}
		c.dropChangedLocked(entry, changes, from)
	}
	for _, entry := range tested {
		if c.entries[entry.name] != entry || !entry.touched(changes, next) {
			continue
		}
		c.dropChangedLocked(entry, changes, from)
	}

	// The values kept read the new parts from now on. faceted holds only
	// finished values: one still computing sets its facets without the mtx.
	repointed := make(map[*revisionEntry]bool)
	for entry := range c.faceted {
		for i := range entry.facets {
			if replaced := next[entry.facets[i].part]; replaced != nil {
				entry.facets[i].part = replaced
				repointed[entry] = true
			}
		}
	}

	// Index them, and every value reaching one through its parts, by the
	// new parts' reads.
	if len(repointed) == 0 {
		return
	}
	for _, entry := range c.entries {
		if entry.finished && reaches(entry, repointed) {
			c.indexLocked(entry)
		}
	}
}

// dropChangedLocked drops entry, which a change after revision from touched.
// One a reader asked for through recent stays as its name's previous value,
// held through from. The caller holds mtx.
func (c *Cache) dropChangedLocked(entry *revisionEntry, changes *world.ChangeSet, from uint64) {
	// Retain a previously observed result until its replacement is ready.
	c.removeLocked(entry.name)
	if entry.recent && entry.err == nil && !entry.stale {
		if c.previous == nil {
			c.previous = make(map[string]previousValue)
		}
		c.previous[entry.name] = previousValue{entry: entry, through: from}
	}
	cause := "changed"
	if changes.Unknown {
		cause = "unknown"
	}
	c.countDrop(entry.name, cause)
}

// prunePreviousLocked forgets the previous values held only through a
// revision before oldest, the oldest the history reaches, so a name no reader
// asks for again does not keep its value. The caller holds mtx.
func (c *Cache) prunePreviousLocked(oldest uint64) {
	for name, previous := range c.previous {
		if previous.through < oldest {
			delete(c.previous, name)
		}
	}
}

// reaches reports whether entry reaches a target through its parts,
// transitively. memo holds the targets as true and records each answer.
func reaches(entry *revisionEntry, memo map[*revisionEntry]bool) bool {
	// Answer from the memo.
	if found, ok := memo[entry]; ok {
		return found
	}

	// Follow the plain and facet parts.
	found := slices.ContainsFunc(entry.parts, func(part *revisionEntry) bool { return reaches(part, memo) }) ||
		slices.ContainsFunc(entry.facets, func(facet facetRead) bool { return reaches(facet.part, memo) })
	memo[entry] = found
	return found
}

// keepLocked keeps entry under its name in place of any other value. A
// finished one is indexed. The caller holds mtx.
func (c *Cache) keepLocked(entry *revisionEntry) {
	// Replace the value kept under the name.
	if c.entries == nil {
		c.entries = make(map[string]*revisionEntry)
	}
	if old := c.entries[entry.name]; old != nil {
		c.unindexLocked(old)
	}
	c.entries[entry.name] = entry

	// Index a finished value; get indexes its own once it finishes.
	if entry.finished {
		c.indexLocked(entry)
	}
}

// removeLocked removes the value kept under name. The caller holds mtx.
func (c *Cache) removeLocked(name string) {
	if entry := c.entries[name]; entry != nil {
		c.unindexLocked(entry)
		delete(c.entries, name)
	}
}

// indexLocked indexes entry, a finished value the cache keeps, by every read
// set checking it consults, replacing an earlier entry. The caller holds mtx.
func (c *Cache) indexLocked(entry *revisionEntry) {
	// Create the index with its first value.
	if c.index == nil {
		c.index = world.NewChangeIndex[*revisionEntry]()
		c.faceted = make(map[*revisionEntry]struct{})
	}

	// Index the value, and note one reading a part by facet. It replaces
	// any previous value of its name.
	c.index.Add(entry, world.NewReadSetsInterest(entry.checkedSets()...))
	if len(entry.facets) != 0 {
		c.faceted[entry] = struct{}{}
	}
	if _, ok := c.previous[entry.name]; ok {
		delete(c.previous, entry.name)
		c.movedLocked()
	}
}

// unindexLocked removes entry from the index. The caller holds mtx.
func (c *Cache) unindexLocked(entry *revisionEntry) {
	if c.index != nil {
		c.index.Remove(entry)
		delete(c.faceted, entry)
	}
}

// changeMergeLimit bounds the keys and quads merged across revisions before
// the cache treats the changes as unknown, as the World's own watch does.
const changeMergeLimit = 4096

// errChangesThrough ends a change watch once it reaches the wanted revision.
var errChangesThrough = errors.New("changes reached the revision")

// changesThrough returns the World's changes after revision from through at
// least revision to, and the revision they run through.
func (c *Cache) changesThrough(ctx context.Context, from, to uint64) (uint64, *world.ChangeSet, error) {
	// Merge each revision's changes until the watch passes to.
	var through uint64
	var changes *world.ChangeSet
	err := world.WatchChanges(ctx, c.engine, from, func(seqno uint64, next *world.ChangeSet) error {
		through, changes = seqno, world.MergeChangeSets(changes, next, changeMergeLimit)
		if seqno >= to {
			return errChangesThrough
		}
		return nil
	})
	if !errors.Is(err, errChangesThrough) {
		return 0, nil, err
	}
	return through, changes, nil
}

// Forget removes the named value, which no reader will ask for again.
func (c *Cache) Forget(name string) {
	c.mtx.Lock()
	c.removeLocked(name)
	delete(c.previous, name)
	c.mtx.Unlock()
}

// drop removes entry from the cache unless another value replaced it.
func (c *Cache) drop(name string, entry *revisionEntry) {
	c.mtx.Lock()
	if c.entries[name] == entry {
		c.removeLocked(name)
	}
	c.mtx.Unlock()
}
