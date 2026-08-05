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
	maxCachedSegmentBlocks = 4
	maxCachedSegmentRead   = cachedSegmentBlockSize * maxCachedSegmentBlocks
)

type segmentReader interface {
	io.ReaderAt
	Size() (int64, error)
}

type cachedSegmentFile struct {
	// rd supplies immutable file bytes and size.
	rd   segmentReader
	size int64

	// coordinator and entry select engine-wide admission.
	coordinator *cacheCoordinator
	entry       *cacheEntry

	// mu guards standalone blocks and their recency order.
	mu     sync.Mutex
	blocks map[int64][]byte
	order  []int64
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
		blocks: make(map[int64][]byte),
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

	// Copy every intersecting aligned block into the caller buffer.
	startBlock := alignSegmentOffset(off)
	endBlock := alignSegmentOffset(readEnd - 1)
	for blockOff := startBlock; blockOff <= endBlock; blockOff += cachedSegmentBlockSize {
		block, err := f.getBlock(blockOff, lease)
		if err != nil {
			return 0, err
		}
		blockStart := max(off, blockOff)
		blockEnd := min(readEnd, blockOff+int64(len(block)))
		copyStart := blockStart - off
		copyEnd := blockEnd - off
		if copyEnd <= copyStart {
			continue
		}
		srcStart := blockStart - blockOff
		srcEnd := blockEnd - blockOff
		copy(p[copyStart:copyEnd], block[srcStart:srcEnd])
	}

	// Preserve short-read and EOF behavior at the final file block.
	n := int(readEnd - off)
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (f *cachedSegmentFile) getBlock(blockOff int64, lease *segmentCacheLease) ([]byte, error) {
	// Reuse a resident coordinated block.
	if f.coordinator != nil {
		if block, ok := f.coordinator.getBlock(f.entry, lease, blockOff); ok {
			return block, nil
		}
	}

	// Reuse a resident standalone block.
	if f.coordinator == nil {
		f.mu.Lock()
		if block := f.blocks[blockOff]; block != nil {
			f.touchBlockLocked(blockOff)
			f.mu.Unlock()
			return block, nil
		}
		f.mu.Unlock()
	}

	// Read one exact aligned block from the immutable file.
	blockEnd := min(blockOff+cachedSegmentBlockSize, f.size)
	if blockEnd <= blockOff {
		return nil, io.EOF
	}
	buf := make([]byte, blockEnd-blockOff)
	n, err := f.rd.ReadAt(buf, blockOff)
	if f.coordinator != nil {
		f.coordinator.recordRead(n)
	}
	if err != nil && err != io.EOF {
		return nil, err
	}
	if n <= 0 {
		return nil, io.EOF
	}
	block := buf[:n]

	// Admit coordinated data through the engine-wide budget.
	if f.coordinator != nil {
		return f.coordinator.admitBlock(f.entry, lease, blockOff, block), nil
	}

	// Publish standalone data under the original per-file policy.
	f.mu.Lock()
	if existing := f.blocks[blockOff]; existing != nil {
		f.touchBlockLocked(blockOff)
		f.mu.Unlock()
		return existing, nil
	}
	f.blocks[blockOff] = block
	f.order = append(f.order, blockOff)
	if len(f.order) > maxCachedSegmentBlocks {
		evict := f.order[0]
		f.order = f.order[1:]
		delete(f.blocks, evict)
	}
	f.mu.Unlock()
	return block, nil
}

func (f *cachedSegmentFile) touchBlockLocked(blockOff int64) {
	idx := slices.Index(f.order, blockOff)
	if idx < 0 || idx == len(f.order)-1 {
		return
	}
	copy(f.order[idx:], f.order[idx+1:])
	f.order[len(f.order)-1] = blockOff
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

func (s *Shard) acquireSegment(ctx context.Context, meta *SegmentMeta) (*segmentCacheLease, error) {
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/acquire-segment")
	defer task.End()

	// Delegate admission and immutable file opening to the engine coordinator.
	return s.cache.acquireSegment(
		ctx,
		cacheKey{shardID: s.id, filename: meta.Filename},
		meta,
		func() (segmentReader, error) {
			_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/acquire-segment/open-file")
			file, err := opfs.OpenAsyncFile(s.dir, meta.Filename)
			subtask.End()
			return file, err
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
