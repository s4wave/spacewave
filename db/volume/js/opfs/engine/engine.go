package engine

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"io/fs"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/promise"
	"github.com/s4wave/spacewave/db/kvtx"
)

const (
	// cacheByteLimit bounds immutable encoded data retained by one volume.
	cacheByteLimit = 64 << 20
	// cacheFileLimit bounds map and recency metadata independently of bytes.
	cacheFileLimit = 512
)

// Engine owns one volume's durable publication, immutable reads, and cache.
// Instances coordinate through Backend locks; cached files never prove freshness.
type Engine struct {
	// backend owns storage and cross-runtime locks for this volume.
	backend Backend
	// workloadIDs numbers transactions, iterators, and block read scopes for
	// workload records.
	workloadIDs atomic.Uint64
	// mtx protects the lifecycle, immutable cache, and pending root read.
	mtx sync.Mutex
	// rootRead shares only an in-flight descriptor read under shared root locks.
	rootRead *promise.Promise[*Root]
	// pin serializes acquisition of the shared cross-runtime reader lease.
	pin csync.Mutex
	// readers counts active local operations sharing reclamation protection.
	readers int
	// releaseReaders releases the shared backend lock when the last reader exits.
	releaseReaders func()
	// closed rejects new work after Close.
	closed bool
	// done wakes generation waiters when the engine closes.
	done chan struct{}
	// cache indexes bounded immutable encoded files.
	cache map[string]*list.Element
	// recency orders cache entries from most to least recently used.
	recency list.List
	// wake schedules the volume-owned maintenance worker after publication.
	wake chan struct{}
	// cacheBytes charges every retained file byte.
	cacheBytes int
}

// cacheEntry records one immutable file and its byte charge.
type cacheEntry struct {
	// name identifies the immutable file.
	name string
	// data contains the immutable encoded bytes.
	data []byte
	// parsed retains a validated immutable catalogue or run.
	parsed message
	// charge includes encoded and decoded residency plus entry overhead.
	charge int
}

// Open validates the current root or creates an empty volume.
func Open(ctx context.Context, backend Backend) (_ *Engine, retErr error) {
	// Keep initialization locks and failed-open cleanup with this construction.
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, backend.Close())
		}
	}()
	e := &Engine{backend: backend, cache: make(map[string]*list.Element), done: make(chan struct{})}
	release, err := backend.Lock(ctx, "reclaim", false)
	if err != nil {
		return nil, err
	}
	defer release()
	unlock, err := backend.Lock(ctx, "publish", true)
	if err != nil {
		return nil, err
	}
	defer unlock()

	// Recover bounded unpublished output before allocating another generation.
	if err := e.cleanIntent(ctx); err != nil {
		return nil, err
	}
	_, err = e.loadRoot(ctx)
	if err == nil {
		if err := e.ensureIdentity(ctx); err != nil {
			return nil, err
		}
		return e, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	// Losing both descriptors in an initialized volume is corruption, not empty data.
	initialized, err := e.hasIdentity(ctx)
	if err != nil {
		return nil, err
	}
	if initialized {
		return nil, ErrCorrupt
	}

	// Publish the first empty partition through the normal commit protocol.
	p := newPublication(e, &Root{ReclaimNext: 1})
	page := &Catalogue{Partitions: []*Partition{{}}}
	name, err := p.add("page", page)
	if err != nil {
		return nil, err
	}
	p.root.Catalogue = name
	if err := p.commit(ctx); err != nil {
		return nil, err
	}
	if err := e.ensureIdentity(ctx); err != nil {
		return nil, err
	}
	return e, nil
}

// Close releases instance cache state and rejects subsequent operations.
func (e *Engine) Close() error {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	close(e.done)
	e.cache = nil
	e.recency.Init()
	e.cacheBytes = 0
	return e.backend.Close()
}

// readFile returns bounded immutable bytes, sharing one cache across families.
func (e *Engine) readFile(ctx context.Context, name string) ([]byte, error) {
	return e.readCached(ctx, name, name, 0, readAll)
}

// readCached charges one immutable file or payload window to the shared cache.
func (e *Engine) readCached(ctx context.Context, key, name string, offset int64, length int) ([]byte, error) {
	// Resolve resident immutable bytes while the cache remains open.
	e.mtx.Lock()
	if e.closed {
		e.mtx.Unlock()
		return nil, ErrClosed
	}
	if elem := e.cache[key]; elem != nil {
		e.recency.MoveToFront(elem)
		data := elem.Value.(*cacheEntry).data
		e.mtx.Unlock()
		return data, nil
	}
	e.mtx.Unlock()

	// Read outside the cache lock, then reconcile any concurrent insertion.
	data, err := e.backend.Read(ctx, name, offset, length)
	if err != nil {
		return nil, err
	}
	if len(data) > maxBatchBytes*2 {
		return nil, ErrCorrupt
	}
	e.mtx.Lock()
	defer e.mtx.Unlock()
	if e.closed {
		return nil, ErrClosed
	}
	return e.cacheBytesLocked(key, data), nil
}

// loadRoot validates both fixed descriptors and each candidate's immediate page.
// Deeper immutable files are validated on demand without a corpus-wide scan.
func (e *Engine) loadRoot(ctx context.Context) (*Root, error) {
	// Join both reads before returning so the caller keeps file protection
	// until all descriptor bytes are owned, including when ctx is canceled.
	first := promise.NewPromise[[]byte]()
	go func() {
		data, err := e.backend.Read(ctx, "root-0", 0, readAll)
		first.SetResult(data, err)
	}()
	secondData, secondErr := e.backend.Read(ctx, "root-1", 0, readAll)
	firstData, firstErr := first.Await(context.WithoutCancel(ctx))

	var best *Root
	var invalid error
	for slot := range 2 {
		data, err := firstData, firstErr
		if slot == 1 {
			data, err = secondData, secondErr
		}
		if errors.Is(err, fs.ErrNotExist) || (err == nil && len(data) == 0) {
			// A new slot remains empty if its first writable stream is interrupted.
			continue
		}
		root := new(Root)
		if err == nil {
			err = decode(data, root)
		}
		if err == nil && (root.Format != formatVersion || root.Generation == 0 || root.Catalogue == "" || root.ReclaimNext == 0) {
			err = ErrCorrupt
		}
		if err == nil {
			_, err = e.readCatalogue(ctx, root.Catalogue)
		}
		if err != nil {
			invalid = errors.Join(ErrCorrupt, err)
			continue
		}
		if best == nil || root.Generation > best.Generation {
			best = root
		}
	}
	if best != nil {
		return best, nil
	}
	if invalid != nil {
		return nil, invalid
	}
	return nil, fs.ErrNotExist
}

// RefreshGenerationContext reads the durable generation under file protection.
func (e *Engine) RefreshGenerationContext(ctx context.Context) (uint64, error) {
	s, err := e.snapshot(ctx)
	if err != nil {
		return 0, err
	}
	defer s.release()
	return s.root.Revision, nil
}

// Apply atomically publishes sorted-record mutations, rejecting a stale base.
// A nil base serializes blind mutations that have observed no committed state.
func (e *Engine) Apply(ctx context.Context, base *uint64, records []*Record) error {
	return e.apply(ctx, base, records, false)
}

// apply validates the caller's revision domain under the shared publication lock.
func (e *Engine) apply(ctx context.Context, base *uint64, records []*Record, metadata bool) error {
	// Serialize publication while protecting the generation's immutable files.
	if err := validateRecords(records); err != nil {
		return err
	}
	release, err := e.protect(ctx)
	if err != nil {
		return err
	}
	defer release()
	unlock, err := e.backend.Lock(ctx, "publish", true)
	if err != nil {
		return err
	}
	defer unlock()

	// Validate against the durable root after acquiring publication authority.
	root, err := e.loadRoot(ctx)
	if err != nil {
		return err
	}
	revision := root.Revision
	if metadata {
		revision = root.MetadataRevision
	}
	if base != nil && revision != *base {
		return kvtx.ErrInvalidSnapshot
	}
	if len(records) == 0 {
		return nil
	}
	// Build the next immutable generation before replacing either descriptor.
	p := newPublication(e, root)
	p.root.Revision++
	for _, record := range records {
		if len(record.Key) != 0 && record.Key[0] == metadataPrefix {
			p.root.MetadataRevision++
			break
		}
	}
	children, err := p.updateCatalogue(ctx, root.Catalogue, records)
	if err != nil {
		return err
	}
	p.root.Catalogue, err = p.finishCatalogue(children)
	if err != nil {
		return err
	}
	return p.commit(ctx)
}

// validateRecords enforces the transaction memory and ordering bounds.
func validateRecords(records []*Record) error {
	if len(records) > maxBatchRecords {
		return ErrLimit
	}
	var size int
	var previous string
	for i, record := range records {
		if record == nil || len(record.Key) > maxKeyBytes || len(record.Value) > MaxValueBytes {
			return ErrLimit
		}
		key := string(record.Key)
		if i != 0 && key <= previous {
			return errors.New("immutable volume mutations must have unique sorted keys")
		}
		previous = key
		size += len(record.Key) + len(record.Value) + 32
		if size > maxBatchBytes {
			return ErrLimit
		}
	}
	return nil
}

// hasIdentity distinguishes an initialized volume from interrupted first creation.
func (e *Engine) hasIdentity(ctx context.Context) (bool, error) {
	data, err := e.backend.Read(ctx, "identity", 0, readAll)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Only first initialization writes this entry; an empty file was never committed.
	if len(data) == 0 {
		return false, nil
	}
	if !bytes.Equal(data, []byte("immutable-opfs-3\n")) {
		return false, ErrCorrupt
	}
	return true, nil
}

// ensureIdentity makes successful initialization durable before exposing writes.
func (e *Engine) ensureIdentity(ctx context.Context) error {
	found, err := e.hasIdentity(ctx)
	if err != nil || found {
		return err
	}
	return e.backend.Write(ctx, "identity", []byte("immutable-opfs-3\n"))
}
