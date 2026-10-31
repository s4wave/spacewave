package provider_local

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/net/hash"
)

// readSetMax bounds the blocks a read set records in one generation. Reads
// past it go unrecorded, so their blocks age as if unread. A key takes 16
// bytes, so a full generation holds 64 MiB of keys.
const readSetMax = 1 << 22

// readKeepAge is how long a read keeps its block on the disk, at least: a block
// read within the last hour is not evicted to the backend.
const readKeepAge = time.Hour

// readSaveInterval is how often the reads recorded since the last save are
// saved, which bounds the reads a restart forgets.
const readSaveInterval = time.Minute

// readSaveChunk bounds the keys one saved chunk holds.
const readSaveChunk = 1 << 16

// readKeyLen is the length of a read key: a prefix of the block hash. A key
// shared by two blocks only keeps the other on the disk too.
const readKeyLen = 16

// readKey identifies a read block by a prefix of its hash.
type readKey [readKeyLen]byte

// newReadKey returns the read key of the block with hash h.
func newReadKey(h *hash.Hash) readKey {
	var key readKey
	copy(key[:], h.GetHash())
	return key
}

// readStore saves the read generations, so they outlive a restart.
type readStore interface {
	// SaveReads saves one chunk of reads to the generation started at
	// generation.
	SaveReads(ctx context.Context, generation time.Time, keys []byte) error
	// LoadReads returns the saved chunks of reads, oldest generation first.
	LoadReads(ctx context.Context) ([]block_store_writeback.ReadChunk, error)
	// DropReadsBefore deletes the saved reads of the generations started
	// before generation.
	DropReadsBefore(ctx context.Context, generation time.Time) error
}

// readSet records the blocks read from the local store, which eviction keeps
// for at least readKeepAge after the read. It holds two generations: the
// reads since the last turn and the ones before it. With a store, it saves
// both, so a restart keeps the blocks read before it.
type readSet struct {
	// enabled is set while the placement has a resident budget, before the
	// budget reaches the store, so reads after it are recorded.
	enabled atomic.Bool
	// store saves the generations, nil to keep them only in memory.
	store readStore
	// clock returns the current time, time.Now when nil.
	clock func() time.Time
	// mtx guards the fields below.
	mtx sync.Mutex
	// hashes holds the keys of the blocks read since turned.
	hashes map[readKey]struct{}
	// prev holds the keys of the generation before.
	prev map[readKey]struct{}
	// pending holds the keys added to hashes since the last save.
	pending []readKey
	// turned is when hashes started, zero before the first read or pass.
	turned time.Time
	// prevTurned is when prev started, zero without one.
	prevTurned time.Time
}

// now returns the current time on the set's clock.
func (r *readSet) now() time.Time {
	if r.clock != nil {
		return r.clock()
	}
	return time.Now()
}

// add records a read of the block with hash h.
func (r *readSet) add(h *hash.Hash) {
	// Record nothing without a budget.
	if !r.enabled.Load() {
		return
	}

	// Record the key while the set has room, starting the generation at the
	// first read.
	key := newReadKey(h)
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if r.hashes == nil {
		r.hashes = make(map[readKey]struct{})
	}
	if r.turned.IsZero() {
		r.turned = r.now()
	}
	if _, ok := r.hashes[key]; ok || len(r.hashes) >= readSetMax {
		return
	}
	r.hashes[key] = struct{}{}
	r.pending = append(r.pending, key)
}

// load restores the saved generations into an empty set and drops the older
// ones. The newest saved generation is current and the one before it previous;
// the next pass turns them when they have aged.
func (r *readSet) load(ctx context.Context, store readStore) error {
	// Read the saved chunks.
	r.store = store
	chunks, err := store.LoadReads(ctx)
	if err != nil {
		return err
	}

	// Fill the two newest generations from the newest chunks back.
	r.mtx.Lock()
	defer r.mtx.Unlock()
	for _, chunk := range slices.Backward(chunks) {

		var gen map[readKey]struct{}
		switch {
		case r.turned.IsZero() || chunk.Generation.Equal(r.turned):
			if r.hashes == nil {
				r.turned, r.hashes = chunk.Generation, make(map[readKey]struct{})
			}
			gen = r.hashes
		case r.prevTurned.IsZero() || chunk.Generation.Equal(r.prevTurned):
			if r.prev == nil {
				r.prevTurned, r.prev = chunk.Generation, make(map[readKey]struct{})
			}
			gen = r.prev
		}
		if gen == nil {
			break
		}
		for keys := chunk.Keys; len(keys) >= readKeyLen; keys = keys[readKeyLen:] {
			gen[readKey(keys[:readKeyLen])] = struct{}{}
		}
	}

	// Drop the saved generations older than the two loaded.
	if r.prevTurned.IsZero() {
		return nil
	}
	return store.DropReadsBefore(ctx, r.prevTurned)
}

// save saves the reads recorded since the last save to the current
// generation. Without a store it does nothing.
func (r *readSet) save(ctx context.Context) error {
	// Take the unsaved keys.
	if r.store == nil {
		return nil
	}
	r.mtx.Lock()
	pending, generation := r.pending, r.turned
	r.pending = nil
	r.mtx.Unlock()

	// Save them, returning the unsaved ones to the set on failure.
	unsaved, err := r.saveKeys(ctx, generation, pending)
	if err != nil {
		r.mtx.Lock()
		if r.turned.Equal(generation) {
			r.pending = append(unsaved, r.pending...)
		}
		r.mtx.Unlock()
	}
	return err
}

// saveKeys saves keys to the generation in chunks, returning the keys left
// unsaved by an error.
func (r *readSet) saveKeys(ctx context.Context, generation time.Time, keys []readKey) ([]readKey, error) {
	for len(keys) != 0 {
		n := min(len(keys), readSaveChunk)
		chunk := make([]byte, 0, n*readKeyLen)
		for _, key := range keys[:n] {
			chunk = append(chunk, key[:]...)
		}
		if err := r.store.SaveReads(ctx, generation, chunk); err != nil {
			return keys, err
		}
		keys = keys[n:]
	}
	return nil, nil
}

// take returns a keep test for an eviction pass: it keeps the blocks read in
// both generations, so every block read within readKeepAge. It turns the
// generations once the current one is readKeepAge old, saving the reads of
// the generation that ended and dropping the saved one before it.
func (r *readSet) take(ctx context.Context) (func(*hash.Hash) bool, error) {
	// Turn the generations when the current one is old enough, timing the
	// first from the first pass. A generation two ages old drops both.
	r.mtx.Lock()
	turned := false
	var ended []readKey
	var endedGen time.Time
	switch now := r.now(); {
	case r.turned.IsZero():
		r.turned = now
	case now.Sub(r.turned) >= 2*readKeepAge:
		r.prev, r.hashes, r.pending = nil, nil, nil
		r.turned, r.prevTurned, turned = now, time.Time{}, true
	case now.Sub(r.turned) >= readKeepAge:
		ended, endedGen = r.pending, r.turned
		r.prev, r.hashes, r.pending = r.hashes, nil, nil
		r.turned, r.prevTurned, turned = now, r.turned, true
	}
	dropBefore := r.prevTurned
	if dropBefore.IsZero() {
		dropBefore = r.turned
	}
	r.mtx.Unlock()

	// Save the reads of the generation that ended and the current one, then
	// drop the saved generations the turn ended.
	if r.store != nil {
		if _, err := r.saveKeys(ctx, endedGen, ended); err != nil {
			return nil, err
		}
		if err := r.save(ctx); err != nil {
			return nil, err
		}
		if turned {
			if err := r.store.DropReadsBefore(ctx, dropBefore); err != nil {
				return nil, err
			}
		}
	}

	// Keep the blocks either generation recorded.
	return func(h *hash.Hash) bool {
		// Look the key up in both generations.
		key := newReadKey(h)
		r.mtx.Lock()
		defer r.mtx.Unlock()
		_, cur := r.hashes[key]
		_, prev := r.prev[key]
		return cur || prev
	}, nil
}

// readRecorder records the blocks the local store serves to reads.
type readRecorder struct {
	block.StoreOps
	// reads records the blocks served.
	reads *readSet
}

// GetBlock reads the block and records a hit.
func (r *readRecorder) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := r.StoreOps.GetBlock(ctx, ref)
	if found {
		r.reads.add(ref.GetHash())
	}
	return data, found, err
}

// GetStoredBlock reads the block with its refs and records a hit.
func (r *readRecorder) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := r.StoreOps.GetStoredBlock(ctx, ref)
	if stored != nil {
		r.reads.add(ref.GetHash())
	}
	return stored, err
}

// setPolicy applies the cache settings of the placement. Reads are recorded
// from before the store sees a resident budget, so no read the budget pass
// should keep goes unrecorded.
func (p *placementState) setPolicy(cache *account_settings.BlockStoreCache) {
	policy := uploadPolicy(cache)
	p.reads.enabled.Store(policy.ResidentHigh > 0)
	p.wb.SetPolicy(policy)
}

// evictShortWait is how long eviction waits after a pass that left the store
// over its budget: the blocks it kept were read within readKeepAge, so a pass
// at the next upload would only queue them again.
const evictShortWait = 10 * time.Minute

// runEviction keeps the store's resident bytes under its placement's budget.
// When the resident bytes cross the high mark, a pass drops the uploaded
// blocks not read within readKeepAge, oldest queued first, down to the low
// mark. Uploads and writes change the status, which starts the next check,
// or evictShortWait after a pass that fell short of the mark. Between passes
// it saves the recorded reads every readSaveInterval.
func (p *placementState) runEviction(ctx context.Context) error {
	save := time.NewTicker(readSaveInterval)
	defer save.Stop()
	for {
		// Run a pass while the store is past its budget.
		status, changed := p.wb.GetStatus()
		var retry <-chan time.Time
		if want := status.EvictBytes(); want > 0 {
			result, err := p.evict(ctx, want)
			if err != nil {
				return err
			}
			if result.EvictedBytes < want {
				changed, retry = nil, time.After(evictShortWait)
			}
		}

		// Wait for the next status change or retry, saving the reads
		// meanwhile.
		for waiting := true; waiting; {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-changed:
				waiting = false
			case <-retry:
				waiting = false
			case <-save.C:
				if err := p.reads.save(ctx); err != nil {
					return err
				}
			}
		}
	}
}

// evict drops up to maxBytes of uploaded local values that the backend holds,
// oldest queued first, keeping the blocks read within readKeepAge. A storage
// move holds passes off through the gate. Returns a zero result without a
// storage backend.
func (p *placementState) evict(ctx context.Context, maxBytes int64) (block_store_writeback.EvictResult, error) {
	// Wait out a storage move.
	var result block_store_writeback.EvictResult
	release, err := p.evictGate.Lock(ctx)
	if err != nil {
		return result, err
	}
	defer release()

	// Evict to the open backend, paced by the account's admit.
	remote := p.remote.Load()
	if remote == nil {
		return result, nil
	}
	admit := func(ctx context.Context, lastBytes int64) error {
		if p.evictAdmit == nil {
			return nil
		}
		if fn := p.evictAdmit.Load(); fn != nil {
			return (*fn)(ctx, lastBytes)
		}
		return nil
	}
	keep, err := p.reads.take(ctx)
	if err != nil {
		return result, err
	}
	return p.wb.Evict(ctx, remote.store, maxBytes, keep, admit)
}

// SetEvictAdmit paces the eviction passes of the account's placed block
// stores: each pass calls admit before each batch, as EvictAdmit describes. A
// nil admit lets passes run unpaced. Running passes see the change at their
// next batch.
func (a *ProviderAccount) SetEvictAdmit(admit block_store_writeback.EvictAdmit) {
	if admit == nil {
		a.evictAdmit.Store(nil)
		return
	}
	a.evictAdmit.Store(&admit)
}
