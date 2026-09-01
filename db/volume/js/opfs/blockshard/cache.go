//go:build js

package blockshard

import (
	"context"
	"io"
	"slices"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

const (
	cachedSegmentBlockSize = 64 * 1024
	maxCachedSegmentSpans  = 4
	maxCachedSegmentRead   = cachedSegmentBlockSize * maxCachedSegmentSpans
)

type segmentReader interface {
	io.ReaderAt
	Size() (int64, error)
}

// segmentDataSpan is one request-local immutable range and its backing allocation.
type segmentDataSpan struct {
	start int64
	data  []byte
}

// segmentFillRange identifies one exact aligned snapshot read.
type segmentFillRange struct {
	start int64
	end   int64
}

// segmentFill publishes one in-flight range result to equal waiters.
type segmentFill struct {
	key      segmentFillRange
	done     chan struct{}
	span     segmentDataSpan
	resource *cacheResource
	err      error
}

// localCachedSpan retains one standalone span behind aligned block views.
type localCachedSpan struct {
	segmentDataSpan
}

type cachedSegmentFile struct {
	// rd supplies immutable file bytes and size.
	rd   segmentReader
	size int64

	// coordinator and entry select engine-wide admission.
	coordinator *cacheCoordinator
	entry       *cacheEntry

	// mu guards standalone spans, fills, and recency.
	mu     sync.Mutex
	blocks map[int64]*localCachedSpan
	order  []*localCachedSpan
	fills  map[segmentFillRange]*segmentFill
}

func newCachedSegmentFile(rd segmentReader, size int64) *cachedSegmentFile {
	if size == 0 {
		if resolved, err := rd.Size(); err == nil {
			size = resolved
		}
	}
	return &cachedSegmentFile{
		rd:     rd,
		size:   size,
		blocks: make(map[int64]*localCachedSpan),
		fills:  make(map[segmentFillRange]*segmentFill),
	}
}

func newCoordinatedSegmentFile(
	rd segmentReader,
	size int64,
	coordinator *cacheCoordinator,
	entry *cacheEntry,
) *cachedSegmentFile {
	if size == 0 {
		if resolved, err := rd.Size(); err == nil {
			size = resolved
		}
	}
	return &cachedSegmentFile{
		rd:          rd,
		size:        size,
		coordinator: coordinator,
		entry:       entry,
	}
}

func (f *cachedSegmentFile) ReadAt(p []byte, off int64) (int, error) {
	return f.readAt(p, off, nil)
}

func (f *cachedSegmentFile) readAt(p []byte, off int64, lease *segmentCacheLease) (int, error) {
	// Return immediately for an empty request.
	if len(p) == 0 {
		return 0, nil
	}

	// Bypass retained spans for requests above the shipping threshold.
	if len(p) > maxCachedSegmentRead {
		if f.coordinator != nil {
			f.coordinator.recordBypass()
		}
		n, err := f.rd.ReadAt(p, off)
		if f.coordinator != nil {
			f.coordinator.recordRead(n)
		}
		return n, err
	}

	// Clamp the cacheable request to immutable file bounds.
	if off >= f.size {
		return 0, io.EOF
	}
	readEnd := min(off+int64(len(p)), f.size)

	// Fill and copy each resident or missing aligned span.
	blockOff := alignSegmentOffset(off)
	endBlock := alignSegmentOffset(readEnd - 1)
	copied := 0
	for blockOff <= endBlock {
		span, err := f.getSpan(blockOff, endBlock, lease)
		if err != nil {
			return copied, err
		}
		spanEnd := span.start + int64(len(span.data))
		copyStart := max(off, blockOff)
		copyEnd := min(readEnd, spanEnd)
		if copyEnd <= copyStart {
			return copied, io.ErrUnexpectedEOF
		}
		copied += copy(
			p[copyStart-off:copyEnd-off],
			span.data[copyStart-span.start:copyEnd-span.start],
		)
		blockOff = alignSegmentOffset(spanEnd-1) + cachedSegmentBlockSize
	}

	// Preserve short-read and EOF behavior at the final file span.
	if copied < len(p) {
		return copied, io.EOF
	}
	return copied, nil
}

func (f *cachedSegmentFile) getSpan(
	blockOff int64,
	endBlock int64,
	lease *segmentCacheLease,
) (segmentDataSpan, error) {
	// Coordinate resident spans and equal in-flight fills for engine entries.
	if f.coordinator != nil {
		span, fill, leader := f.coordinator.startSpanFill(
			f.entry,
			lease,
			blockOff,
			endBlock,
			f.size,
		)
		if fill == nil {
			return span, nil
		}
		if !leader {
			return f.coordinator.awaitSpanFill(fill, lease)
		}
		span, err := f.readSpan(fill.key)
		return f.coordinator.finishSpanFill(f.entry, lease, fill, span, err)
	}

	// Use the standalone span cache for operation-local readers.
	return f.getLocalSpan(blockOff, endBlock)
}

func (f *cachedSegmentFile) readSpan(key segmentFillRange) (segmentDataSpan, error) {
	// Read one exact request-bounded allocation from the immutable snapshot.
	span := segmentDataSpan{
		start: key.start,
		data:  make([]byte, key.end-key.start),
	}
	n, err := f.rd.ReadAt(span.data, key.start)
	if f.coordinator != nil {
		f.coordinator.recordRead(n)
	}
	if err != nil && err != io.EOF {
		return segmentDataSpan{}, err
	}
	if n != len(span.data) {
		return segmentDataSpan{}, io.ErrUnexpectedEOF
	}
	return span, nil
}

func (f *cachedSegmentFile) getLocalSpan(blockOff, endBlock int64) (segmentDataSpan, error) {
	// Reuse a resident standalone span.
	f.mu.Lock()
	if cached := f.blocks[blockOff]; cached != nil {
		f.touchLocalSpanLocked(cached)
		span := cached.segmentDataSpan
		f.mu.Unlock()
		return span, nil
	}

	// Join an equal fill or register the consecutive missing range.
	key := f.localFillRangeLocked(blockOff, endBlock)
	if fill := f.fills[key]; fill != nil {
		f.mu.Unlock()
		<-fill.done
		return fill.span, fill.err
	}
	fill := &segmentFill{key: key, done: make(chan struct{})}
	f.fills[key] = fill
	f.mu.Unlock()

	// Read the range without holding the standalone cache mutex.
	span, err := f.readSpan(key)

	// Publish one span when no unequal fill already covered its blocks.
	f.mu.Lock()
	fill.span = span
	fill.err = err
	if err == nil && !f.localRangeOverlapsLocked(key) {
		if len(f.order) >= maxCachedSegmentSpans {
			f.removeLocalSpanLocked(f.order[0])
		}
		cached := &localCachedSpan{segmentDataSpan: span}
		for off := key.start; off < key.end; off += cachedSegmentBlockSize {
			f.blocks[off] = cached
		}
		f.order = append(f.order, cached)
	}
	delete(f.fills, key)
	close(fill.done)
	f.mu.Unlock()
	return span, err
}

func (f *cachedSegmentFile) localFillRangeLocked(blockOff, endBlock int64) segmentFillRange {
	// Stop the missing run at the next resident block or request boundary.
	end := min(endBlock+cachedSegmentBlockSize, f.size)
	for off := blockOff + cachedSegmentBlockSize; off <= endBlock; off += cachedSegmentBlockSize {
		if f.blocks[off] != nil {
			end = off
			break
		}
	}
	return segmentFillRange{start: blockOff, end: end}
}

func (f *cachedSegmentFile) localRangeOverlapsLocked(key segmentFillRange) bool {
	// Reject admission when an unequal concurrent fill already published a block.
	for off := key.start; off < key.end; off += cachedSegmentBlockSize {
		if f.blocks[off] != nil {
			return true
		}
	}
	return false
}

func (f *cachedSegmentFile) touchLocalSpanLocked(span *localCachedSpan) {
	// Move the reused backing allocation to the recency tail.
	idx := slices.Index(f.order, span)
	if idx < 0 || idx == len(f.order)-1 {
		return
	}
	copy(f.order[idx:], f.order[idx+1:])
	f.order[len(f.order)-1] = span
}

func (f *cachedSegmentFile) removeLocalSpanLocked(span *localCachedSpan) {
	// Remove every aligned block view backed by the evicted allocation.
	for off := span.start; off < span.start+int64(len(span.data)); off += cachedSegmentBlockSize {
		if f.blocks[off] == span {
			delete(f.blocks, off)
		}
	}
	f.order = f.order[1:]
}

func (f *cachedSegmentFile) Size() (int64, error) {
	return f.size, nil
}

func (s *Shard) setManifestLocked(m *Manifest) {
	// Publish the newest observed manifest generation.
	s.manifest = m
	if m.Generation > s.latestGen {
		s.latestGen = m.Generation
	}

	// Retire cache entries omitted by the new manifest.
	s.cache.retainVisible(s.id, m.ReferencedFiles())
}

// installObservedManifest installs a newer manifest and returns the current
// manifest at the installation point. Slot reads may race with publication, so
// the generation comparison and installation occur together under mu.
func (s *Shard) installObservedManifest(m *Manifest) *Manifest {
	s.mu.Lock()
	defer s.mu.Unlock()

	if m.Generation > s.manifest.Generation {
		s.setManifestLocked(m)
	}
	return s.manifest.Clone()
}

func (s *Shard) acquireSegment(ctx context.Context, meta *SegmentMeta) (*segmentCacheLease, error) {
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/acquire-segment")
	defer task.End()

	// Delegate admission and immutable file opening to the engine coordinator.
	return s.cache.acquireSegment(
		ctx,
		cacheKey{shardID: s.id, filename: meta.Filename},
		meta,
		func() (segmentReader, error) {
			_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/acquire-segment/open-snapshot")
			snapshot, err := opfs.OpenReadSnapshot(s.dir, meta.Filename)
			subtask.End()
			return snapshot, err
		},
	)
}

func (s *Shard) dropSegmentFile(filename string) {
	s.cache.remove(cacheKey{shardID: s.id, filename: filename})
}

func loadLookupMeta(ctx context.Context, f segmentReader, meta *SegmentMeta) (*segment.LookupMeta, error) {
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/load-lookup-meta")
	defer task.End()

	// Resolve file size when the manifest predates stored size metadata.
	var err error
	var subtask *trace.Task
	size := int64(meta.Size)
	if size == 0 {
		_, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/load-lookup-meta/stat-size")
		size, err = f.Size()
		subtask.End()
		if err != nil {
			return nil, errors.Wrap(err, "get segment size")
		}
	}

	// Decode the sparse index, key range, and Bloom filter.
	_, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/load-lookup-meta/load")
	lookup, err := segment.LoadLookupMeta(f, size)
	subtask.End()
	if err != nil {
		return nil, errors.Wrap(err, "load segment lookup metadata")
	}
	return lookup, nil
}

func alignSegmentOffset(off int64) int64 {
	return (off / cachedSegmentBlockSize) * cachedSegmentBlockSize
}
