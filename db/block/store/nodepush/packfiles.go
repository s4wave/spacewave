package block_store_nodepush

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/sirupsen/logrus"
)

// fillQueue bounds the packs waiting to fill. A miss past it is forgotten, and
// asked for again by the next read of the pack.
const fillQueue = 1024

// maxReadBytes bounds one read, so a read's answer never holds up the answers
// commits wait for on the same connection. A larger block reads the remote.
const maxReadBytes = 1 << 20

const (
	// sumChunk is the bytes one sum covers. A read costs at most two of these
	// past its range, which is sequential on the disk it already seeks on.
	sumChunk = 64 << 10
	// sumSize is the size of one sum.
	sumSize = 4
	// sumsSuffix ends the name of a pack's sums file.
	sumsSuffix = ".sums"
)

// sumTable is the CRC-32C table of the chunk sums.
var sumTable = crc32.MakeTable(crc32.Castagnoli)

// Source reads packs from the remote.
type Source interface {
	// GetObject opens the object's body. The caller must Close it.
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
}

// PackFiles keeps whole packs on the node's disk and serves byte ranges of
// them. It holds no table of blocks: the local store's index already knows
// which pack and offset it wants, so the node's memory doesn't grow with the
// store. A pack it lacks fills from the remote in the background.
type PackFiles struct {
	le     *logrus.Entry
	dir    string
	remote Source
	bucket string
	// prefix is the object key prefix of the packs.
	prefix string
	fills  chan string
	// room reports whether the disk can take another pack, or is nil.
	room atomic.Pointer[func() bool]

	// mtx guards queued.
	mtx sync.Mutex
	// queued holds the packs in fills, so a pack queues once.
	queued map[string]struct{}
}

// NewPackFiles keeps packs in dir, filling them from remote's objects named
// prefix plus the pack id in bucket.
func NewPackFiles(le *logrus.Entry, dir string, remote Source, bucket, prefix string) (*PackFiles, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &PackFiles{
		le:     le,
		dir:    dir,
		remote: remote,
		bucket: bucket,
		prefix: prefix,
		fills:  make(chan string, fillQueue),
		queued: make(map[string]struct{}),
	}, nil
}

// SetRoom makes the node keep and fill packs only while room reports true, so
// read copies never take the disk the pushes need. Nil always keeps them.
func (p *PackFiles) SetRoom(room func() bool) {
	if room == nil {
		p.room.Store(nil)
		return
	}
	p.room.Store(&room)
}

// hasRoom reports whether the disk can take another pack.
func (p *PackFiles) hasRoom() bool {
	room := p.room.Load()
	return room == nil || (*room)()
}

// ReadRange reads a range of pack id, and queues a fill when it lacks the pack.
// It answers only bytes whose chunks match the sums taken as the pack arrived:
// the local store can check a block's data against its hash, but not its refs.
// A pack that fails drops, so it fills again.
func (p *PackFiles) ReadRange(id string, off int64, length uint32) ([]byte, bool) {
	// Only a valid name reaches the disk: it comes off the wire.
	if identity.ValidatePackID(id) != nil || off < 0 || length > maxReadBytes {
		return nil, false
	}
	f, err := os.Open(p.path(id)) // #nosec G304 -- the id was validated above
	if err != nil {
		p.queueFill(id)
		return nil, false
	}
	defer f.Close()

	// Read the whole chunks the range touches, and their sums.
	data, start, err := readChecked(f, p.sumsPath(id), off, length)
	if err != nil {
		p.le.WithError(err).WithField("pack", id).Warn("dropping a damaged pack copy")
		p.drop(id)
		return nil, false
	}

	// A range past the end comes back short, as an object store's does.
	from := min(off-start, int64(len(data)))
	to := min(from+int64(length), int64(len(data)))
	return data[from:to], true
}

// readChecked reads the chunks of f holding the range, verified against the
// sums file, and returns them with the offset of the first.
func readChecked(f *os.File, sumsPath string, off int64, length uint32) ([]byte, int64, error) {
	// Find the chunks, stopping at the end of the pack.
	info, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	start := off / sumChunk * sumChunk
	end := min(off+int64(length), info.Size())
	if end <= start {
		return nil, start, nil
	}
	end = min((end+sumChunk-1)/sumChunk*sumChunk, info.Size())

	// Read them and the sums that cover them.
	data := make([]byte, end-start)
	if _, err := f.ReadAt(data, start); err != nil {
		return nil, 0, err
	}
	sums, err := os.ReadFile(sumsPath) // #nosec G304 -- derived from a validated id
	if err != nil {
		return nil, 0, err
	}

	// Check each chunk.
	for i := int64(0); i < int64(len(data)); i += sumChunk {
		at := (start + i) / sumChunk * sumSize
		if at+sumSize > int64(len(sums)) {
			return nil, 0, errors.New("pack longer than its sums")
		}
		chunk := data[i:min(i+sumChunk, int64(len(data)))]
		if crc32.Checksum(chunk, sumTable) != binary.BigEndian.Uint32(sums[at:]) {
			return nil, 0, errors.New("chunk does not match its sum")
		}
	}
	return data, start, nil
}

// drop removes pack id's copy.
func (p *PackFiles) drop(id string) {
	_ = os.Remove(p.path(id))
	_ = os.Remove(p.sumsPath(id))
}

// Keep stores data as pack id, a pack the node just uploaded. A failure only
// costs a later fill.
func (p *PackFiles) Keep(id string, data []byte) {
	// A read copy gives way to the pushes.
	if !p.hasRoom() {
		return
	}

	// Store it.
	if err := p.store(id, bytes.NewReader(data)); err != nil {
		p.le.WithError(err).WithField("pack", id).Warn("unable to keep a pack")
	}
}

// Run fills queued packs until ctx ends.
func (p *PackFiles) Run(ctx context.Context) {
	for {
		// Take the next pack.
		var id string
		select {
		case <-ctx.Done():
			return
		case id = <-p.fills:
		}

		// Fill it. A failure is asked for again by the next read.
		if err := p.Fill(ctx, id); err != nil && ctx.Err() == nil {
			p.le.WithError(err).WithField("pack", id).Warn("unable to fill a pack")
		}
		p.mtx.Lock()
		delete(p.queued, id)
		p.mtx.Unlock()
	}
}

// Fill copies pack id from the remote, unless the node holds it.
func (p *PackFiles) Fill(ctx context.Context, id string) error {
	// A pack is whole once it has its name, and a read copy gives way to the
	// pushes.
	if _, err := os.Stat(p.path(id)); err == nil || !p.hasRoom() {
		return nil
	}

	// Stream it to disk.
	body, err := p.remote.GetObject(ctx, p.bucket, path.Join(p.prefix, id))
	if err != nil {
		return err
	}
	defer body.Close()
	return p.store(id, body)
}

// queueFill asks for pack id unless it waits already.
func (p *PackFiles) queueFill(id string) {
	// Mark it, and leave the mark off when the queue is full.
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if _, ok := p.queued[id]; ok {
		return
	}
	select {
	case p.fills <- id:
		p.queued[id] = struct{}{}
	default:
	}
}

// store writes pack id from src and names it once it is whole, its sums first.
// A reader sees a whole pack with its sums, or none.
func (p *PackFiles) store(id string, src io.Reader) error {
	// Write a temporary file beside it, summing each chunk as it goes.
	f, err := os.CreateTemp(p.dir, id+".*"+strconv.Itoa(os.Getpid()))
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	sums := &chunkSums{}
	_, err = io.Copy(io.MultiWriter(f, sums), src)

	// Flush it before naming it.
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}

	// Name the sums, then the pack.
	if err := writeDurable(p.sumsPath(id), sums.finish()); err != nil {
		return err
	}
	return os.Rename(tmp, p.path(id))
}

// chunkSums takes the sum of each sumChunk bytes written to it.
type chunkSums struct {
	sums []byte
	sum  uint32
	n    int64
}

// Write adds data to the running chunk.
func (c *chunkSums) Write(data []byte) (int, error) {
	// Take what fits the chunk at a time.
	written := len(data)
	for len(data) != 0 {
		part := data[:min(int64(len(data)), sumChunk-c.n)]
		c.sum = crc32.Update(c.sum, sumTable, part)
		c.n += int64(len(part))
		data = data[len(part):]

		// A full chunk records its sum.
		if c.n == sumChunk {
			c.finish()
		}
	}
	return written, nil
}

// finish records the partial chunk and returns the sums.
func (c *chunkSums) finish() []byte {
	if c.n != 0 {
		c.sums = binary.BigEndian.AppendUint32(c.sums, c.sum)
		c.sum, c.n = 0, 0
	}
	return c.sums
}

// Backfill fills every pack names yields, one at a time, until ctx ends. It is
// background work: a failed pack is left for a later read to ask for.
func (p *PackFiles) Backfill(ctx context.Context, list func(fn func(id string) error) error) error {
	return list(func(id string) error {
		// Stop for ctx, and skip what a failed fill left.
		if err := ctx.Err(); err != nil {
			return err
		}
		if identity.ValidatePackID(id) != nil {
			return nil
		}
		if err := p.Fill(ctx, id); err != nil {
			p.le.WithError(err).WithField("pack", id).Warn("unable to backfill a pack")
		}
		return nil
	})
}

// sumsPath returns the file of pack id's chunk sums.
func (p *PackFiles) sumsPath(id string) string {
	return filepath.Join(p.dir, id+sumsSuffix)
}

// path returns pack id's file.
func (p *PackFiles) path(id string) string {
	return filepath.Join(p.dir, id)
}
