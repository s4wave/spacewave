//go:build js

package blockshard

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"sync"
	"syscall/js"
	"time"

	trace "github.com/s4wave/spacewave/db/traceutil"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/opfs/filelock"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
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
	// asyncIO forces async OPFS writes for all shard files.
	asyncIO bool

	mu                  sync.Mutex
	manifest            *Manifest
	latestGen           uint64
	seqNum              uint64 // monotonic segment filename counter
	nowFn               func() time.Time
	bloomFPR            float64
	maxSegmentDataBytes int

	lookupCache      map[string]*segment.LookupMeta
	segmentFileCache map[string]*cachedSegmentFile
}

// NewShard opens or creates a shard in the given OPFS directory.
// It reads both manifest slots and picks the higher valid generation.
func NewShard(id int, dir js.Value, lockPrefix string, settings *Settings) (*Shard, error) {
	settings = normalizeSettings(settings)
	s := &Shard{
		id:                  id,
		dir:                 dir,
		lockPrefix:          lockPrefix,
		asyncIO:             settings.AsyncIO,
		nowFn:               time.Now,
		bloomFPR:            settings.BloomFPR,
		maxSegmentDataBytes: settings.MaxSegmentDataBytes,
		lookupCache:         make(map[string]*segment.LookupMeta),
		segmentFileCache:    make(map[string]*cachedSegmentFile),
	}

	if err := s.reloadManifestFromDisk(context.Background()); err != nil {
		return nil, err
	}
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
	if err := s.reloadManifestFromDisk(ctx); err != nil {
		return errors.Wrap(err, "reload manifest")
	}

	groups := splitSegmentEntries(entries, s.maxSegmentDataBytes)
	outputs := make([]writtenSegment, 0, len(groups))
	for i := range groups {
		output, err := s.writeSegment(ctx, groups[i], 0)
		if err != nil {
			return err
		}
		outputs = append(outputs, output)
	}

	_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/shard/publish/write-manifest")
	s.mu.Lock()
	newManifest := s.manifest.Clone()
	newManifest.Generation = s.manifest.Generation + 1
	for i := range outputs {
		newManifest.Segments = append(newManifest.Segments, outputs[i].Meta)
	}
	s.mu.Unlock()

	if err := s.writeManifest(newManifest); err != nil {
		subtask.End()
		return err
	}
	for i := range outputs {
		s.cacheLookup(outputs[i].Meta.Filename, outputs[i].Lookup)
	}
	subtask.End()
	return nil
}

func (s *Shard) writeSegment(ctx context.Context, entries []segment.Entry, level uint8) (writtenSegment, error) {
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/shard/write-segment")
	defer task.End()

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
	if estimated := w.EstimatedSize(); estimated > 0 {
		buf.Grow(estimated)
	}
	written, err := w.Build(&buf)
	subtask.End()
	if err != nil {
		return writtenSegment{}, errors.Wrap(err, "build segment")
	}

	taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/shard/publish/allocate-seqno")
	s.mu.Lock()
	s.seqNum++
	seq := s.seqNum
	s.mu.Unlock()
	subtask.End()

	filename := "seg-" + zeroPad(seq, 6) + ".sst"

	// Write the segment file to OPFS.
	segData := buf.Bytes()
	taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/shard/publish/write-segment-file")

	// Tag the publish with a few coarse buckets so trace output is easier to group.
	_, shardTask := trace.NewTask(taskCtx, publishShardTaskName(s.id))
	_, sizeTask := trace.NewTask(taskCtx, publishSegmentSizeTaskName(len(segData)))
	_, entryTask := trace.NewTask(taskCtx, publishEntryCountTaskName(len(entries)))
	if err := s.writeFileData(taskCtx, filename, segData); err != nil {
		entryTask.End()
		sizeTask.End()
		shardTask.End()
		subtask.End()
		return writtenSegment{}, errors.Wrap(err, "write segment")
	}
	entryTask.End()
	sizeTask.End()
	shardTask.End()
	subtask.End()

	// Build sorted entries to get min/max keys.
	// The writer sorts them, so re-read from the built SSTable.
	taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/shard/publish/build-metadata")
	lookup, err := segment.LoadLookupMeta(bytes.NewReader(segData), written)
	if err != nil {
		subtask.End()
		return writtenSegment{}, errors.Wrap(err, "load built segment metadata")
	}

	meta := SegmentMeta{
		Filename:   filename,
		EntryCount: lookup.Header.EntryCount,
		Size:       uint32(written),
		Level:      level,
		MinKey:     lookup.MinKey,
		MaxKey:     lookup.MaxKey,
	}
	subtask.End()

	return writtenSegment{Meta: meta, Lookup: lookup}, nil
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

	taskName := "hydra/opfs-blockshard/shard/write-file-data/select-sync"
	if s.asyncIO {
		taskName = "hydra/opfs-blockshard/shard/write-file-data/select-async/forced-config"
	} else if !opfs.PreferSyncAccessHandles() {
		taskName = "hydra/opfs-blockshard/shard/write-file-data/select-async/sync-not-preferred"
	} else if !isSegmentFilename(name) {
		taskName = "hydra/opfs-blockshard/shard/write-file-data/select-async/non-segment"
	}

	// Emit a zero-work branch marker so traces show why this write picked sync vs async.
	_, selectTask := trace.NewTask(ctx, taskName)
	selectTask.End()

	if s.shouldUseAsyncWrite(name) {
		_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/shard/write-file-data/write-async-file")
		err := opfs.WriteFile(s.dir, name, data)
		subtask.End()
		return err
	}

	_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/shard/write-file-data/create-sync-file")
	f, err := opfs.CreateSyncFileContext(ctx, s.dir, name)
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
	if !opfs.PreferSyncAccessHandles() {
		return true
	}
	return !isSegmentFilename(name)
}

func isSegmentFilename(name string) bool {
	return strings.HasSuffix(name, ".sst")
}

// AcquirePublishLock acquires the exclusive per-shard publish WebLock.
// Returns a release function.
func (s *Shard) AcquirePublishLock() (func(), error) {
	name := s.lockPrefix + "/shard-" + zeroPad(uint64(s.id), 2) + "/publish"
	return filelock.AcquireWebLock(name, true)
}

func (s *Shard) reloadManifestFromDisk(ctx context.Context) error {
	m, err := readManifestFromDisk(ctx, s.dir)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.setManifestLocked(m)
	s.seqNum = deriveSeqNum(m)
	s.mu.Unlock()
	return nil
}

// deriveSeqNum scans the manifest for the highest segment sequence number.
func deriveSeqNum(m *Manifest) uint64 {
	var max uint64
	for _, seg := range m.Segments {
		// Parse "seg-NNNNNN.sst" -> NNNNNN
		if len(seg.Filename) >= 14 {
			if n, err := strconv.ParseUint(seg.Filename[4:10], 10, 64); err == nil {
				if n > max {
					max = n
				}
			}
		}
	}
	for _, seg := range m.PendingDelete {
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

func readManifestFromDisk(ctx context.Context, dir js.Value) (*Manifest, error) {
	a, err := readFileBytesRequired(ctx, dir, manifestSlotA)
	if err != nil {
		return nil, errors.Wrap(err, "read manifest-a")
	}
	b, err := readFileBytesRequired(ctx, dir, manifestSlotB)
	if err != nil {
		return nil, errors.Wrap(err, "read manifest-b")
	}
	m := PickManifest(a, b)
	if m != nil {
		return m, nil
	}
	if len(a) != 0 || len(b) != 0 {
		return nil, errors.New("no valid shard manifest slots")
	}
	return &Manifest{Generation: 0}, nil
}

// CleanOrphans removes segment files not referenced by the current manifest.
// Called during startup to clean up after interrupted writes.
func (s *Shard) CleanOrphans() error {
	if err := s.reloadManifestFromDisk(context.Background()); err != nil {
		return errors.Wrap(err, "reload manifest")
	}
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
	if err := s.reloadManifestFromDisk(context.Background()); err != nil {
		return false, errors.Wrap(err, "reload manifest")
	}
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

	_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/read-file-bytes/read-all")
	buf, err := opfs.ReadFile(dir, name)
	subtask.End()
	if err != nil || len(buf) == 0 {
		return nil
	}
	return buf
}

func readFileBytesRequired(ctx context.Context, dir js.Value, name string) ([]byte, error) {
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/read-file-bytes-required")
	defer task.End()

	_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/read-file-bytes-required/read-all")
	buf, err := opfs.ReadFile(dir, name)
	subtask.End()
	if err != nil {
		if opfs.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(buf) == 0 {
		return nil, nil
	}
	return buf, nil
}

// zeroPad formats n as a zero-padded decimal string.
func zeroPad(n uint64, width int) string {
	s := strconv.FormatUint(n, 10)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

func publishEntryCountTaskName(n int) string {
	switch {
	case n <= 8:
		return "hydra/opfs-blockshard/shard/publish/entry-count/1-8"
	case n <= 16:
		return "hydra/opfs-blockshard/shard/publish/entry-count/9-16"
	case n <= 32:
		return "hydra/opfs-blockshard/shard/publish/entry-count/17-32"
	case n <= 64:
		return "hydra/opfs-blockshard/shard/publish/entry-count/33-64"
	default:
		return "hydra/opfs-blockshard/shard/publish/entry-count/65+"
	}
}

func publishSegmentSizeTaskName(n int) string {
	switch {
	case n <= 64*1024:
		return "hydra/opfs-blockshard/shard/publish/segment-size/0-64k"
	case n <= 256*1024:
		return "hydra/opfs-blockshard/shard/publish/segment-size/64k-256k"
	case n <= 1024*1024:
		return "hydra/opfs-blockshard/shard/publish/segment-size/256k-1m"
	default:
		return "hydra/opfs-blockshard/shard/publish/segment-size/1m+"
	}
}

func publishShardTaskName(id int) string {
	return "hydra/opfs-blockshard/shard/publish/shard/" + strconv.Itoa(id)
}
