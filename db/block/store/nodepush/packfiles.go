package block_store_nodepush

import (
	"context"
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

	// A range past the end comes back short, as an object store's does.
	data := make([]byte, length)
	n, err := f.ReadAt(data, off)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false
	}
	return data[:n], true
}

// Keep stores data as pack id, a pack the node just uploaded. A failure only
// costs a later fill.
func (p *PackFiles) Keep(id string, data []byte) {
	// A read copy gives way to the pushes.
	if !p.hasRoom() {
		return
	}

	// Store it.
	if err := p.store(id, func(f *os.File) error {
		_, err := f.Write(data)
		return err
	}); err != nil {
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
	return p.store(id, func(f *os.File) error {
		_, err := io.Copy(f, body)
		return err
	})
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

// store writes pack id through write and names it once it is whole. A reader
// sees a whole pack or none, since a torn pack would read as valid ranges.
func (p *PackFiles) store(id string, write func(*os.File) error) error {
	// Write a temporary file beside it.
	f, err := os.CreateTemp(p.dir, id+".*"+strconv.Itoa(os.Getpid()))
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)

	// Flush it before naming it.
	err = write(f)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, p.path(id))
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

// path returns pack id's file.
func (p *PackFiles) path(id string) string {
	return filepath.Join(p.dir, id)
}
