//go:build js

package volume_idb

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"sync"
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/device"
)

const (
	// fileStore maps each file name to its length.
	fileStore = "files"
	// chunkStore holds file contents as chunks keyed by [name, index]. A
	// chunk may be shorter than ChunkSize or absent; the missing bytes below
	// the file length read as zero.
	chunkStore = "chunks"
)

// ChunkSize is the length of a full chunk.
const ChunkSize = 64 << 10

// lockPrefix prefixes the name of the Web Lock each Device holds.
const lockPrefix = "volume-device-idb/"

// Device is a device.Device on one IndexedDB database. It is the database's
// only writer, enforced by an exclusive Web Lock held from OpenDevice until
// Close: it holds the file lengths and the last chunk written to each file,
// so appends need no read.
type Device struct {
	// db is the database connection.
	db js.Value
	// release releases the Device's Web Lock.
	release func()

	// mtx serializes the device calls.
	mtx sync.Mutex
	// sizes holds each file's length.
	sizes map[string]int64
	// tails holds the last chunk written to each file.
	tails map[string]tail
}

// tail is a file's last written chunk.
type tail struct {
	// index is the chunk index.
	index int64
	// data is the chunk.
	data []byte
}

// chunkID names one chunk.
type chunkID struct {
	// name is the file.
	name string
	// index is the chunk index.
	index int64
}

// OpenDevice opens the device database name, creating it when absent. It
// returns device.ErrHeld while another Device in the origin has name open.
func OpenDevice(ctx context.Context, name string) (*Device, error) {
	release, acquired, err := opfs.AcquireWebLockIfAvailable(lockPrefix+name, true)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, errors.Wrap(device.ErrHeld, name)
	}
	db, err := openDB(name, fileStore, chunkStore, syncStore)
	if err != nil {
		release()
		return nil, err
	}

	// Load the file lengths.
	tx := transaction(db, false, false, fileStore)
	store := tx.Call("objectStore", fileStore)
	keysReq, sizesReq := store.Call("getAllKeys"), store.Call("getAll")
	if err := complete(tx); err != nil {
		db.Call("close")
		release()
		return nil, err
	}
	d := &Device{db: db, release: release, sizes: make(map[string]int64), tails: make(map[string]tail)}
	keys, sizes := keysReq.Get("result"), sizesReq.Get("result")
	for i := range keys.Length() {
		d.sizes[keys.Index(i).String()] = int64(sizes.Index(i).Float())
	}
	return d, nil
}

// Write applies writes in one transaction, strict when flush is set. A chunk
// the batch overwrites only in part is read first in one read transaction,
// unless it is a file's last written chunk.
func (d *Device) Write(ctx context.Context, writes []device.Write, flush bool) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	// Find the chunks the batch changes and the ones it needs to read.
	chunks := make(map[chunkID][]byte)
	var order []chunkID
	var reads []chunkID
	sizes := make(map[string]int64)
	for _, w := range writes {
		if err := device.ValidName(w.Name); err != nil {
			return err
		}
		size, ok := sizes[w.Name]
		if !ok {
			size = d.sizes[w.Name]
		}
		for index := w.Offset / ChunkSize; index*ChunkSize < w.Offset+int64(len(w.Data)); index++ {
			id := chunkID{w.Name, index}
			if _, ok := chunks[id]; ok {
				continue
			}
			order = append(order, id)
			start := index * ChunkSize
			covered := w.Offset <= start && w.Offset+int64(len(w.Data)) >= start+ChunkSize
			switch t, ok := d.tails[w.Name]; {
			case covered || start >= d.sizes[w.Name]:
				chunks[id] = nil
			case ok && t.index == index:
				chunks[id] = bytes.Clone(t.data)
			default:
				chunks[id] = nil
				reads = append(reads, id)
			}
		}
		sizes[w.Name] = max(size, w.Offset+int64(len(w.Data)))
	}
	if err := d.readChunks(chunks, reads); err != nil {
		return err
	}

	// Apply the writes to the chunks in order.
	for _, w := range writes {
		for off := w.Offset; off < w.Offset+int64(len(w.Data)); {
			id := chunkID{w.Name, off / ChunkSize}
			at := off - id.index*ChunkSize
			n := min(ChunkSize-at, w.Offset+int64(len(w.Data))-off)
			chunk := chunks[id]
			if int64(len(chunk)) < at+n {
				chunk = append(chunk, make([]byte, at+n-int64(len(chunk)))...)
			}
			copy(chunk[at:at+n], w.Data[off-w.Offset:])
			chunks[id] = chunk
			off += n
		}
	}

	// Store the chunks and lengths.
	tx := transaction(d.db, true, flush, fileStore, chunkStore, syncStore)
	chunkObjects := tx.Call("objectStore", chunkStore)
	for _, id := range order {
		chunkObjects.Call("put", toJS(chunks[id]), chunkKey(id))
	}
	fileObjects := tx.Call("objectStore", fileStore)
	for name, size := range sizes {
		if size != d.sizes[name] {
			fileObjects.Call("put", size, name)
		}
	}
	if len(order) == 0 && flush {
		tx.Call("objectStore", syncStore).Call("put", 0, 0)
	}
	if err := complete(tx); err != nil {
		return err
	}

	// Record the new lengths and each file's last written chunk.
	for name, size := range sizes {
		d.sizes[name] = size
	}
	for _, id := range order {
		if t, ok := d.tails[id.name]; !ok || id.index >= t.index {
			d.tails[id.name] = tail{index: id.index, data: chunks[id]}
		}
	}
	return nil
}

// Read fills every read, reading the chunks in one transaction.
func (d *Device) Read(ctx context.Context, reads []device.Read) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	// Find the chunks the reads cover.
	chunks := make(map[chunkID][]byte)
	var ids []chunkID
	for _, r := range reads {
		size, ok := d.sizes[r.Name]
		if !ok || r.Offset+int64(len(r.Data)) > size {
			return errors.Wrapf(device.ErrShortRead, "%s at %d", r.Name, r.Offset)
		}
		for index := r.Offset / ChunkSize; index*ChunkSize < r.Offset+int64(len(r.Data)); index++ {
			id := chunkID{r.Name, index}
			if _, ok := chunks[id]; ok {
				continue
			}
			if t, ok := d.tails[r.Name]; ok && t.index == index {
				chunks[id] = t.data
				continue
			}
			chunks[id] = nil
			ids = append(ids, id)
		}
	}
	if err := d.readChunks(chunks, ids); err != nil {
		return err
	}

	// Copy each range, zero filling past a chunk's end.
	for _, r := range reads {
		for off := r.Offset; off < r.Offset+int64(len(r.Data)); {
			id := chunkID{r.Name, off / ChunkSize}
			at := off - id.index*ChunkSize
			n := min(ChunkSize-at, r.Offset+int64(len(r.Data))-off)
			dst := r.Data[off-r.Offset : off-r.Offset+n]
			clear(dst)
			if chunk := chunks[id]; at < int64(len(chunk)) {
				copy(dst, chunk[at:])
			}
			off += n
		}
	}
	return nil
}

// Truncate sets a file's length. Shrinking deletes the chunks past the end
// and cuts the new last chunk, so growing again reads zeros.
func (d *Device) Truncate(ctx context.Context, name string, size int64) error {
	if err := device.ValidName(name); err != nil {
		return err
	}
	d.mtx.Lock()
	defer d.mtx.Unlock()

	// Read the new last chunk when the cut falls inside it.
	cut := chunkID{name, size / ChunkSize}
	chunks := map[chunkID][]byte{cut: nil}
	old := d.sizes[name]
	cutting := size < old && size%ChunkSize != 0
	t, hasTail := d.tails[name]
	switch {
	case hasTail && t.index == cut.index:
		chunks[cut] = t.data
	case cutting:
		if err := d.readChunks(chunks, []chunkID{cut}); err != nil {
			return err
		}
	}

	// Delete the chunks past the end, cut the last one, and store the length.
	tx := transaction(d.db, true, false, fileStore, chunkStore)
	chunkObjects := tx.Call("objectStore", chunkStore)
	if size < old {
		first := (size + ChunkSize - 1) / ChunkSize
		chunkObjects.Call("delete", fileRange(name, first))
	}
	chunk := chunks[cut]
	if cutting && int64(len(chunk)) > size%ChunkSize {
		chunk = chunk[:size%ChunkSize]
		chunkObjects.Call("put", toJS(chunk), chunkKey(cut))
	}
	tx.Call("objectStore", fileStore).Call("put", size, name)
	if err := complete(tx); err != nil {
		return err
	}
	d.sizes[name] = size
	delete(d.tails, name)
	return nil
}

// Remove deletes files and their chunks in one transaction.
func (d *Device) Remove(ctx context.Context, names []string) error {
	for _, name := range names {
		if err := device.ValidName(name); err != nil {
			return err
		}
	}
	d.mtx.Lock()
	defer d.mtx.Unlock()
	tx := transaction(d.db, true, false, fileStore, chunkStore)
	fileObjects, chunkObjects := tx.Call("objectStore", fileStore), tx.Call("objectStore", chunkStore)
	for _, name := range names {
		fileObjects.Call("delete", name)
		chunkObjects.Call("delete", fileRange(name, 0))
	}
	if err := complete(tx); err != nil {
		return err
	}
	for _, name := range names {
		delete(d.sizes, name)
		delete(d.tails, name)
	}
	return nil
}

// List returns every file from the lengths the device holds.
func (d *Device) List(ctx context.Context) ([]device.File, error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	out := make([]device.File, 0, len(d.sizes))
	for name, size := range d.sizes {
		out = append(out, device.File{Name: name, Size: size})
	}
	slices.SortFunc(out, func(a, b device.File) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// Close closes the database connection and releases the Web Lock.
func (d *Device) Close() error {
	d.db.Call("close")
	d.release()
	return nil
}

// readChunks reads the chunks of ids into chunks in one transaction, leaving
// an absent chunk nil.
func (d *Device) readChunks(chunks map[chunkID][]byte, ids []chunkID) error {
	if len(ids) == 0 {
		return nil
	}
	tx := transaction(d.db, false, false, chunkStore)
	store := tx.Call("objectStore", chunkStore)
	reqs := make([]js.Value, len(ids))
	for i, id := range ids {
		reqs[i] = store.Call("get", chunkKey(id))
	}
	if err := complete(tx); err != nil {
		return err
	}
	for i, req := range reqs {
		if v := req.Get("result"); !v.IsUndefined() {
			chunks[ids[i]] = toGo(v)
		}
	}
	return nil
}

// chunkKey returns the key of one chunk.
func chunkKey(id chunkID) js.Value {
	return js.ValueOf([]any{id.name, id.index})
}

// fileRange returns the key range of a file's chunks from index first.
func fileRange(name string, first int64) js.Value {
	upper := js.ValueOf([]any{name, js.Global().Get("Infinity")})
	return js.Global().Get("IDBKeyRange").Call("bound", js.ValueOf([]any{name, first}), upper)
}

// _ checks that Device is a device.Device.
var _ device.Device = (*Device)(nil)
