//go:build js

package blockshard

import (
	"context"
	"io"
	"sync"
	"unsafe"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

const (
	defaultCacheByteLimit   = 64 * 1024 * 1024
	defaultCacheHandleLimit = 512
)

type cacheKey struct {
	shardID  int
	filename string
}

type cacheResourceKind uint8

const (
	cacheResourceLookup cacheResourceKind = iota
	cacheResourceSpan
)

type cacheResource struct {
	entry    *cacheEntry
	kind     cacheResourceKind
	span     *cachedSpan
	bytes    uint64
	pins     int
	prev     *cacheResource
	next     *cacheResource
	resident bool
}

// cachedSpan retains one admitted backing allocation and its cache resource.
type cachedSpan struct {
	segmentDataSpan
	resource *cacheResource
}

// cachedBlock maps one aligned offset to its shared admitted span.
type cachedBlock struct {
	span *cachedSpan
}

type cacheEntry struct {
	key cacheKey

	file           *cachedSegmentFile
	lookup         *segment.LookupMeta
	lookupResource *cacheResource
	blocks         map[int64]*cachedBlock
	blockSpans     int
	fills          map[segmentFillRange]*segmentFill
	leases         int
	loading        chan struct{}
	removed        bool
}

// cacheStats reports the engine-local immutable segment cache state.
type cacheStats struct {
	ChargedBytes     uint64
	MetadataBytes    uint64
	BlockBytes       uint64
	PinnedBytes      uint64
	PeakChargedBytes uint64
	LiveHandles      uint64
	PeakLiveHandles  uint64
	Admissions       uint64
	Bypasses         uint64
	Evictions        uint64
	Hits             uint64
	Misses           uint64
	SharedFills      uint64
	ReadCalls        uint64
	FetchedBytes     uint64
	ReleaseErrors    uint64
}

type cacheCoordinator struct {
	// mu guards every field below it.
	mu   sync.Mutex
	cond *sync.Cond

	byteLimit   uint64
	handleLimit uint64
	entries     map[cacheKey]*cacheEntry
	lruHead     *cacheResource
	lruTail     *cacheResource
	stats       cacheStats
	closed      bool
}

func newCacheCoordinator(byteLimit, handleLimit uint64) *cacheCoordinator {
	c := &cacheCoordinator{
		byteLimit:   byteLimit,
		handleLimit: handleLimit,
		entries:     make(map[cacheKey]*cacheEntry),
	}
	c.cond = sync.NewCond(&c.mu)
	return c
}

func newDefaultCacheCoordinator() *cacheCoordinator {
	return newCacheCoordinator(defaultCacheByteLimit, defaultCacheHandleLimit)
}

type segmentCacheLease struct {
	cache     *cacheCoordinator
	entry     *cacheEntry
	lookup    *segment.LookupMeta
	localFile *cachedSegmentFile
	resources map[*cacheResource]struct{}
	once      sync.Once
}

func (l *segmentCacheLease) ReadAt(p []byte, off int64) (int, error) {
	if l.localFile != nil {
		return l.localFile.ReadAt(p, off)
	}
	return l.entry.file.readAt(p, off, l)
}

func (l *segmentCacheLease) Size() (int64, error) {
	if l.localFile != nil {
		return l.localFile.Size()
	}
	return l.entry.file.Size()
}

func (l *segmentCacheLease) Release() {
	l.once.Do(func() {
		if l.localFile != nil {
			if err := closeSegmentReader(l.localFile.rd); err != nil {
				l.cache.recordReleaseError()
			}
			return
		}
		l.cache.releaseLease(l)
	})
}

func (c *cacheCoordinator) acquireSegment(
	ctx context.Context,
	key cacheKey,
	meta *SegmentMeta,
	open func() (segmentReader, error),
) (*segmentCacheLease, error) {
	for {
		// Reject acquisitions after engine close begins.
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return nil, errors.New("blockshard cache is closed")
		}
		entry := c.entries[key]

		// Wait for the current metadata loader without holding the cache mutex.
		if entry != nil && entry.loading != nil {
			loading := entry.loading
			c.mu.Unlock()
			select {
			case <-loading:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// Reuse an admitted entry or become its metadata loader.
		if entry != nil && !entry.removed {
			lease := c.newLeaseLocked(entry)

			// Return resident metadata under the new lease pin.
			if entry.lookupResource != nil {
				lease.lookup = entry.lookup
				c.pinResourceLocked(lease, entry.lookupResource)
				c.mu.Unlock()
				return lease, nil
			}

			// Reload evicted metadata through the retained file handle.
			entry.loading = make(chan struct{})
			c.mu.Unlock()
			lookup, err := loadLookupMeta(ctx, lease, meta)
			return c.finishLookupLoad(lease, lookup, err)
		}

		// Use operation-local state when the handle budget cannot admit the entry.
		closers, admitted := c.reserveHandleLocked()
		if !admitted || entry != nil {
			c.stats.Bypasses++
			c.mu.Unlock()
			c.closeFiles(closers)
			return c.acquireLocal(ctx, meta, open)
		}

		// Publish the handle reservation before opening the immutable file.
		entry = &cacheEntry{
			key:     key,
			blocks:  make(map[int64]*cachedBlock),
			fills:   make(map[segmentFillRange]*segmentFill),
			leases:  1,
			loading: make(chan struct{}),
		}
		c.entries[key] = entry
		c.stats.LiveHandles++
		c.stats.PeakLiveHandles = max(c.stats.PeakLiveHandles, c.stats.LiveHandles)
		lease := &segmentCacheLease{
			cache:     c,
			entry:     entry,
			resources: make(map[*cacheResource]struct{}),
		}
		c.mu.Unlock()
		c.closeFiles(closers)

		// Open the file and load metadata outside the coordinator mutex.
		rd, err := open()
		if err != nil {
			c.failLookupLoad(lease)
			return nil, err
		}
		entry.file = newCoordinatedSegmentFile(rd, int64(meta.Size), c, entry)
		lookup, err := loadLookupMeta(ctx, lease, meta)
		return c.finishLookupLoad(lease, lookup, err)
	}
}

func (c *cacheCoordinator) acquireLocal(
	ctx context.Context,
	meta *SegmentMeta,
	open func() (segmentReader, error),
) (*segmentCacheLease, error) {
	// Open one operation-local reader outside the admitted handle budget.
	rd, err := open()
	if err != nil {
		return nil, err
	}
	lease := &segmentCacheLease{cache: c, localFile: newCachedSegmentFile(rd, int64(meta.Size))}

	// Load metadata and transfer reader release to the returned lease.
	lookup, err := loadLookupMeta(ctx, lease, meta)
	if err != nil {
		lease.Release()
		return nil, err
	}
	lease.lookup = lookup
	return lease, nil
}

func (c *cacheCoordinator) finishLookupLoad(
	lease *segmentCacheLease,
	lookup *segment.LookupMeta,
	loadErr error,
) (*segmentCacheLease, error) {
	// Resolve the loader state before waking competing acquisitions.
	c.mu.Lock()
	entry := lease.entry
	loading := entry.loading
	entry.loading = nil
	var closers []*cachedSegmentFile
	if loadErr != nil {
		entry.removed = true
	}

	// Admit decoded metadata when capacity and lifecycle still allow it.
	if loadErr == nil && !entry.removed && !c.closed {
		charge := lookupMetaCharge(lookup)
		var admitted bool
		closers, admitted = c.reserveBytesLocked(charge)

		// Publish admitted metadata and pin it for the loading operation.
		if admitted {
			resource := &cacheResource{
				entry: entry,
				kind:  cacheResourceLookup,
				bytes: charge,
			}
			entry.lookup = lookup
			entry.lookupResource = resource
			c.addResourceLocked(resource)
			c.pinResourceLocked(lease, resource)
			c.stats.Admissions++
		}

		// Record metadata retained only by the current operation.
		if !admitted {
			c.stats.Bypasses++
		}
	}

	// Publish the load result and release evicted handles outside the mutex.
	lease.lookup = lookup
	close(loading)
	c.cond.Broadcast()
	c.mu.Unlock()
	c.closeFiles(closers)

	// Roll back the lease after a failed metadata read.
	if loadErr != nil {
		lease.Release()
		return nil, loadErr
	}
	return lease, nil
}

func (c *cacheCoordinator) failLookupLoad(lease *segmentCacheLease) {
	// Retire the handle reservation and wake metadata waiters.
	c.mu.Lock()
	entry := lease.entry
	loading := entry.loading
	entry.loading = nil
	entry.removed = true
	close(loading)
	c.cond.Broadcast()
	c.mu.Unlock()

	// Complete handle rollback through normal lease release.
	lease.Release()
}

func (c *cacheCoordinator) newLeaseLocked(entry *cacheEntry) *segmentCacheLease {
	entry.leases++
	return &segmentCacheLease{
		cache:     c,
		entry:     entry,
		resources: make(map[*cacheResource]struct{}),
	}
}

func (c *cacheCoordinator) releaseLease(lease *segmentCacheLease) {
	// Drop every resource pin held by the completed operation.
	c.mu.Lock()
	entry := lease.entry
	for resource := range lease.resources {
		resource.pins--
		if resource.pins == 0 {
			c.stats.PinnedBytes -= resource.bytes
		}
	}
	entry.leases--

	// Complete deferred retirement and detach an empty entry.
	var closers []*cachedSegmentFile
	if entry.removed {
		c.removeIdleResourcesLocked(entry, false)
	}
	if file := c.detachIdleEntryLocked(entry); file != nil {
		closers = append(closers, file)
	}
	c.cond.Broadcast()
	c.mu.Unlock()

	// Release detached driver handles outside the coordinator mutex.
	c.closeFiles(closers)
}

func (c *cacheCoordinator) startSpanFill(
	entry *cacheEntry,
	lease *segmentCacheLease,
	blockOff int64,
	endBlock int64,
	size int64,
) (segmentDataSpan, *segmentFill, bool) {
	// Reuse and pin a resident span covering the next requested block.
	c.mu.Lock()
	if block := entry.blocks[blockOff]; block != nil {
		c.stats.Hits++
		c.pinResourceLocked(lease, block.span.resource)
		span := block.span.segmentDataSpan
		c.mu.Unlock()
		return span, nil, false
	}

	// Bound the fill at the next resident block, request edge, or file edge.
	end := min(endBlock+cachedSegmentBlockSize, size)
	var misses uint64
	for off := blockOff; off <= endBlock; off += cachedSegmentBlockSize {
		if off != blockOff && entry.blocks[off] != nil {
			end = off
			break
		}
		misses++
	}
	c.stats.Misses += misses
	key := segmentFillRange{start: blockOff, end: end}

	// Join an equal in-flight range without starting another snapshot read.
	if entry.fills == nil {
		entry.fills = make(map[segmentFillRange]*segmentFill)
	}
	if fill := entry.fills[key]; fill != nil {
		c.stats.SharedFills++
		c.cond.Broadcast()
		c.mu.Unlock()
		return segmentDataSpan{}, fill, false
	}

	// Publish this caller as the range leader.
	fill := &segmentFill{key: key, done: make(chan struct{})}
	entry.fills[key] = fill
	c.mu.Unlock()
	return segmentDataSpan{}, fill, true
}

func (c *cacheCoordinator) awaitSpanFill(
	fill *segmentFill,
	lease *segmentCacheLease,
) (segmentDataSpan, error) {
	// Wait for the leader to publish bytes or its exact failure.
	<-fill.done
	if fill.err != nil {
		return segmentDataSpan{}, fill.err
	}

	// Pin the admitted resource when it still resides in the cache.
	c.mu.Lock()
	if fill.resource != nil && fill.resource.resident {
		c.pinResourceLocked(lease, fill.resource)
	}
	c.mu.Unlock()
	return fill.span, nil
}

func (c *cacheCoordinator) finishSpanFill(
	entry *cacheEntry,
	lease *segmentCacheLease,
	fill *segmentFill,
	span segmentDataSpan,
	fillErr error,
) (segmentDataSpan, error) {
	// Publish completion state before waking equal-range waiters.
	c.mu.Lock()
	fill.span = span
	fill.err = fillErr
	var closers []*cachedSegmentFile
	admit := fillErr == nil

	// Keep successful bytes operation-local after retirement or an unequal race.
	if admit && (entry.removed || c.closed) {
		c.stats.Bypasses++
		admit = false
	}
	if admit {
		for off := fill.key.start; off < fill.key.end; off += cachedSegmentBlockSize {
			if entry.blocks[off] != nil {
				c.stats.Bypasses++
				admit = false
				break
			}
		}
	}

	// Preserve the per-segment span count without evicting a pinned span.
	if admit && entry.blockSpans >= maxCachedSegmentSpans {
		resource := c.oldestSpanLocked(entry)
		if resource == nil {
			c.stats.Bypasses++
			admit = false
		}
		if resource != nil {
			c.removeResourceLocked(resource, true)
		}
	}

	// Reserve the exact backing allocation through the engine byte budget.
	charge := uint64(cap(span.data))
	if admit {
		var admitted bool
		var reservedClosers []*cachedSegmentFile
		reservedClosers, admitted = c.reserveBytesLocked(charge)
		closers = append(closers, reservedClosers...)
		if !admitted {
			c.stats.Bypasses++
			admit = false
		}
	}

	// Publish one resource and all aligned block views of its backing allocation.
	if admit {
		cached := &cachedSpan{segmentDataSpan: span}
		resource := &cacheResource{
			entry: entry,
			kind:  cacheResourceSpan,
			span:  cached,
			bytes: charge,
		}
		cached.resource = resource
		for off := fill.key.start; off < fill.key.end; off += cachedSegmentBlockSize {
			entry.blocks[off] = &cachedBlock{span: cached}
		}
		entry.blockSpans++
		fill.resource = resource
		c.addResourceLocked(resource)
		c.pinResourceLocked(lease, resource)
		c.stats.Admissions++
	}

	// Remove the in-flight identity and wake every waiter with the same result.
	delete(entry.fills, fill.key)
	close(fill.done)
	c.cond.Broadcast()
	c.mu.Unlock()
	c.closeFiles(closers)
	return span, fillErr
}

func (c *cacheCoordinator) recordRead(n int) {
	c.mu.Lock()
	c.stats.ReadCalls++
	c.stats.FetchedBytes += uint64(max(n, 0))
	c.mu.Unlock()
}

func (c *cacheCoordinator) recordBypass() {
	c.mu.Lock()
	c.stats.Bypasses++
	c.mu.Unlock()
}

func (c *cacheCoordinator) remove(key cacheKey) {
	// Mark the entry retired and drop every idle resource.
	c.mu.Lock()
	entry := c.entries[key]
	if entry == nil {
		c.mu.Unlock()
		return
	}
	entry.removed = true
	c.removeIdleResourcesLocked(entry, false)
	var closers []*cachedSegmentFile
	if file := c.detachIdleEntryLocked(entry); file != nil {
		closers = append(closers, file)
	}
	c.mu.Unlock()

	// Release detached driver handles outside the coordinator mutex.
	c.closeFiles(closers)
}

func (c *cacheCoordinator) retainVisible(shardID int, refs map[string]struct{}) {
	// Retire every cached entry omitted by the shard manifest.
	c.mu.Lock()
	var closers []*cachedSegmentFile
	for key, entry := range c.entries {
		if key.shardID != shardID {
			continue
		}
		if _, ok := refs[key.filename]; ok {
			continue
		}
		entry.removed = true
		c.removeIdleResourcesLocked(entry, false)
		if file := c.detachIdleEntryLocked(entry); file != nil {
			closers = append(closers, file)
		}
	}
	c.mu.Unlock()

	// Release detached driver handles outside the coordinator mutex.
	c.closeFiles(closers)
}

func (c *cacheCoordinator) close() {
	// Stop new acquisitions and wait for active lookup steps to release.
	c.mu.Lock()
	c.closed = true
	for _, entry := range c.entries {
		entry.removed = true
	}
	for c.hasActiveLeasesLocked() {
		c.cond.Wait()
	}

	// Drain every resident resource and detach its handle.
	var closers []*cachedSegmentFile
	for _, entry := range c.entries {
		c.removeIdleResourcesLocked(entry, false)
		if file := c.detachIdleEntryLocked(entry); file != nil {
			closers = append(closers, file)
		}
	}
	c.mu.Unlock()

	// Release detached driver handles after cache state reaches zero.
	c.closeFiles(closers)
}

func (c *cacheCoordinator) snapshot() cacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

func (c *cacheCoordinator) hasActiveLeasesLocked() bool {
	for _, entry := range c.entries {
		if entry.leases != 0 || entry.loading != nil || len(entry.fills) != 0 {
			return true
		}
	}
	return false
}

func (c *cacheCoordinator) reserveHandleLocked() ([]*cachedSegmentFile, bool) {
	if c.handleLimit == 0 {
		return nil, false
	}

	// Evict idle resources until one cache handle reservation fits.
	var closers []*cachedSegmentFile
	for c.stats.LiveHandles >= c.handleLimit {
		resource := c.oldestEvictableLocked(true)
		if resource == nil {
			return closers, false
		}
		entry := resource.entry
		c.removeResourceLocked(resource, true)
		if file := c.detachIdleEntryLocked(entry); file != nil {
			closers = append(closers, file)
		}
	}
	return closers, true
}

func (c *cacheCoordinator) reserveBytesLocked(charge uint64) ([]*cachedSegmentFile, bool) {
	if charge > c.byteLimit {
		return nil, false
	}

	// Evict idle resources until the exact byte charge fits.
	var closers []*cachedSegmentFile
	for c.stats.ChargedBytes+charge > c.byteLimit {
		resource := c.oldestEvictableLocked(false)
		if resource == nil {
			return closers, false
		}
		entry := resource.entry
		c.removeResourceLocked(resource, true)
		if file := c.detachIdleEntryLocked(entry); file != nil {
			closers = append(closers, file)
		}
	}
	return closers, true
}

func (c *cacheCoordinator) oldestEvictableLocked(forHandle bool) *cacheResource {
	for resource := c.lruHead; resource != nil; resource = resource.next {
		if resource.pins != 0 {
			continue
		}
		if forHandle && resource.entry.leases != 0 {
			continue
		}
		return resource
	}
	return nil
}

func (c *cacheCoordinator) oldestSpanLocked(entry *cacheEntry) *cacheResource {
	for resource := c.lruHead; resource != nil; resource = resource.next {
		if resource.entry == entry && resource.kind == cacheResourceSpan && resource.pins == 0 {
			return resource
		}
	}
	return nil
}

func (c *cacheCoordinator) addResourceLocked(resource *cacheResource) {
	// Mark the resource resident and link it at the recency tail.
	resource.resident = true
	resource.prev = c.lruTail
	if c.lruTail == nil {
		c.lruHead = resource
	}
	if c.lruTail != nil {
		c.lruTail.next = resource
	}
	c.lruTail = resource

	// Charge its exact retained payload by resource class.
	c.stats.ChargedBytes += resource.bytes
	c.stats.PeakChargedBytes = max(c.stats.PeakChargedBytes, c.stats.ChargedBytes)
	switch resource.kind {
	case cacheResourceLookup:
		c.stats.MetadataBytes += resource.bytes
	case cacheResourceSpan:
		c.stats.BlockBytes += resource.bytes
	}
}

func (c *cacheCoordinator) pinResourceLocked(lease *segmentCacheLease, resource *cacheResource) {
	if _, ok := lease.resources[resource]; !ok {
		if resource.pins == 0 {
			c.stats.PinnedBytes += resource.bytes
		}
		resource.pins++
		lease.resources[resource] = struct{}{}
	}
	c.touchResourceLocked(resource)
}

func (c *cacheCoordinator) touchResourceLocked(resource *cacheResource) {
	if c.lruTail == resource {
		return
	}
	if resource.prev == nil {
		c.lruHead = resource.next
	}
	if resource.prev != nil {
		resource.prev.next = resource.next
	}
	if resource.next != nil {
		resource.next.prev = resource.prev
	}
	resource.prev = c.lruTail
	resource.next = nil
	if c.lruTail != nil {
		c.lruTail.next = resource
	}
	c.lruTail = resource
	if c.lruHead == nil {
		c.lruHead = resource
	}
}

func (c *cacheCoordinator) removeIdleResourcesLocked(entry *cacheEntry, eviction bool) {
	if entry.lookupResource != nil && entry.lookupResource.pins == 0 {
		c.removeResourceLocked(entry.lookupResource, eviction)
	}
	for _, block := range entry.blocks {
		if block.span.resource != nil && block.span.resource.pins == 0 {
			c.removeResourceLocked(block.span.resource, eviction)
		}
	}
}

func (c *cacheCoordinator) removeResourceLocked(resource *cacheResource, eviction bool) {
	if !resource.resident || resource.pins != 0 {
		return
	}

	// Unlink the resource from the recency list.
	if resource.prev == nil {
		c.lruHead = resource.next
	}
	if resource.prev != nil {
		resource.prev.next = resource.next
	}
	if resource.next == nil {
		c.lruTail = resource.prev
	}
	if resource.next != nil {
		resource.next.prev = resource.prev
	}
	resource.resident = false

	// Remove the typed payload from its segment entry.
	entry := resource.entry
	switch resource.kind {
	case cacheResourceLookup:
		entry.lookup = nil
		entry.lookupResource = nil
		c.stats.MetadataBytes -= resource.bytes
	case cacheResourceSpan:
		span := resource.span
		for off := span.start; off < span.start+int64(len(span.data)); off += cachedSegmentBlockSize {
			if block := entry.blocks[off]; block != nil && block.span == span {
				delete(entry.blocks, off)
			}
		}
		entry.blockSpans--
		span.resource = nil
		resource.span = nil
		c.stats.BlockBytes -= resource.bytes
	}

	// Release its aggregate charge and record capacity eviction.
	c.stats.ChargedBytes -= resource.bytes
	if eviction {
		c.stats.Evictions++
	}
}

func (c *cacheCoordinator) detachIdleEntryLocked(entry *cacheEntry) *cachedSegmentFile {
	// Keep entries with active operations or resident resources attached.
	if entry.leases != 0 ||
		entry.loading != nil ||
		len(entry.fills) != 0 ||
		entry.lookupResource != nil ||
		len(entry.blocks) != 0 {
		return nil
	}
	if current := c.entries[entry.key]; current != entry {
		return nil
	}

	// Remove the handle reservation before transferring driver release.
	delete(c.entries, entry.key)
	c.stats.LiveHandles--
	if entry.file == nil {
		return nil
	}
	file := entry.file
	entry.file = nil
	return file
}

func lookupMetaCharge(meta *segment.LookupMeta) uint64 {
	bytes := uint64(unsafe.Sizeof(*meta))
	bytes += uint64(unsafe.Sizeof(*meta.Header))
	bytes += uint64(cap(meta.MinKey) + cap(meta.MaxKey))
	bytes += uint64(cap(meta.Index)) * uint64(unsafe.Sizeof(segment.IndexEntry{}))
	for i := range meta.Index {
		bytes += uint64(cap(meta.Index[i].Key))
	}
	if meta.Bloom != nil {
		bytes += uint64(unsafe.Sizeof(*meta.Bloom))
		if meta.Header.BloomSize >= 5 {
			bytes += uint64(meta.Header.BloomSize - 5)
		}
	}
	return bytes
}

func (c *cacheCoordinator) closeFiles(files []*cachedSegmentFile) {
	// Release each detached driver handle exactly once.
	var failures uint64
	for _, file := range files {
		if err := closeSegmentReader(file.rd); err != nil {
			failures++
		}
	}

	// Preserve asynchronous release failures in observable cache state.
	if failures != 0 {
		c.mu.Lock()
		c.stats.ReleaseErrors += failures
		c.mu.Unlock()
	}
}

func (c *cacheCoordinator) recordReleaseError() {
	c.mu.Lock()
	c.stats.ReleaseErrors++
	c.mu.Unlock()
}

func closeSegmentReader(reader segmentReader) error {
	closer, ok := reader.(io.Closer)
	if !ok {
		return nil
	}
	return closer.Close()
}
