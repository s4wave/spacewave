package block_store_nodepush

import (
	"bytes"
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/sirupsen/logrus"
)

// fillQueue bounds the packs waiting to fill. A miss past it is forgotten, and
// asked for again by the next read of the pack.
const fillQueue = 1024

// maxReadBytes bounds one read, so a read's answer never holds up the answers
// commits wait for on the same connection. A larger block reads the remote.
const maxReadBytes = 1 << 20

// remoteReadWait bounds the node's range read of a pack it lacks. It is longer
// than the wait a local store gives the node, so the node never gives up on a
// read someone still waits for; by then the local store reads the remote
// itself.
const remoteReadWait = 5 * time.Second

// defaultRemoteReads is how many range reads of the remote run at once when
// PackFilesOptions sets no bound.
const defaultRemoteReads = 2

// Source reads packs from the remote.
type Source interface {
	// GetObject opens the object's body. The caller must Close it.
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	// GetObjectRange reads length bytes of the object from off, shorter when
	// the object ends first.
	GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error)
}

// PackFilesOptions configures a PackFiles.
type PackFilesOptions struct {
	// RemoteReads bounds the range reads of the remote that run at once for
	// packs the node lacks. A read past it misses at once, and the local store
	// reads the remote itself. Zero uses defaultRemoteReads.
	RemoteReads int
	// ListPath is the SQLite file listing the read copies. Empty puts it
	// beside the directory, as the directory's name plus ".db".
	ListPath string
}

// PackFilesStats counts what a PackFiles holds and has served.
type PackFilesStats struct {
	// Copies counts the read copies held.
	Copies int64
	// CopyBytes sums their sizes.
	CopyBytes int64
	// CopyReads counts reads served from a read copy.
	CopyReads uint64
	// RemoteReads counts reads of a pack the node lacked, served by a range
	// read of the remote.
	RemoteReads uint64
	// Misses counts reads answered with nothing: no remote read slot was free,
	// or the remote read failed.
	Misses uint64
	// Kept counts the packs kept as read copies once the node uploaded them.
	Kept uint64
	// Filled counts the packs fetched whole into read copies.
	Filled uint64
	// FilledBytes sums their sizes.
	FilledBytes uint64
	// FillsFailed counts the fetches that failed.
	FillsFailed uint64
	// Pruned counts the read copies pruned for disk space.
	Pruned uint64
	// PrunedBytes sums their sizes.
	PrunedBytes uint64
}

// PackFiles keeps read copies of whole packs on the node's disk and serves byte
// ranges of them. It holds no table of blocks: the local store's index already
// knows which pack and offset it wants, so the node's memory doesn't grow with
// the store.
//
// A read of a pack it lacks is a range read of the remote, answered as soon as
// those bytes arrive, and queues the whole pack's fetch in the background, one
// fetch per pack however many reads miss on it. So the first read of a pack
// waits for its own bytes only, and the later ones read the copy.
//
// Invariants:
//   - A read copy is a byte-identical copy of a pack the remote held when it
//     was stored, under the remote's pack ID: a fill copies the remote's
//     object as it is, and Keep stores a pack only after the remote took it.
//     So nothing a reader needs is in a read copy alone, and dropping one costs
//     only a later read of the remote.
//   - Read copies never reach the pack log or its upload: PackFiles writes only
//     its own directory and holds no handle to the log. A copy is never
//     repacked or uploaded again.
//   - Prune removes only read copies, files of this directory named in the
//     list, never a file of the pack log, which keeps its own directory.
type PackFiles struct {
	// le logs the failed fills and stores.
	le *logrus.Entry
	// dir holds the read copies, each named by its pack ID.
	dir string
	// remote reads the packs on the remote.
	remote Source
	// bucket is the remote's bucket of the packs.
	bucket string
	// prefix is the object key prefix of the packs.
	prefix string
	// list lists the read copies held.
	list *packList
	// fills queues the packs reads missed on, filled before any backfill.
	fills chan string
	// backfills hands the backfill's next pack to the filler.
	backfills chan backfillPack
	// remoteSlots bounds the range reads of the remote in flight.
	remoteSlots chan struct{}
	// room reports whether the disk can take another pack, or is nil.
	room atomic.Pointer[func() bool]
	// stats counts reads, fills and prunes.
	stats packFilesCounters

	// mtx guards queued.
	mtx sync.Mutex
	// queued holds the packs in fills or being filled, so a pack queues once.
	queued map[string]struct{}
}

// backfillPack is a pack the backfill hands the filler, with the channel the
// filler closes once the pack is filled or failed.
type backfillPack struct {
	// id is the pack.
	id string
	// done closes when the fill ends.
	done chan struct{}
}

// packFilesCounters are the running counts of PackFilesStats.
type packFilesCounters struct {
	// copyReads counts reads served from a read copy.
	copyReads atomic.Uint64
	// remoteReads counts reads served by a range read of the remote.
	remoteReads atomic.Uint64
	// misses counts reads answered with nothing.
	misses atomic.Uint64
	// kept counts the uploaded packs kept.
	kept atomic.Uint64
	// filled counts the packs fetched whole.
	filled atomic.Uint64
	// filledBytes sums their sizes.
	filledBytes atomic.Uint64
	// fillsFailed counts the failed fetches.
	fillsFailed atomic.Uint64
	// pruned counts the copies pruned.
	pruned atomic.Uint64
	// prunedBytes sums their sizes.
	prunedBytes atomic.Uint64
}

// NewPackFiles keeps read copies in dir, filling them from remote's objects
// named prefix plus the pack ID in bucket. Close releases its list.
func NewPackFiles(ctx context.Context, le *logrus.Entry, dir string, remote Source, bucket, prefix string, opts PackFilesOptions) (*PackFiles, error) {
	// Open the directory and its list.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	listPath := opts.ListPath
	if listPath == "" {
		listPath = filepath.Clean(dir) + ".db"
	}
	list, err := openPackList(ctx, listPath, dir)
	if err != nil {
		return nil, err
	}

	// Bound the remote reads.
	remoteReads := opts.RemoteReads
	if remoteReads <= 0 {
		remoteReads = defaultRemoteReads
	}
	return &PackFiles{
		le:          le,
		dir:         dir,
		remote:      remote,
		bucket:      bucket,
		prefix:      prefix,
		list:        list,
		fills:       make(chan string, fillQueue),
		backfills:   make(chan backfillPack),
		remoteSlots: make(chan struct{}, remoteReads),
		queued:      make(map[string]struct{}),
	}, nil
}

// Close closes the list. Call it after Run returns.
func (p *PackFiles) Close() error {
	return p.list.close()
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

// Stats returns the counts.
func (p *PackFiles) Stats() PackFilesStats {
	return PackFilesStats{
		Copies:      p.list.count.Load(),
		CopyBytes:   p.list.bytes.Load(),
		CopyReads:   p.stats.copyReads.Load(),
		RemoteReads: p.stats.remoteReads.Load(),
		Misses:      p.stats.misses.Load(),
		Kept:        p.stats.kept.Load(),
		Filled:      p.stats.filled.Load(),
		FilledBytes: p.stats.filledBytes.Load(),
		FillsFailed: p.stats.fillsFailed.Load(),
		Pruned:      p.stats.pruned.Load(),
		PrunedBytes: p.stats.prunedBytes.Load(),
	}
}

// ReadRange reads a range of pack id: from its read copy, or, lacking one,
// from the remote while the pack's fetch is queued.
func (p *PackFiles) ReadRange(ctx context.Context, id string, off int64, length uint32) ([]byte, bool) {
	// Only a valid name reaches the disk or the remote: it comes off the wire.
	if identity.ValidatePackID(id) != nil || off < 0 || length > maxReadBytes {
		return nil, false
	}

	// Serve the read copy.
	if data, ok, held := p.readCopy(id, off, length); held {
		if ok {
			p.stats.copyReads.Add(1)
		}
		return data, ok
	}

	// Lacking it, fetch the pack in the background and read the range now.
	p.queueFill(id)
	return p.readRemote(ctx, id, off, length)
}

// readCopy reads a range of the read copy of pack id. held is false when the
// node has no copy.
func (p *PackFiles) readCopy(id string, off int64, length uint32) ([]byte, bool, bool) {
	// Open the copy, which is whole once it has its name.
	f, err := os.Open(p.path(id)) // #nosec G304 -- the id was validated
	if err != nil {
		return nil, false, false
	}
	defer f.Close()

	// A range past the end comes back short, as an object store's does.
	data := make([]byte, length)
	n, err := f.ReadAt(data, off)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, true
	}
	return data[:n], true, true
}

// readRemote range-reads pack id from the remote, in a free remote read slot.
// With none free it misses at once, so a burst of misses costs the local
// store no wait: it reads the remote itself.
func (p *PackFiles) readRemote(ctx context.Context, id string, off int64, length uint32) ([]byte, bool) {
	// Take a slot or miss.
	select {
	case p.remoteSlots <- struct{}{}:
	default:
		p.stats.misses.Add(1)
		return nil, false
	}
	defer func() { <-p.remoteSlots }()

	// Read the range as someone waits for it, which is the class of an
	// untagged context.
	ctx, cancel := context.WithTimeout(ctx, remoteReadWait)
	defer cancel()
	data, err := p.remote.GetObjectRange(ctx, p.bucket, p.key(id), off, int(length))
	if err != nil {
		p.stats.misses.Add(1)
		return nil, false
	}
	p.stats.remoteReads.Add(1)
	return data, true
}

// Keep stores data as pack id, a pack the node just uploaded. A failure only
// costs a later fill.
func (p *PackFiles) Keep(id string, data []byte) {
	// A read copy gives way to the pushes.
	if !p.hasRoom() {
		return
	}

	// Store it.
	if _, err := p.store(context.Background(), id, bytes.NewReader(data)); err != nil {
		p.le.WithError(err).WithField("pack", id).Warn("unable to keep a pack")
		return
	}
	p.stats.kept.Add(1)
}

// Run fills packs until ctx ends: those reads missed on first, then the
// backfill's, one pack at a time.
func (p *PackFiles) Run(ctx context.Context) {
	for {
		// Take the next pack.
		id, done, ok := p.next(ctx)
		if !ok {
			return
		}

		// Fill it. A failure is asked for again by the next read.
		if err := p.Fill(ctx, id); err != nil && ctx.Err() == nil {
			p.stats.fillsFailed.Add(1)
			p.le.WithError(err).WithField("pack", id).Warn("unable to fill a pack")
		}

		// Let the pack queue again, or tell the backfill it is done.
		if done != nil {
			close(done)
			continue
		}
		p.mtx.Lock()
		delete(p.queued, id)
		p.mtx.Unlock()
	}
}

// next returns the next pack to fill, a missed one before a backfill's, with
// the backfill's done channel, nil for a missed pack. It reports false once
// ctx ends.
func (p *PackFiles) next(ctx context.Context) (string, chan struct{}, bool) {
	// A missed pack goes first.
	select {
	case id := <-p.fills:
		return id, nil, true
	default:
	}

	// Otherwise wait for either.
	select {
	case <-ctx.Done():
		return "", nil, false
	case id := <-p.fills:
		return id, nil, true
	case pack := <-p.backfills:
		return pack.id, pack.done, true
	}
}

// Fill copies pack id from the remote, unless the node holds it or the disk
// has no room. It is background work: the remote serves its reads after the
// ones a reader waits for.
func (p *PackFiles) Fill(ctx context.Context, id string) error {
	// A pack is whole once it has its name, and a read copy gives way to the
	// pushes.
	if p.holds(id) || !p.hasRoom() {
		return nil
	}

	// Stream it to disk as it is.
	body, err := p.remote.GetObject(block.WithBackground(ctx), p.bucket, p.key(id))
	if err != nil {
		return err
	}
	defer body.Close()
	size, err := p.store(ctx, id, body)
	if err != nil {
		return err
	}
	p.stats.filled.Add(1)
	p.stats.filledBytes.Add(uint64(size)) //nolint:gosec // a copied size is never negative
	return nil
}

// holds reports whether the node has a read copy of pack id.
func (p *PackFiles) holds(id string) bool {
	_, err := os.Stat(p.path(id))
	return err == nil
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

// store writes pack id from src, names it once it is whole and lists it, and
// returns its size. A reader sees a whole pack or none, since a torn pack
// would read as valid ranges.
func (p *PackFiles) store(ctx context.Context, id string, src io.Reader) (int64, error) {
	// Write a temporary file beside it, whose name is no pack ID.
	f, err := os.CreateTemp(p.dir, id+".*"+strconv.Itoa(os.Getpid()))
	if err != nil {
		return 0, err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	size, err := io.Copy(f, src)

	// Flush it before naming it.
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, err
	}
	if err := os.Rename(tmp, p.path(id)); err != nil {
		return 0, err
	}

	// List it.
	return size, p.list.add(ctx, id, size)
}

// Backfill fills every pack list yields that the node lacks, one at a time,
// after the packs reads missed on, and at most bytesPerSecond, until ctx
// ends. It stops once the disk has no room for read copies, and reports
// errNoRoom then. It is background work: a failed pack is left for a later
// read to ask for. Its copies are read copies like any fill's, never the log's.
func (p *PackFiles) Backfill(ctx context.Context, list func(fn func(id string, size int64) error) error, bytesPerSecond func() int64) error {
	return list(func(id string, size int64) error {
		// Stop for ctx and a full disk, and skip what is held or misnamed.
		if err := ctx.Err(); err != nil {
			return err
		}
		if !p.hasRoom() {
			return errNoRoom
		}
		if identity.ValidatePackID(id) != nil || p.holds(id) {
			return nil
		}

		// Hand it to the filler, once the missed packs are done, and wait for
		// its fill.
		pack := backfillPack{id: id, done: make(chan struct{})}
		select {
		case p.backfills <- pack:
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case <-pack.done:
		case <-ctx.Done():
			return ctx.Err()
		}

		// Pace the next by this one's size.
		if rate := bytesPerSecond(); rate > 0 && size > 0 {
			select {
			case <-time.After(time.Duration(float64(size) / float64(rate) * float64(time.Second))):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
}

// errNoRoom ends a backfill that found the disk without room.
var errNoRoom = errors.New("no room for read copies")

// Prune removes read copies, oldest stored first, while short reports that the
// disk lacks space, and returns the bytes it removed. It removes only files of
// the read copy directory, never the log's, and every copy is a copy of a pack
// the remote holds, so pruning loses nothing a reader needs.
func (p *PackFiles) Prune(ctx context.Context, short func() bool) (int64, error) {
	var pruned int64
	for short() {
		// Take the oldest copy listed.
		id, size, ok, err := p.list.oldest(ctx)
		if err != nil || !ok {
			return pruned, err
		}

		// Remove its file, then its row: a crash between leaves a row the
		// next open drops.
		if err := os.Remove(p.path(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return pruned, err
		}
		if err := p.list.remove(ctx, id, size); err != nil {
			return pruned, err
		}
		pruned += size
		p.stats.pruned.Add(1)
		p.stats.prunedBytes.Add(uint64(size)) //nolint:gosec // a listed size is never negative
	}
	return pruned, nil
}

// key returns pack id's object key on the remote.
func (p *PackFiles) key(id string) string {
	return path.Join(p.prefix, id)
}

// path returns pack id's file.
func (p *PackFiles) path(id string) string {
	return filepath.Join(p.dir, id)
}
