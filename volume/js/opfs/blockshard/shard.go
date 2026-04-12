//go:build js

package blockshard

import (
	"bytes"
	"context"
	"encoding/binary"
	"runtime/trace"
	"strconv"
	"sync"
	"syscall/js"
	"time"

	"github.com/aperturerobotics/hydra/opfs"
	"github.com/aperturerobotics/hydra/opfs/filelock"
	"github.com/aperturerobotics/hydra/volume/js/opfs/segment"
	"github.com/pkg/errors"
)

const (
	manifestSlotA = "manifest-a"
	manifestSlotB = "manifest-b"
	manifestGen   = "manifest-gen"
)

// Shard is a single block shard backed by an OPFS directory.
// It owns a set of immutable SSTable segment files and a double-buffered manifest.
type Shard struct {
	id         int
	dir        js.Value
	lockPrefix string
	// asyncIO forces async OPFS writes for all shard files.
	asyncIO bool

	mu        sync.Mutex
	manifest  *Manifest
	latestGen uint64
	seqNum    uint64 // monotonic segment filename counter
	nowFn     func() time.Time
	bloomFPR  float64

	lookupCache      map[string]*segment.LookupMeta
	segmentFileCache map[string]*opfs.AsyncFile
}

// NewShard opens or creates a shard in the given OPFS directory.
// It reads both manifest slots and picks the higher valid generation.
func NewShard(id int, dir js.Value, lockPrefix string, settings *Settings) (*Shard, error) {
	settings = normalizeSettings(settings)
	s := &Shard{
		id:               id,
		dir:              dir,
		lockPrefix:       lockPrefix,
		asyncIO:          settings.AsyncIO,
		nowFn:            time.Now,
		bloomFPR:         settings.BloomFPR,
		lookupCache:      make(map[string]*segment.LookupMeta),
		segmentFileCache: make(map[string]*opfs.AsyncFile),
	}

	// Read both manifest slots.
	a := readFileBytes(dir, manifestSlotA)
	b := readFileBytes(dir, manifestSlotB)
	m := PickManifest(a, b)
	if m == nil {
		m = &Manifest{Generation: 0}
	}
	s.manifest = m
	s.latestGen = m.Generation

	// Derive the next segment sequence number from existing segments.
	s.seqNum = s.deriveSeqNum()

	return s, nil
}

// ID returns the shard index.
func (s *Shard) ID() int {
	return s.id
}

// Manifest returns a snapshot of the current manifest.
func (s *Shard) Manifest() *Manifest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.manifest.Clone()
}

func (s *Shard) getLatestGeneration() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latestGen
}

func (s *Shard) observeGeneration(gen uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if gen > s.latestGen {
		s.latestGen = gen
	}
}

// Publish writes a batch of key-value entries as a new SSTable segment,
// then flips the manifest to include it. Caller must hold the shard publish lock.
func (s *Shard) Publish(ctx context.Context, entries []segment.Entry) error {
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/shard/publish")
	defer task.End()

	if len(entries) == 0 {
		return nil
	}

	// Build the SSTable in memory.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/shard/publish/build-segment")
	w := segment.NewWriter()
	w.SetBloomFPR(s.bloomFPR)
	for i := range entries {
		if entries[i].Tombstone {
			w.AddTombstone(entries[i].Key)
		} else {
			w.Add(entries[i].Key, entries[i].Value)
		}
	}

	var buf bytes.Buffer
	written, err := w.Build(&buf)
	subtask.End()
	if err != nil {
		return errors.Wrap(err, "build segment")
	}

	taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/shard/publish/allocate-seqno")
	s.mu.Lock()
	s.seqNum++
	seq := s.seqNum
	gen := s.manifest.Generation + 1
	s.mu.Unlock()
	subtask.End()

	filename := "seg-" + zeroPad(seq, 6) + ".sst"

	// Write the segment file to OPFS.
	segData := buf.Bytes()
	taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/shard/publish/write-segment-file")
	if err := s.writeFileData(taskCtx, filename, segData); err != nil {
		subtask.End()
		return errors.Wrap(err, "write segment")
	}
	subtask.End()

	// Build sorted entries to get min/max keys.
	// The writer sorts them, so re-read from the built SSTable.
	taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/shard/publish/build-metadata")
	rd, err := segment.NewReader(bytes.NewReader(segData), written)
	if err != nil {
		subtask.End()
		return errors.Wrap(err, "read built segment for metadata")
	}

	meta := SegmentMeta{
		Filename:   filename,
		EntryCount: rd.EntryCount(),
		Size:       uint32(written),
		Level:      0,
		MinKey:     rd.MinKey(),
		MaxKey:     rd.MaxKey(),
	}
	lookup := lookupFromReader(rd)
	subtask.End()

	// Update manifest.
	taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/shard/publish/write-manifest")
	s.mu.Lock()
	newManifest := &Manifest{
		Generation: gen,
		Segments:   append(append([]SegmentMeta{}, s.manifest.Segments...), meta),
	}
	s.mu.Unlock()

	if err := s.writeManifest(newManifest); err != nil {
		subtask.End()
		return err
	}
	s.cacheLookup(filename, lookup)
	subtask.End()
	return nil
}

// writeManifest writes a manifest to the alternate slot and commits in-memory.
func (s *Shard) writeManifest(m *Manifest) error {
	slot := manifestSlotA
	if m.Generation%2 == 0 {
		slot = manifestSlotB
	}
	mdata := m.Encode()
	if err := s.writeFileData(context.Background(), slot, mdata); err != nil {
		return errors.Wrap(err, "write manifest")
	}
	if err := s.writeFileData(context.Background(), manifestGen, encodeManifestGeneration(m.Generation)); err != nil {
		return errors.Wrap(err, "write manifest generation")
	}

	s.mu.Lock()
	s.setManifestLocked(m)
	s.mu.Unlock()
	return nil
}

// writeFileData writes data to a file in the shard directory.
// By default, immutable segment files use sync access handles when available
// while manifest writes stay async. asyncIO forces the all-async behavior.
func (s *Shard) writeFileData(ctx context.Context, name string, data []byte) error {
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/shard/write-file-data")
	defer task.End()

	if s.shouldUseAsyncWrite(name) {
		_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/shard/write-file-data/create-async-file")
		f, err := opfs.CreateAsyncFile(s.dir, name)
		subtask.End()
		if err != nil {
			return err
		}
		_, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/shard/write-file-data/write-async")
		_, err = f.WriteAtContext(ctx, data, 0)
		subtask.End()
		return err
	}
	_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/shard/write-file-data/create-sync-file")
	f, err := opfs.CreateSyncFile(s.dir, name)
	subtask.End()
	if err != nil {
		return err
	}
	_, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/shard/write-file-data/truncate")
	f.Truncate(int64(len(data)))
	subtask.End()
	_, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/shard/write-file-data/write-sync")
	if _, err := f.WriteAt(data, 0); err != nil {
		subtask.End()
		f.Close()
		return err
	}
	subtask.End()
	_, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/shard/write-file-data/flush-sync")
	f.Flush()
	subtask.End()
	_, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/shard/write-file-data/close-sync")
	err = f.Close()
	subtask.End()
	return err
}

func (s *Shard) shouldUseAsyncWrite(name string) bool {
	if s.asyncIO {
		return true
	}
	if !opfs.SyncAvailable() {
		return true
	}
	return !isSegmentFilename(name)
}

func isSegmentFilename(name string) bool {
	if len(name) < 4 || name[len(name)-4:] != ".sst" {
		return false
	}
	return true
}

// AcquirePublishLock acquires the exclusive per-shard publish WebLock.
// Returns a release function.
func (s *Shard) AcquirePublishLock() (func(), error) {
	name := s.lockPrefix + "/shard-" + zeroPad(uint64(s.id), 2) + "/publish"
	return filelock.AcquireWebLock(name, true)
}

// deriveSeqNum scans the manifest for the highest segment sequence number.
func (s *Shard) deriveSeqNum() uint64 {
	var max uint64
	for _, seg := range s.manifest.Segments {
		// Parse "seg-NNNNNN.sst" -> NNNNNN
		if len(seg.Filename) >= 14 {
			if n, err := strconv.ParseUint(seg.Filename[4:10], 10, 64); err == nil {
				if n > max {
					max = n
				}
			}
		}
	}
	for _, seg := range s.manifest.PendingDelete {
		if len(seg.Filename) >= 14 {
			if n, err := strconv.ParseUint(seg.Filename[4:10], 10, 64); err == nil {
				if n > max {
					max = n
				}
			}
		}
	}
	return max
}

// CleanOrphans removes segment files not referenced by the current manifest.
// Called during startup to clean up after interrupted writes.
func (s *Shard) CleanOrphans() error {
	entries, err := opfs.ListDirectory(s.dir)
	if err != nil {
		return errors.Wrap(err, "list shard directory")
	}

	// Build set of referenced segment filenames.
	s.mu.Lock()
	refs := s.manifest.ReferencedFiles()
	s.mu.Unlock()

	// Delete .sst files not in the manifest.
	for _, name := range entries {
		if len(name) < 4 || name[len(name)-4:] != ".sst" {
			continue
		}
		if _, ok := refs[name]; ok {
			continue
		}
		opfs.DeleteFile(s.dir, name)
	}
	return nil
}

// ReclaimPendingDelete removes manifest-retired segment files once both the
// generation gate and grace-period gate say they are safe to reclaim. Caller
// must hold the shard publish lock.
func (s *Shard) ReclaimPendingDelete() (bool, error) {
	s.mu.Lock()
	current := s.manifest.Clone()
	nowUnixMilli := uint64(s.nowFn().UnixMilli())
	keep, reclaim := selectReclaimablePending(current, nowUnixMilli)
	if len(reclaim) == 0 {
		s.mu.Unlock()
		return false, nil
	}
	next := buildReclaimManifest(current, keep)
	s.mu.Unlock()

	if err := s.writeManifest(next); err != nil {
		return false, errors.Wrap(err, "write reclaim manifest")
	}

	for _, seg := range reclaim {
		err := opfs.DeleteFile(s.dir, seg.Filename)
		if err == nil || opfs.IsNotFound(err) {
			continue
		}
		// Best-effort: the manifest no longer references this file, so a failed
		// delete leaves an orphan to be cleaned up later rather than reopening
		// stale-reader risk.
	}
	return true, nil
}

// readFileBytes reads the full contents of an OPFS file, returning nil on error.
func readFileBytes(dir js.Value, name string) []byte {
	return readFileBytesContext(context.Background(), dir, name)
}

func readFileBytesContext(ctx context.Context, dir js.Value, name string) []byte {
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/read-file-bytes")
	defer task.End()

	if name == manifestGen && opfs.SyncAvailable() {
		_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/read-file-bytes/read-sync")
		buf, err := readSyncFileBytes(dir, name)
		subtask.End()
		if err == nil || !opfs.IsNoModificationAllowed(err) {
			return buf
		}
	}

	_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/read-file-bytes/read-all")
	buf, err := opfs.ReadFile(dir, name)
	subtask.End()
	if err != nil || len(buf) == 0 {
		return nil
	}
	return buf
}

func readSyncFileBytes(dir js.Value, name string) ([]byte, error) {
	f, err := opfs.OpenSyncFile(dir, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	size := f.Size()
	if size == 0 {
		return nil, nil
	}
	buf := make([]byte, size)
	if _, err := f.ReadAt(buf, 0); err != nil {
		return nil, err
	}
	return buf, nil
}

func decodeManifestGeneration(buf []byte) (uint64, bool) {
	if len(buf) != 8 {
		return 0, false
	}
	return binary.BigEndian.Uint64(buf), true
}

func encodeManifestGeneration(gen uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, gen)
	return buf
}

// zeroPad formats n as a zero-padded decimal string.
func zeroPad(n uint64, width int) string {
	s := strconv.FormatUint(n, 10)
	for len(s) < width {
		s = "0" + s
	}
	return s
}
