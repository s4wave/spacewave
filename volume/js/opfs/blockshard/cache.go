//go:build js

package blockshard

import (
	"context"
	"runtime/trace"

	"github.com/aperturerobotics/hydra/opfs"
	"github.com/aperturerobotics/hydra/volume/js/opfs/segment"
	"github.com/pkg/errors"
)

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

func (s *Shard) getSegmentFile(ctx context.Context, meta *SegmentMeta) (*opfs.AsyncFile, error) {
	s.mu.Lock()
	f := s.segmentFileCache[meta.Filename]
	s.mu.Unlock()
	if f != nil {
		return f, nil
	}

	_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/get-segment-file/open-file")
	f, err := opfs.OpenAsyncFile(s.dir, meta.Filename)
	subtask.End()
	if err != nil {
		return nil, err
	}

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

func loadLookupMeta(ctx context.Context, f *opfs.AsyncFile, meta *SegmentMeta) (*segment.LookupMeta, error) {
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
