package device

import (
	"context"
	"io"
	"slices"
	"sync"
)

// Handle is one device file used as a positional file. Writes collect until
// Sync, which issues them with the flush in one device call, so a caller that
// writes many ranges and then syncs costs one round trip. Close and every
// other call issue collected writes first, unflushed, to keep device order.
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
			h.size = f.Size
		}
	}
	return h, nil
}

// ReadAt reads len(p) bytes at off, returning io.EOF when the file ends first.
func (h *Handle) ReadAt(p []byte, off int64) (int, error) {
	// Issue pending handle writes before reading under its lock.
	h.mtx.Lock()
	defer h.mtx.Unlock()
	if err := h.issue(false); err != nil {
		return 0, err
	}

	// Read the available file range and report EOF for a short result.
	n := int(min(int64(len(p)), max(h.size-off, 0)))
	if n != 0 {
		if err := h.dev.Read(h.ctx, []Read{{Name: h.name, Offset: off, Data: p[:n]}}); err != nil {
			return 0, err
		}
	}
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
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
	h.size = size
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
// makes the call even when nothing is collected.
func (h *Handle) issue(flush bool) error {
	if len(h.pending) == 0 && !flush {
		return nil
	}
	err := h.dev.Write(h.ctx, h.pending, flush)
	h.pending = nil
	return err
}
