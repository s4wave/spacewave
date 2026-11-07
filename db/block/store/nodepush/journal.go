package block_store_nodepush

import (
	"bufio"
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// journalFile is the name of the journal's live file in its directory.
const journalFile = "journal.log"

// journalCompactBytes is the size past which a trim rewrites the live file
// with only the records still live, which trims keep to a few seconds' worth.
const journalCompactBytes = 4 << 20

// castagnoli checksums each journal frame.
var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// ErrEpochCut refuses a record of a run the journal already cut.
var ErrEpochCut = errors.New("journal epoch already cut")

// Journal keeps the records of a local store's commits on the node's disk
// until the local store holds them durably itself. It is the local store's
// write-ahead log: a record is durable once Append returns, and a local store
// that crashed replays the records past its own durable head from Tail.
//
// The live file is a sequence of frames, each a uvarint length, an encoded
// Record and its CRC32C. Open keeps the leading frames whose checksums match
// and truncates at the first that does not, which only a write a crash cut
// short leaves, and no Append of it returned. A record with seq zero and no
// writes marks its epoch cut, so a record of a run the local store replaced
// is refused after a restart too.
//
// Invariants: a record is dropped only once a trim says the local store holds
// it durably, or a cut sets it aside in a quarantine file, which is never
// deleted; and Append returns nil only once its frame is synced.
type Journal struct {
	// dir holds the live file and the quarantine files.
	dir string

	// mtx guards the fields below and serializes writes to file.
	mtx sync.Mutex
	// file is the live file, open for appending.
	file *os.File
	// size is the live file's length.
	size int64
	// records holds the live records of each epoch by seq.
	records map[uint64]map[uint64]*Record
	// cut lists the epochs cut, kept in the live file as markers.
	cut map[uint64]bool

	// syncMtx serializes fsyncs; one sync covers every frame written before
	// it began.
	syncMtx sync.Mutex
	// synced is the live file length the last sync covered, guarded by mtx.
	synced int64
}

// OpenJournal opens the journal in dir, keeping the records a previous
// process left.
func OpenJournal(dir string) (*Journal, error) {
	// Open the directory and the live file.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, journalFile)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) // #nosec G304 -- the path is under the journal's own directory
	if err != nil {
		return nil, err
	}
	j := &Journal{dir: dir, file: f, records: make(map[uint64]map[uint64]*Record), cut: make(map[uint64]bool)}

	// Load the valid frames, and cut off a torn tail.
	end, err := j.load(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Truncate(end); err != nil {
		_ = f.Close()
		return nil, err
	}
	if _, err := f.Seek(end, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, err
	}

	// Append from the end, the directory entry durable first.
	j.size, j.synced = end, end
	if err := syncDir(dir); err != nil {
		_ = f.Close()
		return nil, err
	}
	return j, nil
}

// load reads f's frames into j and returns the length of its valid prefix.
func (j *Journal) load(f *os.File) (int64, error) {
	// Apply frames until the first that does not read whole.
	r := bufio.NewReader(f)
	var end int64
	for {
		rec, n, err := readFrame(r)
		if err != nil {
			return end, nil
		}
		end += n
		j.apply(rec)
	}
}

// apply adds rec to the in-memory state. The caller holds mtx or owns j.
func (j *Journal) apply(rec *Record) {
	// A marker cuts its epoch.
	if rec.GetSeq() == 0 {
		j.cut[rec.GetEpoch()] = true
		delete(j.records, rec.GetEpoch())
		return
	}

	// A trim is a record's absence: frames past a compaction hold only live
	// records, and a trimmed record that reappears from an older frame is
	// dropped again by the next trim.
	if j.cut[rec.GetEpoch()] {
		return
	}
	byEpoch := j.records[rec.GetEpoch()]
	if byEpoch == nil {
		byEpoch = make(map[uint64]*Record)
		j.records[rec.GetEpoch()] = byEpoch
	}
	byEpoch[rec.GetSeq()] = rec
}

// Append makes rec durable. It refuses a record of a cut epoch.
func (j *Journal) Append(rec *Record) error {
	// Encode the frame.
	frame, err := encodeFrame(rec)
	if err != nil {
		return err
	}

	// Write it and add the record.
	end, err := j.write(rec, frame)
	if err != nil {
		return err
	}

	// Sync it, sharing the sync with every frame written meanwhile.
	return j.syncTo(end)
}

// write appends frame, rec's, to the live file and adds rec, returning the
// file's length after it.
func (j *Journal) write(rec *Record, frame []byte) (int64, error) {
	// A cut run takes no more records.
	j.mtx.Lock()
	defer j.mtx.Unlock()
	if j.cut[rec.GetEpoch()] {
		return 0, ErrEpochCut
	}

	// Write the frame, then add the record.
	if _, err := j.file.Write(frame); err != nil {
		return 0, err
	}
	j.size += int64(len(frame))
	j.apply(rec)
	return j.size, nil
}

// syncTo returns once the live file is synced through end.
func (j *Journal) syncTo(end int64) error {
	// One sync at a time; a sync that began after end was written covers it.
	j.syncMtx.Lock()
	defer j.syncMtx.Unlock()
	f, target, done := j.syncTarget(end)
	if done {
		return nil
	}

	// Sync, and record what it covered unless a rewrite replaced the file.
	if err := f.Sync(); err != nil {
		return err
	}
	j.mtx.Lock()
	defer j.mtx.Unlock()
	if j.file == f {
		j.synced = max(j.synced, target)
	}
	return nil
}

// syncTarget returns the live file and its length to sync, or done when a
// sync already covered end.
func (j *Journal) syncTarget(end int64) (*os.File, int64, bool) {
	j.mtx.Lock()
	defer j.mtx.Unlock()
	return j.file, j.size, j.synced >= end
}

// Tail returns the consecutive records of epoch past after, up to maxBytes of
// encoded records, and whether more follow.
func (j *Journal) Tail(epoch, after uint64, maxBytes int) ([]*Record, bool) {
	// Walk the seqs from after until one is missing or the frame is full.
	j.mtx.Lock()
	defer j.mtx.Unlock()
	byEpoch := j.records[epoch]
	var out []*Record
	size := 0
	for seq := after + 1; ; seq++ {
		rec := byEpoch[seq]
		if rec == nil {
			return out, false
		}
		size += rec.SizeVT()
		if len(out) != 0 && size > maxBytes {
			return out, true
		}
		out = append(out, rec)
	}
}

// Trim drops the records of epoch up to through, which the local store holds
// durably, and rewrites the live file once it has grown past
// journalCompactBytes. A trim is not written down: after a restart the trimmed
// records come back until the next trim, and a local store never asks for
// records at or below its own durable head.
func (j *Journal) Trim(epoch, through uint64) error {
	// Drop the records.
	j.mtx.Lock()
	defer j.mtx.Unlock()
	for seq := range j.records[epoch] {
		if seq <= through {
			delete(j.records[epoch], seq)
		}
	}
	if len(j.records[epoch]) == 0 {
		delete(j.records, epoch)
	}

	// Compact a large file.
	if j.size < journalCompactBytes {
		return nil
	}
	return j.rewriteLocked()
}

// Cut sets aside every live record except those of epoch up to through,
// which the local store replayed and holds durably, and marks epoch cut. The
// set-aside records go to a quarantine file first, so a cut never deletes a
// record an operator may need.
func (j *Journal) Cut(epoch, through uint64) error {
	// Cut under the lock, so no record lands meanwhile.
	j.mtx.Lock()
	defer j.mtx.Unlock()

	// Quarantine what the local store did not replay.
	if aside := j.asideLocked(epoch, through); len(aside) != 0 {
		name := "quarantine-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".log"
		if err := writeRecords(filepath.Join(j.dir, name), aside); err != nil {
			return err
		}
	}

	// Start the live file afresh with the epoch's marker.
	j.cut[epoch] = true
	j.records = make(map[uint64]map[uint64]*Record)
	return j.rewriteLocked()
}

// asideLocked returns the live records a cut of epoch through through sets
// aside, in order. The caller holds mtx.
func (j *Journal) asideLocked(epoch, through uint64) []*Record {
	// Collect them.
	var aside []*Record
	for e, byEpoch := range j.records {
		for seq, rec := range byEpoch {
			if e != epoch || seq > through {
				aside = append(aside, rec)
			}
		}
	}

	// Order them by run, then seq.
	slices.SortFunc(aside, func(a, b *Record) int {
		if a.GetEpoch() != b.GetEpoch() {
			return compareUint(a.GetEpoch(), b.GetEpoch())
		}
		return compareUint(a.GetSeq(), b.GetSeq())
	})
	return aside
}

// rewriteLocked replaces the live file with the cut markers and the live
// records, durably. A frame an Append wrote to the old file is in the new one,
// since its record is live. The caller holds mtx.
func (j *Journal) rewriteLocked() error {
	// Gather what the new file holds.
	var keep []*Record
	for e := range j.cut {
		keep = append(keep, &Record{Epoch: e})
	}
	for _, byEpoch := range j.records {
		for _, rec := range byEpoch {
			keep = append(keep, rec)
		}
	}

	// Write it beside the live file, then rename it over.
	path := filepath.Join(j.dir, journalFile)
	if err := writeRecords(path+".tmp", keep); err != nil {
		return err
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		return err
	}
	if err := syncDir(j.dir); err != nil {
		return err
	}

	// Append to the new file from here.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND, 0o600) // #nosec G304 -- the path is under the journal's own directory
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	_ = j.file.Close()
	j.file, j.size, j.synced = f, info.Size(), info.Size()
	return nil
}

// Close closes the live file.
func (j *Journal) Close() error {
	// Close under the lock, so no write races it.
	j.mtx.Lock()
	defer j.mtx.Unlock()
	return j.file.Close()
}

// encodeFrame returns rec as one frame.
func encodeFrame(rec *Record) ([]byte, error) {
	// Frame the body with its length and checksum.
	body, err := rec.MarshalVT()
	if err != nil {
		return nil, err
	}
	frame := binary.AppendUvarint(nil, uint64(len(body)))
	frame = append(frame, body...)
	return binary.LittleEndian.AppendUint32(frame, crc32.Checksum(body, castagnoli)), nil
}

// readFrame reads one frame and returns its record and length. Any error,
// including a checksum mismatch, ends the valid prefix.
func readFrame(r *bufio.Reader) (*Record, int64, error) {
	// Read the length, then the body and checksum.
	size, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, 0, err
	}
	if size > maxMessageBytes {
		return nil, 0, errors.New("journal frame too large")
	}
	buf := make([]byte, size+4)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, 0, err
	}

	// Check and decode the body.
	body := buf[:size]
	if binary.LittleEndian.Uint32(buf[size:]) != crc32.Checksum(body, castagnoli) {
		return nil, 0, errors.New("journal frame checksum mismatch")
	}
	rec := &Record{}
	if err := rec.UnmarshalVT(body); err != nil {
		return nil, 0, err
	}
	return rec, int64(len(binary.AppendUvarint(nil, size))) + int64(len(buf)), nil
}

// writeRecords writes records to a new file at path and syncs it.
func writeRecords(path string, records []*Record) error {
	// Encode the frames.
	var data []byte
	for _, rec := range records {
		frame, err := encodeFrame(rec)
		if err != nil {
			return err
		}
		data = append(data, frame...)
	}

	// Write and sync them.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) // #nosec G304 -- the path is under the journal's own directory
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// syncDir flushes dir's entries.
func syncDir(dir string) error {
	d, err := os.Open(dir) // #nosec G304 -- the journal's own directory
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// compareUint orders a before b.
func compareUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
