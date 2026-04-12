//go:build js

package blockshard

import (
	"context"
	"runtime/trace"
	"syscall/js"

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
	lookup, err := loadLookupMeta(taskCtx, s.dir, meta)
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

func loadLookupMeta(ctx context.Context, dir js.Value, meta *SegmentMeta) (*segment.LookupMeta, error) {
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/load-lookup-meta")
	defer task.End()

	_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/load-lookup-meta/open-segment")
	f, err := opfs.OpenAsyncFile(dir, meta.Filename)
	subtask.End()
	if err != nil {
		return nil, errors.Wrap(err, "open segment file")
	}
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
