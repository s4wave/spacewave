package device

import (
	"context"
	"io"
	"slices"
	"sync"
)

// Handle is one device file used as a positional file. Writes collect until
// Sync, which issues them with the flush in one device call, so a caller that
// writes many ranges and then syncs costs one round trip. Truncate and Close
// issue collected writes first, unflushed, to keep device order; reads see
// them without issuing. A failed device write keeps the collected writes for
// the next call, so a write the caller saw succeed is never dropped.
// A Handle satisfies bbolt's Storage.
type Handle struct {
	// ctx bounds every device call the handle makes.
	ctx context.Context
	// dev is the device holding the file.
	dev Device
	// name is the file name.
	name string

	// mtx guards the fields below.
	mtx sync.Mutex
	// size is the file length including collected writes.
	size int64
	// devSize is the file length on the device, excluding collected writes.
	devSize int64
	// pending holds collected writes in order.
	pending []Write
}

// OpenHandle returns a Handle on the named file, which need not exist.
func OpenHandle(ctx context.Context, dev Device, name string) (*Handle, error) {
	// Validate the device file name before opening a handle.
	if err := ValidName(name); err != nil {
		return nil, err
	}

	// Read device file metadata to initialize the handle size.
	files, err := dev.List(ctx)
	if err != nil {
		return nil, err
	}

	// Construct the handle with the existing file size when present.
	h := &Handle{ctx: ctx, dev: dev, name: name}
	for _, f := range files {
		if f.Name == name {
			h.size, h.devSize = f.Size, f.Size
		}
	}
	return h, nil
}

// ReadAt reads len(p) bytes at off, returning io.EOF when the file ends first.
func (h *Handle) ReadAt(p []byte, off int64) (int, error) {
	// Read the range the device holds and zero the rest of the file range.
	h.mtx.Lock()
	defer h.mtx.Unlock()
	n := min(int64(len(p)), max(h.size-off, 0))
	d := min(n, max(h.devSize-off, 0))
	if d != 0 {
		if err := h.dev.Read(h.ctx, []Read{{Name: h.name, Offset: off, Data: p[:d]}}); err != nil {
			return 0, err
		}
	}
	clear(p[d:n])

	// Lay the collected writes over the range in the order they were made.
	for _, w := range h.pending {
		start, end := max(w.Offset, off), min(w.Offset+int64(len(w.Data)), off+n)
		if start < end {
			copy(p[start-off:end-off], w.Data[start-w.Offset:])
		}
	}

	// Report EOF for a short result.
	if n < int64(len(p)) {
		return int(n), io.EOF
	}
	return int(n), nil
}

// WriteAt collects a copy of p for the next device call, merging it into the
// previous write when it continues that write.
func (h *Handle) WriteAt(p []byte, off int64) (int, error) {
	// Merge a contiguous range into the preceding pending handle write under its lock.
	h.mtx.Lock()
	defer h.mtx.Unlock()
	if n := len(h.pending); n != 0 {
		last := &h.pending[n-1]
		if last.Offset+int64(len(last.Data)) == off {
			last.Data = append(last.Data, p...)
			h.size = max(h.size, off+int64(len(p)))
			return len(p), nil
		}
	}

	// Collect a separate pending write and extend the handle size as needed.
	h.pending = append(h.pending, Write{Name: h.name, Offset: off, Data: slices.Clone(p)})
	h.size = max(h.size, off+int64(len(p)))
	return len(p), nil
}

// Size returns the file length including collected writes.
func (h *Handle) Size() (int64, error) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return h.size, nil
}

// Truncate sets the file length.
func (h *Handle) Truncate(size int64) error {
	// Issue pending handle writes before truncating under its lock.
	h.mtx.Lock()
	defer h.mtx.Unlock()
	if err := h.issue(false); err != nil {
		return err
	}

	// Truncate the device file and retain its new size in the handle.
	if err := h.dev.Truncate(h.ctx, h.name, size); err != nil {
		return err
	}
	h.size, h.devSize = size, size
	return nil
}

// Sync issues the collected writes with a flush, making every earlier call on
// the device durable, including calls through other handles.
func (h *Handle) Sync() error {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return h.issue(true)
}

// Close issues the collected writes without a flush.
func (h *Handle) Close() error {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return h.issue(false)
}

// issue sends the collected writes in one device call. With flush set it
// makes the call even when nothing is collected. On failure it keeps the
// writes; the device may hold any part of them, and sending them again in
// order rewrites the same bytes.
func (h *Handle) issue(flush bool) error {
	if len(h.pending) == 0 && !flush {
		return nil
	}
	if err := h.dev.Write(h.ctx, h.pending, flush); err != nil {
		return err
	}
	h.pending, h.devSize = nil, h.size
	return nil
}
