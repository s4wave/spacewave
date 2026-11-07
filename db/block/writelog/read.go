package writelog

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// segmentExt is the extension of a segment file.
const segmentExt = ".wlog"

// Position names a line of an epoch: the reader returns the lines after it.
type Position struct {
	// Epoch is the epoch's id.
	Epoch string
	// Seq is the line, zero for before the first.
	Seq uint64
}

// String encodes the position as epoch/seq, empty for the zero position.
func (p Position) String() string {
	if p.Epoch == "" {
		return ""
	}
	return p.Epoch + "/" + strconv.FormatUint(p.Seq, 10)
}

// ParsePosition parses a position String wrote.
func ParsePosition(s string) (Position, error) {
	// Split the epoch from the sequence and parse the sequence.
	epoch, seq, ok := strings.Cut(s, "/")
	if !ok || epoch == "" {
		return Position{}, errors.Errorf("malformed log position %q", s)
	}
	n, err := strconv.ParseUint(seq, 10, 64)
	if err != nil {
		return Position{}, errors.Wrapf(err, "log position %q", s)
	}
	return Position{Epoch: epoch, Seq: n}, nil
}

// ReadState is where a read stopped.
type ReadState int

// Read states.
const (
	// ReadMore stopped at the line bound, with more lines written.
	ReadMore ReadState = iota
	// ReadOpen read every line written so far of an epoch that is open.
	ReadOpen
	// ReadClosed read the epoch to its close line.
	ReadClosed
	// ReadGap found lines missing: the epoch is gone, trimmed past the
	// position, broken, or ended with no close line.
	ReadGap
)

// String names the state.
func (s ReadState) String() string {
	switch s {
	case ReadMore:
		return "more"
	case ReadOpen:
		return "open"
	case ReadClosed:
		return "closed"
	default:
		return "gap"
	}
}

// ReadResult is where a read stopped.
type ReadResult struct {
	// State is why it stopped.
	State ReadState
	// Next is the last line read, or the position read from when none was.
	Next Position
	// Reason says what a gap found, empty for none.
	Reason string
}

// segment is one segment file.
type segment struct {
	// epoch is its epoch.
	epoch string
	// first is its first line.
	first uint64
	// size is its size on disk.
	size int64
}

// path returns the segment's path in dir.
func (s segment) path(dir string) string {
	return filepath.Join(dir, fmt.Sprintf("%s-%020d%s", s.epoch, s.first, segmentExt))
}

// listSegments lists the segments of dir by epoch, then first line.
func listSegments(dir string) ([]segment, error) {
	// Read the directory; a missing one holds none.
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Parse each segment's name and size.
	var segs []segment
	for _, e := range entries {
		base, ok := strings.CutSuffix(e.Name(), segmentExt)
		epoch, first, ok2 := strings.Cut(base, "-")
		n, err := strconv.ParseUint(first, 10, 64)
		if !ok || !ok2 || err != nil || e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		segs = append(segs, segment{epoch: epoch, first: n, size: info.Size()})
	}
	slices.SortFunc(segs, func(a, b segment) int {
		if c := strings.Compare(a.epoch, b.epoch); c != 0 {
			return c
		}
		return cmp.Compare(a.first, b.first)
	})
	return segs, nil
}

// epochSegments returns the segments of epoch in order.
func epochSegments(segs []segment, epoch string) []segment {
	var out []segment
	for _, s := range segs {
		if s.epoch == epoch {
			out = append(out, s)
		}
	}
	return out
}

// readDir reads the lines of dir after from, at most max when max is
// positive. live is set when from's epoch is open, with upto its last line
// flushed; a live read stops at upto, and an unclosed epoch read while not live
// is a gap.
func readDir(ctx context.Context, dir string, from Position, live bool, upto uint64, max int, fn func(Line) error) (ReadResult, error) {
	// List the epoch's segments.
	res := ReadResult{Next: from}
	all, err := listSegments(dir)
	if err != nil {
		return res, err
	}
	segs := epochSegments(all, from.Epoch)
	if len(segs) == 0 {
		return gap(res, "epoch "+from.Epoch+" not found"), nil
	}

	// Start at the last segment beginning at or before the next line.
	want := from.Seq + 1
	if segs[0].first > want {
		return gap(res, "lines trimmed past the position"), nil
	}
	start := 0
	for i, s := range segs {
		if s.first <= want {
			start = i
		}
	}

	// Read the segments in order, each continuing the last.
	read := 0
	for _, seg := range segs[start:] {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		state, err := readSegment(dir, seg, &res, live, upto, max, &read, fn)
		if err != nil || state != nil {
			if state != nil {
				res = *state
			}
			return res, err
		}
	}

	// The epoch has no close line: open when live, else a gap.
	if live {
		res.State = ReadOpen
		return res, nil
	}
	return gap(res, "epoch ended without a close line"), nil
}

// readSegment reads one segment's lines after res.Next into fn, advancing
// res.Next. It returns the read's final result when the read ends in this
// segment, nil to go on to the next.
func readSegment(dir string, seg segment, res *ReadResult, live bool, upto uint64, max int, read *int, fn func(Line) error) (*ReadResult, error) {
	// Open the segment; one trimmed since the listing is a gap.
	f, err := os.Open(seg.path(dir))
	if os.IsNotExist(err) {
		out := gap(*res, "segment removed while reading")
		return &out, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, bufferBytes)

	// Check the epoch line continues the lines read so far.
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	seq, kind, rest, perr := parseLine(line)
	if line == nil || perr != nil || kind != kindEpoch || string(rest) != seg.epoch || seq != seg.first || seg.first > res.Next.Seq+1 {
		out := gap(*res, "segment "+filepath.Base(seg.path(dir))+" does not continue the epoch")
		return &out, nil
	}
	prev := seg.first - 1

	// Read each line, each the one after the last.
	for {
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		if line == nil {
			return nil, nil
		}
		seq, kind, rest, err := parseLine(line)
		if err != nil {
			return nil, err
		}
		if kind == kindClose {
			if seq != prev || string(rest) != seg.epoch {
				out := gap(*res, "malformed close line")
				return &out, nil
			}
			res.State = ReadClosed
			return res, nil
		}
		if seq != prev+1 {
			out := gap(*res, fmt.Sprintf("line %d follows line %d", seq, prev))
			return &out, nil
		}
		prev = seq

		// Skip the lines at or before the position, and stop a live read
		// at its last flushed line or a read at its bound.
		if seq <= res.Next.Seq {
			continue
		}
		if live && seq > upto {
			res.State = ReadOpen
			return res, nil
		}
		if max > 0 && *read >= max {
			res.State = ReadMore
			return res, nil
		}
		h, err := parseHash(rest)
		if err != nil {
			return nil, err
		}
		if err := fn(Line{Seq: seq, Kind: kind, Hash: h}); err != nil {
			return nil, err
		}
		res.Next.Seq = seq
		*read++
	}
}

// readLine reads one whole line without its newline, or nil at the end of
// the file or at a final line its writer had not finished.
func readLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadSlice('\n')
	switch err {
	case nil:
		return bytes.TrimSuffix(line, []byte{'\n'}), nil
	case io.EOF:
		return nil, nil
	case bufio.ErrBufferFull:
		return nil, errors.New("log line longer than the read buffer")
	default:
		return nil, err
	}
}

// gap returns res as a gap with reason.
func gap(res ReadResult, reason string) ReadResult {
	res.State, res.Reason = ReadGap, reason
	return res
}

// ReadLog calls fn with the lines after from, at most max when max is
// positive, flushing the buffered lines first when from's epoch is open.
func (l *Log) ReadLog(ctx context.Context, from Position, max int, fn func(Line) error) (ReadResult, error) {
	// Flush the open epoch's lines and note its last, under the lock.
	l.mu.Lock()
	dir := l.opts.Dir
	live := l.file != nil && from.Epoch == l.epoch
	upto := l.seq
	if live {
		if err := l.w.Flush(); err != nil {
			l.breakEpoch(err)
			live = false
		}
	}
	l.mu.Unlock()

	// Read without the lock.
	if dir == "" {
		return gap(ReadResult{Next: from}, "write log is off"), nil
	}
	return readDir(ctx, dir, from, live, upto, max, fn)
}

// TrimLog removes the segments whose every line is at or before to, and the
// segments of every epoch but to's and the open one.
func (l *Log) TrimLog(_ context.Context, to Position) error {
	// Trim under the lock, which guards the tracked bytes.
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.opts.Dir == "" {
		return nil
	}
	removed, err := trimDir(l.opts.Dir, to, l.epoch)
	l.bytes -= removed
	return err
}

// trimDir removes the segments of dir whose every line is at or before to,
// keeping each epoch's last segment, and every segment of an epoch other than
// to's and keep. It returns the bytes removed.
func trimDir(dir string, to Position, keep string) (int64, error) {
	// List the segments.
	segs, err := listSegments(dir)
	if err != nil {
		return 0, err
	}

	// Remove each covered one.
	var removed int64
	for i, s := range segs {
		var drop bool
		switch s.epoch {
		case to.Epoch:
			drop = i+1 < len(segs) && segs[i+1].epoch == s.epoch && segs[i+1].first-1 <= to.Seq
		case keep:
		default:
			drop = true
		}
		if !drop {
			continue
		}
		if err := os.Remove(s.path(dir)); err != nil && !os.IsNotExist(err) {
			return removed, err
		}
		removed += s.size
	}
	return removed, nil
}

// Dir reads and trims a log directory without its writer, such as a copy. It
// cannot tell an epoch still written from one whose writer stopped, so it
// reads an epoch with no close line as open; a writer's own ReadLog answers a
// gap instead.
type Dir string

// ReadLog calls fn with the lines of the directory after from, at most max
// when max is positive.
func (d Dir) ReadLog(ctx context.Context, from Position, max int, fn func(Line) error) (ReadResult, error) {
	return readDir(ctx, string(d), from, true, ^uint64(0), max, fn)
}

// TrimLog removes the segments of the directory whose every line is at or
// before to, and every segment of another epoch.
func (d Dir) TrimLog(_ context.Context, to Position) error {
	_, err := trimDir(string(d), to, to.Epoch)
	return err
}
