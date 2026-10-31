package sobject_world_engine

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
)

// worldPagesMaxBytes and worldPagesMaxCount bound the length and number of the
// pages a worldPages keeps. A page offered past either is left for the caller
// to store. Both sit under the volume's publication bound, so the publication
// naming a World fits its kept pages.
const (
	worldPagesMaxBytes = 16 << 20
	worldPagesMaxCount = 16384
)

// worldPages is the SharedObject's block store with the World pages the engine
// computed and has not stored in front of it. Every World reader the engine
// builds reads through it, so a page is readable from the moment it is kept.
// It also tracks each page written through it above a kept page, such as a
// page past the bound or an early buffer drain stored, so a walk from a root
// finds every kept page below it.
//
// Invariants:
//   - A tracked page carries its complete child list, so a walk through
//     tracked pages never reads the store to find a child.
//   - A stored page whose graph reaches a kept page is tracked: a walk that
//     stops at an untracked page leaves nothing unwritten below it.
//   - A page leaves only once the store holds the graph below it durably
//     (stored) or no live root and no reader pin reaches it (sweep). A reader
//     that finds a page missing here finds it in the store, or holds nothing
//     that reaches it.
//   - A reader pin on a kept page becomes a pin in the store before the page
//     leaves for the store, so the collector never sees the page unpinned.
//   - No completion proof names a tracked root: its graph is not stored.
type worldPages struct {
	// pageView serves kept pages and forwards the rest to the store.
	pageView
	// inner is the SharedObject's block store.
	inner bstore.BlockStore
	// stamps stamps each reader pin with the adopted deadlist record.
	stamps readerStamps
}

// newWorldPages wraps the SharedObject's block store with an empty page set.
func newWorldPages(inner bstore.BlockStore) *worldPages {
	set := &pageSet{pages: make(map[string]*trackedPage)}
	return &worldPages{pageView: pageView{StoreOps: inner, set: set}, inner: inner}
}

// newWorldPages returns the page view of so's block store, or nil when the
// engine stores every page as it computes it: when the Config asks it to, for
// an object whose publications copy the World into another store, which reads
// the pages from so's block store, and for an object whose state cannot join a
// block publication, since a World is written only in the publication that
// names it.
func (c *Controller) newWorldPages(so sobject.SharedObject) *worldPages {
	// Keep nothing where the Config, the publications or the store rule it
	// out.
	if c.conf.GetStoreEveryWorld() {
		return nil
	}
	if _, ok := so.(sobject.PublicationRetention); ok {
		return nil
	}
	if _, ok := so.(sobject.StatePublisher); !ok {
		return nil
	}
	store := so.GetBlockStore()
	if p, ok := store.(block.AtomicPublisher); !ok || !p.SupportsAtomicPublication() {
		return nil
	}
	return newWorldPages(store)
}

// keep adds the World pages among entries and returns the entries it did not
// keep, which the caller stores: tombstones, blocks without a complete child
// list, and pages past the length bound, which it tracks.
func (w *worldPages) keep(entries []*block.PutBatchEntry) []*block.PutBatchEntry {
	return w.set.keep(entries)
}

// keeping returns a view of the store whose writes keep the World pages they
// write, as a commit keeps its pages, and store the rest. Replay writes through
// it, so the Worlds it computes reach the store only with the replay base or
// checkpoint that names them. The view buffers every write until the writer
// syncs: a store that buffers itself would take each page as it is written.
func (w *worldPages) keeping() bstore.BlockStore {
	return &keepingPages{worldPages: w}
}

// keepingPages is the view keeping returns.
type keepingPages struct {
	*worldPages
}

// GetSupportedFeatures reports the store's features, minus its own buffering.
func (k *keepingPages) GetSupportedFeatures() block.StoreFeature {
	return k.worldPages.GetSupportedFeatures() &^ block.StoreFeatureSelfBuffered
}

// PutBlockBatch keeps the World pages among entries and stores the rest. A
// kept page reports that it was not found stored.
func (k *keepingPages) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Keep the pages, then store the rest.
	rest := k.keep(entries)
	existed := make([]bool, len(entries))
	if len(rest) == 0 {
		return existed, nil
	}
	found, err := k.worldPages.PutBlockBatch(ctx, rest)
	if err != nil {
		return nil, err
	}

	// Report each stored entry's result at its place in entries.
	j := 0
	for i, e := range entries {
		if j < len(rest) && rest[j] == e {
			existed[i] = j < len(found) && found[j]
			j++
		}
	}
	return existed, nil
}

// pagesOf returns the kept pages the roots reach, which a publication naming
// the roots must carry. It reads nothing from the store.
func (w *worldPages) pagesOf(roots ...*block.BlockRef) []*block.PutBatchEntry {
	// A nil view keeps nothing.
	if w == nil {
		return nil
	}
	w.set.mu.RLock()
	defer w.set.mu.RUnlock()
	var entries []*block.PutBatchEntry
	for key := range w.set.reach(roots) {
		if e := w.set.pages[key].entry; e != nil {
			entries = append(entries, e)
		}
	}
	return entries
}

// split separates the kept pages among entries, which a publication naming
// their World carries, from the rest.
func (w *worldPages) split(entries []*block.PutBatchEntry) (pages, rest []*block.PutBatchEntry) {
	// A nil view keeps nothing.
	if w == nil {
		return nil, entries
	}
	for _, e := range entries {
		if w.set.get(e.Ref) == e {
			pages = append(pages, e)
		} else {
			rest = append(rest, e)
		}
	}
	return pages, rest
}

// write runs fn on a buffer over the view and keeps the World pages it
// writes, as a commit keeps its pages: they are stored with the first World
// a checkpoint or stall base names that reaches them. A page past the bound,
// or a block without its child list, is stored through the view.
func (w *worldPages) write(ctx context.Context, fn func(block.StoreOps) error) error {
	// Buffer fn's writes.
	buf := block.NewBufferedStore(ctx, w)
	if err := fn(buf); err != nil {
		return err
	}
	batch, err := buf.TakePending(ctx)
	if err != nil {
		return err
	}

	// Keep the pages and store the rest.
	if rest := w.keep(batch.Entries); len(rest) != 0 {
		_, err = w.PutBlockBatch(ctx, rest)
	}
	batch.Complete(err)
	return err
}

// frontier returns the roots of the stored graph below roots: each root not
// tracked, and each untracked child of a tracked page the roots reach. A copy
// of the frontier holds every block of the roots' graphs the view does not.
func (w *worldPages) frontier(roots []*block.BlockRef) []*block.BlockRef {
	// Without a view, the store holds the whole graph.
	if w == nil {
		return roots
	}

	// Collect each untracked ref once.
	w.set.mu.RLock()
	defer w.set.mu.RUnlock()
	seen := make(map[string]struct{})
	var out []*block.BlockRef
	add := func(refs []*block.BlockRef) {
		for _, ref := range refs {
			if ref.GetEmpty() {
				continue
			}
			key := ref.MarshalString()
			if _, ok := seen[key]; ok || w.set.pages[key] != nil {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, ref)
		}
	}
	add(roots)
	for key := range w.set.reach(roots) {
		add(w.set.pages[key].refs)
	}
	return out
}

// full reports whether the kept pages reached half of either bound, when the
// engine names the head World so the next pages fit.
func (w *worldPages) full() bool {
	w.set.mu.RLock()
	defer w.set.mu.RUnlock()
	return w.set.bytes >= worldPagesMaxBytes/2 || w.set.count >= worldPagesMaxCount/2
}

// stored drops the kept pages the roots reach once a durable write put them in
// the store, moving each reader pin on them to the store first. On failure
// the pages stay kept.
func (w *worldPages) stored(ctx context.Context, roots []*block.BlockRef) error {
	// Take the pages and the pins on them.
	w.set.mu.Lock()
	taken := make(map[string]*trackedPage)
	var pins []*pagePin
	var pinned []*block.BlockRef
	for key := range w.set.reach(roots) {
		page := w.set.pages[key]
		taken[key] = page
		for _, pin := range page.pins {
			pins, pinned = append(pins, pin), append(pinned, page.entry.Ref)
		}
		w.set.remove(key)
	}
	w.set.mu.Unlock()

	// Pin each pinned page in the store, which now holds it. A failure puts
	// the pages back with the pins not yet moved, so no pin is ever left
	// uncovered.
	for i, pin := range pins {
		release, err := block.PinRoot(ctx, w.inner, pinned[i])
		if err != nil {
			w.set.restore(taken)
			return err
		}
		w.set.mu.Lock()
		released := pin.released
		if !released {
			pin.release = release
		}
		w.set.mu.Unlock()
		if released {
			release()
		}
	}
	return nil
}

// sweep drops the kept pages that neither a live root nor a reader pin reaches.
func (w *worldPages) sweep(live []*block.BlockRef) {
	// Mark from the live roots and the pinned pages, then drop the rest.
	w.set.mu.Lock()
	defer w.set.mu.Unlock()
	roots := slices.Clone(live)
	for _, page := range w.set.pages {
		if len(page.pins) != 0 {
			roots = append(roots, page.entry.Ref)
		}
	}
	reached := w.set.reach(roots)
	for key := range w.set.pages {
		if _, ok := reached[key]; !ok {
			w.set.remove(key)
		}
	}
}

// keptBytes returns the length of the kept pages.
func (w *worldPages) keptBytes() int {
	w.set.mu.RLock()
	defer w.set.mu.RUnlock()
	return w.set.bytes
}

// GetID returns the store's ID.
func (w *worldPages) GetID() string {
	return w.inner.GetID()
}

// GetDecodedBlockCache returns the store's decoded block cache.
func (w *worldPages) GetDecodedBlockCache() *block.DecodedBlockCache {
	return w.inner.GetDecodedBlockCache()
}

// ReclaimStorage forwards the backend reclaim pass. Kept pages are not in the
// backend, so the pass cannot drop them.
func (w *worldPages) ReclaimStorage(ctx context.Context, fence bstore.ReclaimFence) (time.Time, error) {
	return w.inner.ReclaimStorage(ctx, fence)
}

// AtomicPublicationVolumeID returns the store's publication namespace.
func (w *worldPages) AtomicPublicationVolumeID() string {
	if !w.SupportsAtomicPublication() {
		return ""
	}
	return w.inner.(block.AtomicPublisher).AtomicPublicationVolumeID()
}

// SupportsAtomicPublication reports whether the store publishes atomically.
func (w *worldPages) SupportsAtomicPublication() bool {
	p, ok := w.inner.(block.AtomicPublisher)
	return ok && p.SupportsAtomicPublication()
}

// SubmitAtomic forwards a publication to the store.
func (w *worldPages) SubmitAtomic(ctx context.Context, p *block.AtomicPublication) (*block.PublicationReceipt, error) {
	if !w.SupportsAtomicPublication() {
		return nil, block.ErrAtomicPublicationUnsupported
	}
	return w.inner.(block.AtomicPublisher).SubmitAtomic(ctx, p)
}

// PublishAtomic forwards a publication to the store and waits for it.
func (w *worldPages) PublishAtomic(ctx context.Context, p *block.AtomicPublication) error {
	if !w.SupportsAtomicPublication() {
		return block.ErrAtomicPublicationUnsupported
	}
	return w.inner.(block.AtomicPublisher).PublishAtomic(ctx, p)
}

// SupportsRootRetention reports the store's root ownership.
func (w *worldPages) SupportsRootRetention() bool {
	return block.SupportsRootRetention(w.inner)
}

// KeepsHoldCounts reports whether the store keeps shared counts.
func (w *worldPages) KeepsHoldCounts() bool {
	return block.KeepsHoldCounts(w.inner)
}

// SetRetainedRoots forwards named roots to the store.
func (w *worldPages) SetRetainedRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return block.SetRetainedRoots(ctx, w.inner, entries, roots)
}

// PinRoot protects ref until release. A kept page is not in the store, so no
// sweep there can collect it: the pin waits here and moves to the store when
// stored drops the page.
func (w *worldPages) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	// Pin a kept page here.
	key := ref.MarshalString()
	pin := &pagePin{}
	w.set.mu.Lock()
	page := w.set.pages[key]
	if page != nil && page.entry != nil {
		page.pins = append(page.pins, pin)
	} else {
		page = nil
	}
	w.set.mu.Unlock()
	if page != nil {
		return w.stamps.stamp(func() { w.set.unpin(key, pin) }), nil
	}

	// Pin a root the store holds.
	release, err := block.PinRoot(ctx, w.inner, ref)
	if err != nil {
		return nil, err
	}
	return w.stamps.stamp(release), nil
}

// OpenStage opens a stage on the store. Reads see the kept pages; writes go
// to the stage and are tracked like writes through the view.
func (w *worldPages) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	stage, release, err := block.OpenStage(ctx, w.inner)
	if err != nil {
		return nil, nil, err
	}
	return block.NewStoreRW(&pageView{StoreOps: w.inner, set: w.set}, &pageView{StoreOps: stage, set: w.set}), release, nil
}

// ReleaseRoots drops the staging ownership of roots in the store.
func (w *worldPages) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	return block.ReleaseRoots(ctx, w.inner, refs)
}

// MarkRootsComplete records the completion proofs of the roots the store
// holds. A tracked root's graph is not stored, so it gets no proof.
func (w *worldPages) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return block.MarkRootsComplete(ctx, w.inner, w.set.unkept(roots))
}

// RootsComplete reports the completion proof of each root. A tracked root
// has none.
func (w *worldPages) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Ask the store, then clear the proof of each tracked root.
	complete, err := block.RootsComplete(ctx, w.inner, refs)
	if err != nil {
		return nil, err
	}
	w.set.mu.RLock()
	defer w.set.mu.RUnlock()
	for i, ref := range refs {
		if w.set.pages[ref.MarshalString()] != nil {
			complete[i] = false
		}
	}
	return complete, nil
}

// BeginDeferFlush forwards the GC defer-flush scope to the store.
func (w *worldPages) BeginDeferFlush() {
	block.BeginDeferFlush(w.inner)
}

// EndDeferFlush closes the GC defer-flush scope on the store.
func (w *worldPages) EndDeferFlush(ctx context.Context) error {
	return block.EndDeferFlush(ctx, w.inner)
}

// pageView serves the kept pages of a pageSet in front of a store.
type pageView struct {
	// StoreOps is the store behind the kept pages.
	block.StoreOps
	// set holds the kept pages.
	set *pageSet
}

// BeginReadOperation opens a read scope on the store that also sees the kept
// pages.
func (v *pageView) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	ops, release, err := v.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &pageView{StoreOps: ops, set: v.set}, release, nil
}

// EnsureDecodedBlockCacheFresh forwards decoded-cache freshness to the store.
func (v *pageView) EnsureDecodedBlockCacheFresh(ctx context.Context) error {
	freshener, ok := v.StoreOps.(block.DecodedBlockCacheFreshener)
	if !ok {
		return nil
	}
	return freshener.EnsureDecodedBlockCacheFresh(ctx)
}

// GetBlock returns a kept page or reads the store.
func (v *pageView) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if e := v.set.get(ref); e != nil {
		return e.Data, true, nil
	}
	return v.StoreOps.GetBlock(ctx, ref)
}

// GetStoredBlock returns a kept page with its child list or reads the store.
func (v *pageView) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if e := v.set.get(ref); e != nil {
		return &block.StoredBlock{Data: e.Data, Refs: e.Refs, RefsKnown: true}, nil
	}
	return v.StoreOps.GetStoredBlock(ctx, ref)
}

// GetBlockExists reports a kept page or asks the store.
func (v *pageView) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	if v.set.get(ref) != nil {
		return true, nil
	}
	return v.StoreOps.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch reports the kept pages and asks the store for the rest.
func (v *pageView) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Answer the kept pages.
	out := make([]bool, len(refs))
	var rest []*block.BlockRef
	var restIdx []int
	for i, ref := range refs {
		if v.set.get(ref) != nil {
			out[i] = true
			continue
		}
		rest = append(rest, ref)
		restIdx = append(restIdx, i)
	}
	if len(rest) == 0 {
		return out, nil
	}

	// Ask the store for the rest.
	found, err := v.StoreOps.GetBlockExistsBatch(ctx, rest)
	if err != nil {
		return nil, err
	}
	for i, ok := range found {
		out[restIdx[i]] = ok
	}
	return out, nil
}

// PutBlock writes a block to the store and tracks it when its graph reaches a
// kept page.
func (v *pageView) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := v.StoreOps.PutBlock(ctx, data, opts)
	if err == nil && opts.GetRefsKnown() {
		v.set.track(ref, opts.GetRefs())
	}
	return ref, existed, err
}

// PutBlockBatch writes blocks to the store and tracks each whose graph reaches
// a kept page.
func (v *pageView) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	existed, err := v.StoreOps.PutBlockBatch(ctx, entries)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.Tombstone && e.RefsKnown {
			v.set.track(e.Ref, e.Refs)
		}
	}
	return existed, nil
}

// StatBlock reports a kept page or asks the store.
func (v *pageView) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	if e := v.set.get(ref); e != nil {
		return &block.BlockStat{Ref: ref.Clone(), Size: int64(len(e.Data))}, nil
	}
	return v.StoreOps.StatBlock(ctx, ref)
}

// pageSet holds the tracked World pages by reference: the kept pages, and the
// stored pages whose graph reaches one.
type pageSet struct {
	// mu guards the fields below.
	mu sync.RWMutex
	// pages are the tracked pages by marshaled reference.
	pages map[string]*trackedPage
	// bytes and count are the length and number of the kept pages.
	bytes, count int
}

// trackedPage is a tracked World page: a kept page with its data and the
// reader pins on it, or a stored page with its child list.
type trackedPage struct {
	// entry is the kept page with its complete child list, or nil for a
	// stored page.
	entry *block.PutBatchEntry
	// refs is the complete child list.
	refs []*block.BlockRef
	// pins are the reader pins taken while the page was kept.
	pins []*pagePin
}

// pagePin is a reader pin taken on a kept page.
type pagePin struct {
	// release drops the store pin, or is nil while the page is kept.
	release func()
	// released records that the holder released the pin before it moved to
	// the store.
	released bool
}

// keep adds the World pages among entries and returns the rest. A kept page
// is skipped. A page tracked as stored is returned, so its put reaches the
// store and finds it, which counts the new parent's edge. A page past the
// bound is returned and tracked, since the caller stores it.
func (s *pageSet) keep(entries []*block.PutBatchEntry) []*block.PutBatchEntry {
	// Add each page within the bound once.
	s.mu.Lock()
	defer s.mu.Unlock()
	var rest []*block.PutBatchEntry
	for _, e := range entries {
		if e.Tombstone || !e.RefsKnown {
			rest = append(rest, e)
			continue
		}
		key := e.Ref.MarshalString()
		if page := s.pages[key]; page != nil {
			if page.entry == nil {
				rest = append(rest, e)
			}
			continue
		}
		if s.bytes+len(e.Data) > worldPagesMaxBytes || s.count >= worldPagesMaxCount {
			rest = append(rest, e)
			s.trackLocked(key, e.Refs)
			continue
		}
		s.pages[key] = &trackedPage{entry: e, refs: e.Refs}
		s.bytes += len(e.Data)
		s.count++
	}
	return rest
}

// track records the child list of a page stored under ref when a child is
// tracked. A stored page whose children are all untracked has its graph
// stored, so it needs no record.
func (s *pageSet) track(ref *block.BlockRef, refs []*block.BlockRef) {
	// Check the children under the read lock first, since most writes reach
	// no tracked page.
	s.mu.RLock()
	reaches := slices.ContainsFunc(refs, func(r *block.BlockRef) bool { return s.pages[r.MarshalString()] != nil })
	s.mu.RUnlock()
	if !reaches {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trackLocked(ref.MarshalString(), refs)
}

// trackLocked records the stored page under key with its child list, unless
// it is kept. Caller holds mu.
func (s *pageSet) trackLocked(key string, refs []*block.BlockRef) {
	if s.pages[key] == nil {
		s.pages[key] = &trackedPage{refs: refs}
	}
}

// get returns the kept page ref names, or nil.
func (s *pageSet) get(ref *block.BlockRef) *block.PutBatchEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if page := s.pages[ref.MarshalString()]; page != nil {
		return page.entry
	}
	return nil
}

// reach returns the keys of the tracked pages the roots reach through tracked
// pages. Caller holds mu.
func (s *pageSet) reach(roots []*block.BlockRef) map[string]struct{} {
	reached := make(map[string]struct{})
	stack := slices.Clone(roots)
	for len(stack) != 0 {
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if ref.GetEmpty() {
			continue
		}
		key := ref.MarshalString()
		page := s.pages[key]
		if page == nil {
			continue
		}
		if _, ok := reached[key]; ok {
			continue
		}
		reached[key] = struct{}{}
		stack = append(stack, page.refs...)
	}
	return reached
}

// remove drops the tracked page under key. Caller holds mu.
func (s *pageSet) remove(key string) {
	if e := s.pages[key].entry; e != nil {
		s.bytes -= len(e.Data)
		s.count--
	}
	delete(s.pages, key)
}

// restore puts taken pages back with the pins still waiting on them, except a
// page kept again since, whose waiting pins join the kept copy. A taken page
// replaces a stored record written since, since it carries the data the pins
// need.
func (s *pageSet) restore(taken map[string]*trackedPage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, page := range taken {
		page.pins = slices.DeleteFunc(page.pins, func(p *pagePin) bool { return p.release != nil || p.released })
		if cur := s.pages[key]; cur != nil && (cur.entry != nil || page.entry == nil) {
			cur.pins = append(cur.pins, page.pins...)
			continue
		}
		s.pages[key] = page
		if page.entry != nil {
			s.bytes += len(page.entry.Data)
			s.count++
		}
	}
}

// unpin releases pin on the page under key, the store pin once the page
// moved to the store.
func (s *pageSet) unpin(key string, pin *pagePin) {
	// Drop a waiting pin here, or release the store pin it moved to.
	s.mu.Lock()
	release := pin.release
	if release == nil {
		pin.released = true
		if page := s.pages[key]; page != nil {
			page.pins = slices.DeleteFunc(page.pins, func(p *pagePin) bool { return p == pin })
		}
	}
	s.mu.Unlock()
	if release != nil {
		release()
	}
}

// unkept returns the refs that name no tracked page.
func (s *pageSet) unkept(refs []*block.BlockRef) []*block.BlockRef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.DeleteFunc(slices.Clone(refs), func(ref *block.BlockRef) bool {
		return s.pages[ref.MarshalString()] != nil
	})
}

// _ is a type assertion
var (
	_ bstore.BlockStore                = (*worldPages)(nil)
	_ block.AtomicPublisher            = (*worldPages)(nil)
	_ block.RootRetainer               = (*worldPages)(nil)
	_ block.DeferFlusher               = (*worldPages)(nil)
	_ block.DecodedBlockCacheFreshener = (*pageView)(nil)
)
