// Package extsort sorts key and value records larger than memory.
//
// A Sorter buffers records up to a byte budget, writes each full buffer to a
// file in its directory as a sorted run, and merges the runs, the last buffer
// and any input already in order into one stream in ascending key order.
// Memory stays near the budget plus one read buffer per run, whatever the
// number of records.
//
// The output is a function of the records and their order of arrival alone:
// records with equal keys come out in the order they were added, and a sorted
// source's records after every added one with the same key. The budget and the
// run boundaries it sets do not change the output, so two hosts sorting the
// same input produce the same stream.
package extsort

import (
	"bufio"
	"bytes"
	"cmp"
	"container/heap"
	"context"
	"encoding/binary"
	"io"
	"io/fs"
	"iter"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/util/diskfree"
)

// readBufferSize is the read buffer of each run during the merge.
const readBufferSize = 64 << 10

// maxLen is the longest key or value a record holds.
const maxLen = math.MaxUint32

// recordOverhead is the budget a buffered record costs beyond its bytes: its
// entry in the record list.
const recordOverhead = 24

// minMargin is the least free space Reserve leaves beyond its estimate, so a
// sort never leaves the disk full for the writes of the program around it.
const minMargin = 1 << 30

// ErrNoSpace is returned by Reserve when the runs might not fit.
var ErrNoSpace = errors.New("extsort: not enough free space for the runs")

// ErrUnsorted is returned when a sorted source yields a key below the one
// before it.
var ErrUnsorted = errors.New("extsort: sorted source is out of order")

// Sorter sorts records through run files in one directory. It is not safe for
// concurrent use. Close removes its runs.
type Sorter struct {
	// dir holds this Sorter's runs, created on the first spill.
	dir string
	// parent is the directory dir is created in.
	parent string
	// budget is the buffer size in bytes that triggers a spill.
	budget int
	// arena holds the buffered keys and values back to back.
	arena []byte
	// recs locates each buffered record in arena, in arrival order.
	recs []record
	// used is the buffer's size against budget.
	used int
	// runs are the paths of the spilled runs, in spill order.
	runs []string
	// spilled is the bytes written to runs.
	spilled int64
	// merged is set once Merge starts; Add then fails.
	merged bool
}

// record locates one buffered record in the arena.
type record struct {
	off        int
	klen, vlen uint32
}

// New returns a Sorter that spills runs to a new directory inside parent once
// its buffer holds budget bytes.
func New(parent string, budget int) *Sorter {
	return &Sorter{parent: parent, budget: max(budget, 1)}
}

// Reserve checks that the file system of the Sorter's parent directory has room
// for n bytes of runs plus a margin of a quarter of n, at least 1 GiB. A caller
// estimates n before its first Add, so a sort too large for the disk is
// refused before it writes anything. A disk that fills anyway fails the spill,
// and Close removes the runs.
func (s *Sorter) Reserve(n int64) error {
	// Compare the parent's free bytes with the runs plus the margin.
	free, err := diskfree.Bytes(s.parent)
	if err != nil {
		return errors.Wrapf(err, "extsort: free space of %s", s.parent)
	}
	need := uint64(n + max(n/4, minMargin)) //nolint:gosec // n counts bytes, never negative.
	if free < need {
		return errors.Wrapf(ErrNoSpace, "%d bytes free in %s, need %d", free, s.parent, need)
	}
	return nil
}

// Spilled returns the bytes written to run files so far.
func (s *Sorter) Spilled() int64 {
	return s.spilled
}

// Add buffers a copy of one record, spilling the buffer as a run when it
// reaches the budget. An error from the spill, such as a full disk, leaves the
// Sorter unusable; Close still removes its runs.
func (s *Sorter) Add(key, value []byte) error {
	// Refuse records once the merge has begun, and keys or values too long
	// for a run's length prefix.
	if s.merged {
		return errors.New("extsort: add after merge")
	}
	if len(key) > maxLen || len(value) > maxLen {
		return errors.New("extsort: record too long")
	}

	// Buffer the record, then spill a full buffer.
	s.reserve(len(key) + len(value))
	off := len(s.arena)
	s.arena = append(append(s.arena, key...), value...)
	s.recs = append(s.recs, record{off: off, klen: uint32(len(key)), vlen: uint32(len(value))}) //nolint:gosec // both lengths were checked against maxLen.
	s.used += len(key) + len(value) + recordOverhead
	if s.used < s.budget {
		return nil
	}
	return s.spill()
}

// reserve makes room in the buffer for one record of n bytes. Grown by append
// alone, the first buffer would leave every outgrown copy of the arena and the
// record list as garbage, about four times the buffer by the time it spills.
// Once a sixty-fourth of the budget is used, reserve sizes both for the whole
// budget at the mean record size so far, so the first buffer grows about once
// more, and a sort that stays small never takes the budget. Later buffers
// reuse the room the spill leaves.
func (s *Sorter) reserve(n int) {
	if s.used == 0 || s.used < s.budget/64 {
		return
	}
	s.arena = growForBudget(s.arena, n, s.used, s.budget)
	s.recs = growForBudget(s.recs, 1, s.used, s.budget)
}

// growForBudget returns buf with room for n more elements. When buf is full,
// it grows to the length buf would reach at budget bytes used, at its length
// per used byte so far, plus a sixteenth for records larger than the mean.
// used is positive.
func growForBudget[T any](buf []T, n, used, budget int) []T {
	if len(buf)+n <= cap(buf) {
		return buf
	}
	want := int(int64(len(buf)) * int64(budget) / int64(used))
	want += want / 16
	return slices.Grow(buf, max(want, len(buf)+n)-len(buf))
}

// key returns the key of buffered record r.
func (s *Sorter) key(r record) []byte {
	return s.arena[r.off : r.off+int(r.klen)]
}

// value returns the value of buffered record r.
func (s *Sorter) value(r record) []byte {
	k := r.off + int(r.klen)
	return s.arena[k : k+int(r.vlen)]
}

// sortBuffer orders the buffered records by key, keeping arrival order among
// equal keys.
func (s *Sorter) sortBuffer() {
	slices.SortStableFunc(s.recs, func(a, b record) int {
		return bytes.Compare(s.key(a), s.key(b))
	})
}

// spill writes the buffer to a new run file and empties it.
func (s *Sorter) spill() error {
	// Create the run directory on the first spill.
	if s.dir == "" {
		dir, err := os.MkdirTemp(s.parent, runDirPrefix+"*")
		if err != nil {
			return err
		}
		s.dir = dir
	}

	// Write the sorted buffer as the next run.
	s.sortBuffer()
	path := filepath.Join(s.dir, strconv.Itoa(len(s.runs)))
	s.runs = append(s.runs, path)
	n, err := s.writeRun(path)
	if err != nil {
		return err
	}

	// Empty the buffer, keeping its memory for the next run.
	s.spilled += n
	s.arena = s.arena[:0]
	s.recs = s.recs[:0]
	s.used = 0
	return nil
}

// writeRun writes the buffered records to a new file at path and returns the
// bytes written.
func (s *Sorter) writeRun(path string) (int64, error) {
	// Create the file; its close reports a write the disk deferred.
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	n, err := s.writeRecords(f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return n, err
}

// writeRecords writes each buffered record as its key and value lengths, then
// its bytes, and returns the bytes written.
func (s *Sorter) writeRecords(out io.Writer) (int64, error) {
	// Buffer the writes and count the bytes as each record goes out.
	w := bufio.NewWriter(out)
	var n int64
	var hdr [2 * binary.MaxVarintLen64]byte
	for _, r := range s.recs {
		h := binary.AppendUvarint(hdr[:0], uint64(r.klen))
		h = binary.AppendUvarint(h, uint64(r.vlen))
		if _, err := w.Write(h); err != nil {
			return n, err
		}
		if _, err := w.Write(s.arena[r.off : r.off+int(r.klen)+int(r.vlen)]); err != nil {
			return n, err
		}
		n += int64(len(h)) + int64(r.klen) + int64(r.vlen)
	}
	return n, w.Flush()
}

// Merge calls fn with every added record and every record of sorted, in
// ascending key order. Each source in sorted must yield ascending keys; Merge
// returns ErrUnsorted when one does not. key and value are valid only during
// the call. An error from fn stops the merge and is returned. Merge runs once.
func (s *Sorter) Merge(ctx context.Context, sorted []iter.Seq2[[]byte, []byte], fn func(key, value []byte) error) error {
	// Merge once: the cursors consume the runs and the buffer.
	if s.merged {
		return errors.New("extsort: merge called twice")
	}
	s.merged = true

	// Open a cursor on each run, then the buffer, then each sorted source,
	// in the order that breaks ties between equal keys.
	s.sortBuffer()
	var m mergeHeap
	defer m.close()
	for _, path := range s.runs {
		c, err := openRun(path)
		if err != nil {
			return err
		}
		m.add(c)
	}
	m.add(&bufferCursor{s: s})
	for _, src := range sorted {
		next, stop := iter.Pull2(src)
		m.add(&seqCursor{pull: next, stop: stop})
	}
	if err := m.init(); err != nil {
		return err
	}

	// Emit the least record and advance its cursor until every one ends.
	for m.Len() != 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		c := m.items[0]
		if err := fn(c.cur.key(), c.cur.value()); err != nil {
			return err
		}
		ok, err := c.cur.next()
		if err != nil {
			return err
		}
		if ok {
			heap.Fix(&m, 0)
		} else {
			heap.Pop(&m)
		}
	}
	return nil
}

// Close removes the runs and releases the buffer.
func (s *Sorter) Close() error {
	// Drop the buffer; a Sorter that never spilled has no directory.
	s.arena, s.recs, s.runs = nil, nil, nil
	if s.dir == "" {
		return nil
	}

	// Remove the directory and every run in it.
	dir := s.dir
	s.dir = ""
	return os.RemoveAll(dir)
}

// runDirPrefix starts the name of each Sorter's run directory.
const runDirPrefix = "extsort-"

// RemoveRuns removes the run directories that Sorters under parent left
// behind when their process ended before Close. The caller must own parent: a
// Sorter still sorting under it loses its runs. A missing parent holds none.
func RemoveRuns(parent string) error {
	// List the run directories under parent.
	entries, err := os.ReadDir(parent)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	// Remove each with every run in it.
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), runDirPrefix) {
			if err := os.RemoveAll(filepath.Join(parent, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// cursor reads one ascending stream of records.
type cursor interface {
	// next advances to the next record and reports whether there is one.
	next() (bool, error)
	// key returns the current record's key.
	key() []byte
	// value returns the current record's value.
	value() []byte
	// close releases the stream.
	close()
}

// runCursor reads a run file.
type runCursor struct {
	f      *os.File
	r      *bufio.Reader
	buf    []byte
	klen   int
	closed bool
}

// openRun opens a run file at its first record.
func openRun(path string) (*runCursor, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &runCursor{f: f, r: bufio.NewReaderSize(f, readBufferSize)}, nil
}

func (c *runCursor) next() (bool, error) {
	// Read the lengths; a clean end of file ends the run.
	klen, err := binary.ReadUvarint(c.r)
	if err == io.EOF {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	vlen, err := binary.ReadUvarint(c.r)
	if err != nil {
		return false, errors.Wrap(err, "extsort: read run")
	}

	// Read the record into the cursor's buffer; a length past maxLen is a
	// corrupt run.
	if klen > maxLen || vlen > maxLen {
		return false, errors.New("extsort: corrupt run")
	}
	n := int(klen) + int(vlen)
	if cap(c.buf) < n {
		c.buf = make([]byte, n)
	}
	c.buf = c.buf[:n]
	if _, err := io.ReadFull(c.r, c.buf); err != nil {
		return false, errors.Wrap(err, "extsort: read run")
	}
	c.klen = int(klen)
	return true, nil
}

func (c *runCursor) key() []byte   { return c.buf[:c.klen] }
func (c *runCursor) value() []byte { return c.buf[c.klen:] }

func (c *runCursor) close() {
	if !c.closed {
		c.closed = true
		_ = c.f.Close()
	}
}

// bufferCursor reads the sorted buffer.
type bufferCursor struct {
	s *Sorter
	i int
}

func (c *bufferCursor) next() (bool, error) {
	c.i++
	return c.i <= len(c.s.recs), nil
}

func (c *bufferCursor) key() []byte   { return c.s.key(c.s.recs[c.i-1]) }
func (c *bufferCursor) value() []byte { return c.s.value(c.s.recs[c.i-1]) }
func (c *bufferCursor) close()        {}

// seqCursor reads a sorted source and checks that it ascends.
type seqCursor struct {
	pull  func() ([]byte, []byte, bool)
	stop  func()
	k, v  []byte
	prev  []byte
	began bool
}

func (c *seqCursor) next() (bool, error) {
	// Take the next record, refusing a key below the one before it.
	k, v, ok := c.pull()
	if !ok {
		return false, nil
	}
	if c.began && bytes.Compare(k, c.prev) < 0 {
		return false, ErrUnsorted
	}
	c.prev = append(c.prev[:0], k...)
	c.began = true
	c.k, c.v = k, v
	return true, nil
}

func (c *seqCursor) key() []byte   { return c.k }
func (c *seqCursor) value() []byte { return c.v }
func (c *seqCursor) close()        { c.stop() }

// mergeItem is one cursor in the merge, with its tie-breaking rank.
type mergeItem struct {
	cur  cursor
	rank int
}

// mergeHeap orders cursors by current key, then by rank.
type mergeHeap struct {
	items []*mergeItem
	all   []cursor
}

// add places a cursor before its first record.
func (m *mergeHeap) add(c cursor) {
	m.all = append(m.all, c)
}

// init advances each cursor to its first record and builds the heap.
func (m *mergeHeap) init() error {
	for i, c := range m.all {
		ok, err := c.next()
		if err != nil {
			return err
		}
		if ok {
			m.items = append(m.items, &mergeItem{cur: c, rank: i})
		}
	}
	heap.Init(m)
	return nil
}

// close releases every cursor.
func (m *mergeHeap) close() {
	for _, c := range m.all {
		c.close()
	}
}

// Len returns the number of cursors with a record.
func (m *mergeHeap) Len() int { return len(m.items) }

// Less orders cursors by current key, then by rank.
func (m *mergeHeap) Less(i, j int) bool {
	a, b := m.items[i], m.items[j]
	if c := bytes.Compare(a.cur.key(), b.cur.key()); c != 0 {
		return c < 0
	}
	return cmp.Less(a.rank, b.rank)
}

// Swap swaps two cursors.
func (m *mergeHeap) Swap(i, j int) { m.items[i], m.items[j] = m.items[j], m.items[i] }

// Push adds a cursor.
func (m *mergeHeap) Push(x any) { m.items = append(m.items, x.(*mergeItem)) }

// Pop removes the last cursor.
func (m *mergeHeap) Pop() any {
	n := len(m.items) - 1
	x := m.items[n]
	m.items = m.items[:n]
	return x
}
