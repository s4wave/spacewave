//go:build js

package blockshard

import (
	"bytes"
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
)

// Shard is a single block shard backed by an OPFS directory.
// It owns a set of immutable SSTable segment files and a double-buffered manifest.
type Shard struct {
	id         int
	dir        js.Value
	lockPrefix string

	mu       sync.Mutex
	manifest *Manifest
	seqNum   uint64 // monotonic segment filename counter
	nowFn    func() time.Time
	bloomFPR float64

	lookupCache map[string]*segment.LookupMeta
}

// NewShard opens or creates a shard in the given OPFS directory.
// It reads both manifest slots and picks the higher valid generation.
func NewShard(id int, dir js.Value, lockPrefix string, settings *Settings) (*Shard, error) {
	settings = normalizeSettings(settings)
	s := &Shard{
		id:          id,
		dir:         dir,
		lockPrefix:  lockPrefix,
		nowFn:       time.Now,
		bloomFPR:    settings.BloomFPR,
		lookupCache: make(map[string]*segment.LookupMeta),
	}

	// Read both manifest slots.
	a := readFileBytes(dir, manifestSlotA)
	b := readFileBytes(dir, manifestSlotB)
	m := PickManifest(a, b)
	if m == nil {
		m = &Manifest{Generation: 0}
	}
	s.manifest = m

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

// Publish writes a batch of key-value entries as a new SSTable segment,
// then flips the manifest to include it. Caller must hold the shard publish lock.
func (s *Shard) Publish(entries []segment.Entry) error {
	if len(entries) == 0 {
		return nil
	}

	// Build the SSTable in memory.
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
	if err != nil {
		return errors.Wrap(err, "build segment")
	}

	s.mu.Lock()
	s.seqNum++
	seq := s.seqNum
	gen := s.manifest.Generation + 1
	s.mu.Unlock()

	filename := "seg-" + zeroPad(seq, 6) + ".sst"

	// Write the segment file to OPFS using sync handle.
	segFile, err := opfs.CreateSyncFile(s.dir, filename)
	if err != nil {
		return errors.Wrap(err, "create segment file")
	}
	segData := buf.Bytes()
	if _, err := segFile.WriteAt(segData, 0); err != nil {
		segFile.Close()
		return errors.Wrap(err, "write segment")
	}
	segFile.Flush()
	if err := segFile.Close(); err != nil {
		return errors.Wrap(err, "close segment")
	}

	// Build sorted entries to get min/max keys.
	// The writer sorts them, so re-read from the built SSTable.
	rd, err := segment.NewReader(bytes.NewReader(segData), written)
	if err != nil {
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

	// Update manifest.
	s.mu.Lock()
	newManifest := &Manifest{
		Generation: gen,
		Segments:   append(append([]SegmentMeta{}, s.manifest.Segments...), meta),
	}
	s.mu.Unlock()

	if err := s.writeManifest(newManifest); err != nil {
		return err
	}
	s.cacheLookup(filename, lookup)
	return nil
}

// writeManifest writes a manifest to the alternate slot and commits in-memory.
func (s *Shard) writeManifest(m *Manifest) error {
	slot := manifestSlotA
	if m.Generation%2 == 0 {
		slot = manifestSlotB
	}
	mdata := m.Encode()
	mf, err := opfs.CreateSyncFile(s.dir, slot)
	if err != nil {
		return errors.Wrap(err, "create manifest file")
	}
	mf.Truncate(0)
	if _, err := mf.WriteAt(mdata, 0); err != nil {
		mf.Close()
		return errors.Wrap(err, "write manifest")
	}
	mf.Flush()
	if err := mf.Close(); err != nil {
		return errors.Wrap(err, "close manifest")
	}

	s.mu.Lock()
	s.setManifestLocked(m)
	s.mu.Unlock()
	return nil
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
	f, err := opfs.OpenAsyncFile(dir, name)
	if err != nil {
		return nil
	}
	size, err := f.Size()
	if err != nil || size == 0 {
		return nil
	}
	buf := make([]byte, size)
	if _, err := f.ReadAt(buf, 0); err != nil {
		return nil
	}
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
