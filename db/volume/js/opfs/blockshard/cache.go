//go:build js

package blockshard

import (
	"context"

	trace "github.com/s4wave/spacewave/db/traceutil"
	"io"
	"slices"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
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
	rd   segmentReader
	size int64

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

func (f *cachedSegmentFile) ReadAt(p []byte, off int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(p) > maxCachedSegmentRead {
		return f.rd.ReadAt(p, off)
	}
	if off >= f.size {
		return 0, io.EOF
	}

	readEnd := off + int64(len(p))
	if readEnd > f.size {
		readEnd = f.size
	}

	startBlock := alignSegmentOffset(off)
	endBlock := alignSegmentOffset(readEnd - 1)
	for blockOff := startBlock; blockOff <= endBlock; blockOff += cachedSegmentBlockSize {
		block, err := f.getBlock(blockOff)
		if err != nil {
			return 0, err
		}
		blockStart := maxInt64(off, blockOff)
		blockEnd := minInt64(readEnd, blockOff+int64(len(block)))
		copyStart := blockStart - off
		copyEnd := blockEnd - off
		if copyEnd <= copyStart {
			continue
		}
		srcStart := blockStart - blockOff
		srcEnd := blockEnd - blockOff
		copy(p[copyStart:copyEnd], block[srcStart:srcEnd])
	}

	n := int(readEnd - off)
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (f *cachedSegmentFile) getBlock(blockOff int64) ([]byte, error) {
	f.mu.Lock()
	if block := f.blocks[blockOff]; block != nil {
		f.touchBlockLocked(blockOff)
		f.mu.Unlock()
		return block, nil
	}
	f.mu.Unlock()

	blockEnd := minInt64(blockOff+cachedSegmentBlockSize, f.size)
	if blockEnd <= blockOff {
		return nil, io.EOF
	}
	buf := make([]byte, blockEnd-blockOff)
	n, err := f.rd.ReadAt(buf, blockOff)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if n <= 0 {
		return nil, io.EOF
	}
	block := buf[:n]

	f.mu.Lock()
	if existing := f.blocks[blockOff]; existing != nil {
		f.touchBlockLocked(blockOff)
		block = existing
	} else {
		f.blocks[blockOff] = block
		f.order = append(f.order, blockOff)
		if len(f.order) > maxCachedSegmentBlocks {
			evict := f.order[0]
			f.order = f.order[1:]
			delete(f.blocks, evict)
		}
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
	s.manifest = m
	if m.Generation > s.latestGen {
		s.latestGen = m.Generation
	}
	refs := m.ReferencedFiles()
	for name := range s.lookupCache {
		if _, ok := refs[name]; ok {
			continue
		}
		delete(s.lookupCache, name)
	}
	for name := range s.segmentFileCache {
		if _, ok := refs[name]; ok {
			continue
		}
		delete(s.segmentFileCache, name)
	}
}

func (s *Shard) cacheLookup(filename string, lookup *segment.LookupMeta) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lookupCache == nil {
		s.lookupCache = make(map[string]*segment.LookupMeta)
	}
	s.lookupCache[filename] = lookup
}

func (s *Shard) getLookup(ctx context.Context, meta *SegmentMeta) (*segment.LookupMeta, error) {
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/get-lookup")
	defer task.End()

	s.mu.Lock()
	lookup := s.lookupCache[meta.Filename]
	s.mu.Unlock()
	if lookup != nil {
		return lookup, nil
	}
	taskCtx, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/get-lookup/load-meta")
	f, err := s.getSegmentFile(taskCtx, meta)
	if err == nil {
		lookup, err = loadLookupMeta(taskCtx, f, meta)
	}
	subtask.End()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if existing := s.lookupCache[meta.Filename]; existing != nil {
		lookup = existing
	} else {
		s.lookupCache[meta.Filename] = lookup
	}
	s.mu.Unlock()
	return lookup, nil
}

func (s *Shard) getSegmentFile(ctx context.Context, meta *SegmentMeta) (*cachedSegmentFile, error) {
	s.mu.Lock()
	f := s.segmentFileCache[meta.Filename]
	s.mu.Unlock()
	if f != nil {
		return f, nil
	}

	_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/get-segment-file/open-file")
	af, err := opfs.OpenAsyncFile(s.dir, meta.Filename)
	subtask.End()
	if err != nil {
		return nil, err
	}
	f = newCachedSegmentFile(af, int64(meta.Size))

	s.mu.Lock()
	if existing := s.segmentFileCache[meta.Filename]; existing != nil {
		f = existing
	} else {
		s.segmentFileCache[meta.Filename] = f
	}
	s.mu.Unlock()
	return f, nil
}

func (s *Shard) dropSegmentFile(filename string) {
	s.mu.Lock()
	delete(s.segmentFileCache, filename)
	s.mu.Unlock()
}

func loadLookupMeta(ctx context.Context, f segmentReader, meta *SegmentMeta) (*segment.LookupMeta, error) {
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/load-lookup-meta")
	defer task.End()

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
	_, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/load-lookup-meta/load")
	lookup, err := segment.LoadLookupMeta(f, size)
	subtask.End()
	if err != nil {
		return nil, errors.Wrap(err, "load segment lookup metadata")
	}
	return lookup, nil
}

func lookupFromReader(rd *segment.Reader) *segment.LookupMeta {
	return &segment.LookupMeta{
		Header: rd.Header(),
		MinKey: append([]byte{}, rd.MinKey()...),
		MaxKey: append([]byte{}, rd.MaxKey()...),
		Index:  cloneIndex(rd.Index()),
		Bloom:  rd.Bloom(),
	}
}

func cloneIndex(idx []segment.IndexEntry) []segment.IndexEntry {
	out := make([]segment.IndexEntry, len(idx))
	for i := range idx {
		out[i] = segment.IndexEntry{
			Key:        append([]byte{}, idx[i].Key...),
			DataOffset: idx[i].DataOffset,
		}
	}
	return out
}

func alignSegmentOffset(off int64) int64 {
	return (off / cachedSegmentBlockSize) * cachedSegmentBlockSize
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
