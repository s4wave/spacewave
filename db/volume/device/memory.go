package device

import (
	"context"
	"maps"
	"math/rand/v2"
	"slices"
	"sync"

	"github.com/pkg/errors"
)

// SectorSize is the unit a torn write keeps or loses in a simulated power
// loss.
const SectorSize = 512

// ErrCrashed reports a call on a memory device after its injected crash.
var ErrCrashed = errors.New("device crashed")

// Memory is an in-memory Device that simulates power loss. It keeps the
// durable image as of the last flush and the calls made since, so PowerLoss
// can build any image a crash could leave.
type Memory struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// files is the current image every read sees.
	files map[string][]byte
	// durable is the image as of the last completed flush.
	durable map[string][]byte
	// pending holds the mutations since the last completed flush, in order.
	pending []memoryOp
	// crashAfter is the number of further mutating calls that succeed before
	// the crash; negative never crashes.
	crashAfter int
	// crashed rejects every call until PowerLoss.
	crashed bool
	// calls counts mutating calls, including rejected ones.
	calls int
}

// memoryOp is one unflushed mutation.
type memoryOp struct {
	// name is the file the mutation changes.
	name string
	// remove deletes the file.
	remove bool
	// truncate sets the file length to size.
	truncate bool
	// size is the truncated length.
	size int64
	// offset is the offset of a write.
	offset int64
	// data is a write's bytes.
	data []byte
}

// NewMemory returns an empty memory device that never crashes.
func NewMemory() *Memory {
	return &Memory{
		files:      make(map[string][]byte),
		durable:    make(map[string][]byte),
		crashAfter: -1,
	}
}

// CrashAfter crashes the device after n further mutating calls succeed. The
// crashing call applies part or none of its mutations, and every later call
// fails with ErrCrashed until PowerLoss. A negative n never crashes.
func (m *Memory) CrashAfter(n int) {
	m.mtx.Lock()
	m.crashAfter = n
	m.mtx.Unlock()
}

// Calls returns the number of Write, Truncate, and Remove calls made.
func (m *Memory) Calls() int {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.calls
}

// PowerLoss replaces the device contents with an image a power loss could
// leave: every flushed mutation, and each later one kept, dropped, or for a
// write torn to a random subset of its sectors, as rng chooses. The device then
// accepts calls again.
func (m *Memory) PowerLoss(rng *rand.Rand) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	image := cloneImage(m.durable)
	for _, op := range m.pending {
		switch {
		case op.remove || op.truncate:
			if rng.IntN(2) == 0 {
				applyOp(image, op)
			}
		default:
			applyWrite(image, op, rng)
		}
	}
	m.files, m.durable, m.pending = image, cloneImage(image), nil
	m.crashed, m.crashAfter = false, -1
}

// Write applies writes in order and flushes when flush is set.
func (m *Memory) Write(ctx context.Context, writes []Write, flush bool) error {
	for _, w := range writes {
		if err := ValidName(w.Name); err != nil {
			return err
		}
	}
	ops := make([]memoryOp, len(writes))
	for i, w := range writes {
		ops[i] = memoryOp{name: w.Name, offset: w.Offset, data: slices.Clone(w.Data)}
	}
	return m.mutate(ctx, ops, flush)
}

// Truncate sets a file's length.
func (m *Memory) Truncate(ctx context.Context, name string, size int64) error {
	if err := ValidName(name); err != nil {
		return err
	}
	return m.mutate(ctx, []memoryOp{{name: name, truncate: true, size: size}}, false)
}

// Remove deletes files.
func (m *Memory) Remove(ctx context.Context, names []string) error {
	ops := make([]memoryOp, len(names))
	for i, name := range names {
		if err := ValidName(name); err != nil {
			return err
		}
		ops[i] = memoryOp{name: name, remove: true}
	}
	return m.mutate(ctx, ops, false)
}

// Read fills every read from the current image.
func (m *Memory) Read(ctx context.Context, reads []Read) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.crashed {
		return ErrCrashed
	}
	for _, r := range reads {
		data, ok := m.files[r.Name]
		end := r.Offset + int64(len(r.Data))
		if !ok || r.Offset < 0 || end > int64(len(data)) {
			return errors.Wrapf(ErrShortRead, "%s at %d", r.Name, r.Offset)
		}
		copy(r.Data, data[r.Offset:end])
	}
	return nil
}

// List returns every file in the current image.
func (m *Memory) List(ctx context.Context) ([]File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.crashed {
		return nil, ErrCrashed
	}
	files := make([]File, 0, len(m.files))
	for name, data := range m.files {
		files = append(files, File{Name: name, Size: int64(len(data))})
	}
	return files, nil
}

// mutate applies one mutating call, injecting the crash when it is due. The
// crashing call's mutations join the unflushed ones, so PowerLoss may keep
// any part of them.
func (m *Memory) mutate(ctx context.Context, ops []memoryOp, flush bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.calls++
	if m.crashed {
		return ErrCrashed
	}
	m.pending = append(m.pending, ops...)
	if m.crashAfter == 0 {
		m.crashed = true
		return ErrCrashed
	}
	if m.crashAfter > 0 {
		m.crashAfter--
	}
	for _, op := range ops {
		applyOp(m.files, op)
	}
	if flush {
		for _, op := range m.pending {
			applyOp(m.durable, op)
		}
		m.pending = nil
	}
	return nil
}

// applyOp applies one mutation to image in full.
func applyOp(image map[string][]byte, op memoryOp) {
	switch {
	case op.remove:
		delete(image, op.name)
	case op.truncate:
		image[op.name] = resize(image[op.name], op.size)
	default:
		data := image[op.name]
		data = resize(data, max(int64(len(data)), op.offset+int64(len(op.data))))
		copy(data[op.offset:], op.data)
		image[op.name] = data
	}
}

// applyWrite applies an unflushed write to image as a power loss could leave
// it: in full, not at all, or extended with a random subset of its sectors.
func applyWrite(image map[string][]byte, op memoryOp, rng *rand.Rand) {
	switch rng.IntN(3) {
	case 0:
		applyOp(image, op)
		return
	case 1:
		return
	}
	data := image[op.name]
	data = resize(data, max(int64(len(data)), op.offset+int64(len(op.data))))
	for start := 0; start < len(op.data); start += SectorSize {
		if rng.IntN(2) == 0 {
			end := min(start+SectorSize, len(op.data))
			copy(data[op.offset+int64(start):], op.data[start:end])
		}
	}
	image[op.name] = data
}

// resize returns data with length size, zero filling any growth.
func resize(data []byte, size int64) []byte {
	if size <= int64(len(data)) {
		return data[:size]
	}
	return append(data, make([]byte, size-int64(len(data)))...)
}

// cloneImage deep copies a file image.
func cloneImage(image map[string][]byte) map[string][]byte {
	out := maps.Clone(image)
	for name, data := range out {
		out[name] = slices.Clone(data)
	}
	return out
}

// _ is a type assertion
var _ Device = (*Memory)(nil)
