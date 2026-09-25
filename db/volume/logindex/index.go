// Package logindex is an ordered key-value index on a storage Device that
// keeps the whole table in memory and writes it as a log.
//
// A commit flushes the device's earlier writes and then appends one record to
// the current log file with a flush. An ordered commit appends its record
// without either flush. Recovery replays records only while their sequence
// continues, so a crash loses at most a suffix of the ordered commits. A
// checkpoint writes the table to a checkpoint file and then a manifest naming
// that checkpoint and the first log file written after it, and removes the
// files the manifest no longer names. Recovery loads the manifest's
// checkpoint and replays the later log records in sequence order.
//
// Published tables are copy-on-write snapshots that are never changed, so a
// read transaction sees one snapshot for its whole life without locks.
package logindex

import (
	"bytes"
	"context"
	"hash/crc32"
	"slices"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume/device"
	"github.com/tidwall/btree"
)

// defaultCheckpointBytes is the log length that starts a checkpoint by
// default.
const defaultCheckpointBytes = 4 << 20

// checkpointChunk is the length of each checkpoint write.
const checkpointChunk = 1 << 20

// Options configures an Index.
type Options struct {
	// CheckpointBytes is the log length that starts a checkpoint once the log
	// is also longer than half the last checkpoint. Zero selects 4 MiB.
	CheckpointBytes int64
	// Foreground runs each checkpoint inside the commit that starts it and
	// returns its error from that commit, which fixes the device call order.
	Foreground bool
}

// entry is one key and its value.
type entry struct {
	// key is the key.
	key []byte
	// value is the value.
	value []byte
}

// lessEntry orders entries by key.
func lessEntry(a, b entry) bool {
	return bytes.Compare(a.key, b.key) < 0
}

// table is the ordered set of entries.
type table = btree.BTreeG[entry]

// newTable returns an empty table. Tables skip the btree's lock: a published
// table is only read, and a write transaction owns its copy.
func newTable() *table {
	return btree.NewBTreeGOptions(lessEntry, btree.Options{NoLocks: true})
}

// Index is the log-structured index.
type Index struct {
	// ctx bounds background checkpoints.
	ctx context.Context
	// dev holds the index files.
	dev device.Device
	// opts holds the options with defaults applied.
	opts Options
	// wg tracks the running checkpoint.
	wg sync.WaitGroup

	// wmtx is held by the open write transaction.
	wmtx sync.Mutex

	// mtx guards the fields below.
	mtx sync.Mutex
	// tree is the published table.
	tree *table
	// seq is the sequence of the last committed record.
	seq uint64
	// log is the number of the log file taking records.
	log uint64
	// logSize is the length of the log file taking records.
	logSize int64
	// err fails every later commit once a log write fails, since the log may
	// end in a torn record.
	err error
	// man is the last written manifest.
	man manifest
	// checkpointing is set while a checkpoint runs.
	checkpointing bool
	// checkpointErr is the error of the last failed background checkpoint.
	checkpointErr error
}

// Open recovers the index from dev, creating it when dev has none. ctx
// bounds the index's background checkpoints until Close.
func Open(ctx context.Context, dev device.Device, opts Options) (*Index, error) {
	if opts.CheckpointBytes <= 0 {
		opts.CheckpointBytes = defaultCheckpointBytes
	}
	i := &Index{ctx: ctx, dev: dev, opts: opts, tree: newTable()}

	// Find the manifest and the files.
	files, err := dev.List(ctx)
	if err != nil {
		return nil, err
	}
	sizes := make(map[string]int64, len(files))
	for _, f := range files {
		sizes[f.Name] = f.Size
	}
	i.man, err = readManifest(ctx, dev, sizes[manifestName])
	if err != nil {
		return nil, err
	}

	// Load the checkpoint and replay the later logs in order.
	i.seq = i.man.seq
	if i.man.checkpoint != 0 {
		if err := i.loadCheckpoint(ctx); err != nil {
			return nil, err
		}
	}
	var logs []uint64
	for _, f := range files {
		if n, ok := parseFile(logPrefix, f.Name); ok && n >= i.man.log {
			logs = append(logs, n)
		}
	}
	slices.Sort(logs)
	for _, n := range logs {
		if err := i.replayLog(ctx, n, sizes[fileName(logPrefix, n)]); err != nil {
			return nil, err
		}
	}

	// Take records in a new log file, since an existing one may end torn.
	i.log = i.man.log
	if len(logs) != 0 {
		i.log = logs[len(logs)-1] + 1
	}

	// Remove the files no manifest names.
	var stale []string
	for _, f := range files {
		if n, ok := parseFile(checkpointPrefix, f.Name); ok && n != i.man.checkpoint {
			stale = append(stale, f.Name)
		}
		if n, ok := parseFile(logPrefix, f.Name); ok && n < i.man.log {
			stale = append(stale, f.Name)
		}
	}
	if len(stale) != 0 {
		if err := dev.Remove(ctx, stale); err != nil {
			return nil, err
		}
	}
	return i, nil
}

// readManifest returns the valid manifest slot with the highest generation,
// or the empty index's manifest when there is none.
func readManifest(ctx context.Context, dev device.Device, size int64) (manifest, error) {
	best := manifest{log: 1}
	for slot := range int64(2) {
		off := slot * slotSpacing
		if size < off+slotSize {
			continue
		}
		b := make([]byte, slotSize)
		if err := dev.Read(ctx, []device.Read{{Name: manifestName, Offset: off, Data: b}}); err != nil {
			return manifest{}, errors.Wrap(err, "read manifest")
		}
		if m, ok := unmarshalManifest(b); ok && m.gen > best.gen {
			best = m
		}
	}
	return best, nil
}

// loadCheckpoint loads the manifest's checkpoint into the table. The entries
// alias the file buffer.
func (i *Index) loadCheckpoint(ctx context.Context) error {
	b := make([]byte, i.man.checkpointLen)
	name := fileName(checkpointPrefix, i.man.checkpoint)
	if err := i.dev.Read(ctx, []device.Read{{Name: name, Offset: 0, Data: b}}); err != nil {
		return errors.Wrap(err, "read checkpoint")
	}
	if crc32.Checksum(b, castagnoli) != i.man.checkpointSum {
		return errors.New("checkpoint checksum mismatch")
	}
	for len(b) != 0 {
		key, rest, err := readBytes(b)
		if err != nil {
			return err
		}
		value, rest, err := readBytes(rest)
		if err != nil {
			return err
		}
		i.tree.Load(entry{key: key, value: value})
		b = rest
	}
	return nil
}

// replayLog applies the records of log file n that continue the sequence,
// stopping at the first torn or absent record.
func (i *Index) replayLog(ctx context.Context, n uint64, size int64) error {
	b := make([]byte, size)
	if err := i.dev.Read(ctx, []device.Read{{Name: fileName(logPrefix, n), Offset: 0, Data: b}}); err != nil {
		return errors.Wrap(err, "read log")
	}
	for {
		seq, ops, l, ok := readRecord(b)
		if !ok || seq != i.seq+1 {
			return nil
		}
		apply(i.tree, ops)
		i.seq = seq
		b = b[l:]
	}
}

// apply applies ops to t.
func apply(t *table, ops []logOp) {
	for _, op := range ops {
		if op.del {
			t.Delete(entry{key: op.key})
			continue
		}
		t.Set(entry{key: op.key, value: op.value})
	}
}

// NewTransaction opens a transaction on the published table. A write
// transaction holds the index's writer lock until Commit or Discard.
func (i *Index) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if !write {
		i.mtx.Lock()
		tree := i.tree
		i.mtx.Unlock()
		return &Tx{tree: tree}, nil
	}
	i.wmtx.Lock()
	i.mtx.Lock()
	tree := i.tree.Copy()
	i.mtx.Unlock()
	return &Tx{index: i, tree: tree}, nil
}

// commit appends ops as one log record, publishes tree, and starts a
// checkpoint when the log is long enough. With flush set, a flush barrier
// precedes the record and the record is flushed. The caller holds wmtx.
func (i *Index) commit(ctx context.Context, tree *table, ops []logOp, flush bool) error {
	i.mtx.Lock()
	seq, log, off, err := i.seq+1, i.log, i.logSize, i.err
	i.mtx.Unlock()
	if err != nil {
		return err
	}

	// Flush the earlier writes on the device, which the record may reference,
	// so a crash during the record's own flush never keeps the record without
	// them. The barrier costs nothing on a device with no unflushed writes.
	if flush {
		if err := i.dev.Write(ctx, nil, true); err != nil {
			return errors.Wrap(err, "flush before log")
		}
	}

	// Write the record.
	rec := appendRecord(nil, seq, ops)
	w := device.Write{Name: fileName(logPrefix, log), Offset: off, Data: rec}
	if err := i.dev.Write(ctx, []device.Write{w}, flush); err != nil {
		err = errors.Wrap(err, "write log")
		i.mtx.Lock()
		i.err = err
		i.mtx.Unlock()
		return err
	}

	// Publish the table and decide on a checkpoint.
	i.mtx.Lock()
	i.tree, i.seq = tree, seq
	i.logSize += int64(len(rec))
	start := !i.checkpointing && i.logSize > max(i.opts.CheckpointBytes, int64(i.man.checkpointLen/2)) //nolint:gosec
	if start {
		// Later records go to a new log, which the checkpoint's manifest
		// names as the first to replay.
		i.checkpointing = true
		i.log++
		i.logSize = 0
	}
	next := i.log
	i.mtx.Unlock()
	if !start {
		return nil
	}

	// Checkpoint the published table.
	if i.opts.Foreground {
		return i.checkpoint(ctx, tree, seq, next)
	}
	i.wg.Go(func() {
		err := i.checkpoint(i.ctx, tree, seq, next)
		if err != nil {
			i.mtx.Lock()
			i.checkpointErr = err
			i.mtx.Unlock()
		}
	})
	return nil
}

// checkpoint writes tree, which holds every record through seq, as checkpoint
// number log, then a manifest naming it with log as the first log to replay,
// and removes the files that manifest replaces.
func (i *Index) checkpoint(ctx context.Context, tree *table, seq, log uint64) error {
	defer func() {
		i.mtx.Lock()
		i.checkpointing = false
		i.mtx.Unlock()
	}()

	// Write the checkpoint in chunks, flushing the last.
	name := fileName(checkpointPrefix, log)
	sum := crc32.New(castagnoli)
	var off int64
	var buf []byte
	flushChunk := func(flush bool) error {
		_, _ = sum.Write(buf)
		w := device.Write{Name: name, Offset: off, Data: buf}
		if err := i.dev.Write(ctx, []device.Write{w}, flush); err != nil {
			return errors.Wrap(err, "write checkpoint")
		}
		off += int64(len(buf))
		buf = buf[:0]
		return nil
	}
	var err error
	tree.Scan(func(e entry) bool {
		buf = appendBytes(buf, e.key)
		buf = appendBytes(buf, e.value)
		if len(buf) >= checkpointChunk {
			err = flushChunk(false)
		}
		return err == nil
	})
	if err != nil {
		return err
	}
	if err := flushChunk(true); err != nil {
		return err
	}

	// Write the manifest into the slot the previous one does not hold.
	i.mtx.Lock()
	prev := i.man
	i.mtx.Unlock()
	m := manifest{
		gen:           prev.gen + 1,
		checkpoint:    log,
		checkpointLen: uint64(off), //nolint:gosec
		checkpointSum: sum.Sum32(),
		seq:           seq,
		log:           log,
	}
	slot := int64(m.gen%2) * slotSpacing //nolint:gosec
	w := device.Write{Name: manifestName, Offset: slot, Data: m.marshal()}
	if err := i.dev.Write(ctx, []device.Write{w}, true); err != nil {
		return errors.Wrap(err, "write manifest")
	}
	i.mtx.Lock()
	i.man = m
	i.mtx.Unlock()

	// Remove the replaced checkpoint and logs.
	var stale []string
	if prev.checkpoint != 0 {
		stale = append(stale, fileName(checkpointPrefix, prev.checkpoint))
	}
	for n := prev.log; n < log; n++ {
		stale = append(stale, fileName(logPrefix, n))
	}
	if err := i.dev.Remove(ctx, stale); err != nil {
		return errors.Wrap(err, "remove replaced files")
	}
	return nil
}

// _ is a type assertion
var _ kvtx.Store = (*Index)(nil)

// Close waits for a running checkpoint and returns the error of the last
// failed background checkpoint.
func (i *Index) Close() error {
	i.wg.Wait()
	i.mtx.Lock()
	defer i.mtx.Unlock()
	return i.checkpointErr
}
