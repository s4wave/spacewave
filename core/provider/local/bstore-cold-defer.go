package provider_local

import (
	"context"
	"fmt"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
)

// backendDefer is the storage backend's place in a placed store's reads. A
// read reaches it only after the local store and the fill cache missed, so the
// backend must fetch the block over the network.
//
// Under block.WithPeerReads the read fetches nothing: it answers a block the
// PeerReads fetched from the backend, passes a block the backend lacks on to
// the tiers below, and otherwise defers the block to the PeerReads with
// DeferUpper and returns errBackendDeferred at once, since its caller may hold
// the World's writer. The fill cache over the backend then fills with only
// what the backend served. Any other read passes through to the backend
// unchanged, and is counted in the context's block.ReadCounter.
//
// A paused backend makes a deferred fetch wait instead of failing, so with a
// read fallback the fetch races the fallback's verified read while the
// backend is paused, as raceAlternates describes. A block the fallback holds
// ends the fetch with errBackendPaused, a failed fetch that says nothing of
// the backend. A fallback miss never ends the fetch, which keeps waiting for
// the backend.
//
// A read under PeerReads that finds the backend paused reads the fallback
// itself, verified, and on a hit defers nothing: it fails with fallbackServed,
// which carries the block to the fallback tier above, and that tier serves
// it. A fallback miss, failure or corrupt row defers the block as usual, so it
// waits for the backend. So the caller's attempt after a fetch
// the fallback won completes with nothing pending, and an attempt during a
// pause never fetches a block the fallback holds.
//
// Invariant: everything backendDefer returns as found, in place or from
// LookupUpper, the backend served. The fill cache and the backend's fill
// above it store what backendDefer returns as the backend's copy, without an
// upload marker, so a block the fallback served must never pass through here
// as found; it reaches the caller only through the fallback tier, whose fill
// marks it for upload.
type backendDefer struct {
	// StoreOps is the backend store.
	block.StoreOps
	// remote is the unscoped backend store a deferred read fetches from
	// after its read scope ends.
	remote block_store.StoreSource
	// pause watches the backend's pause, or holds nil.
	pause backendPause
	// fallback is the read fallback a deferred fetch races, or nil.
	fallback block.StoreOps
}

// errBackendDeferred is the error of a backend read deferred to the caller's
// PeerReads. It is block.ErrUnavailable, so the read ends there and the tiers
// below are not asked meanwhile.
var errBackendDeferred = fmt.Errorf("%w: deferred to a backend fetch", block.ErrUnavailable)

// errBackendPaused is the error of a deferred backend fetch that ended while
// the backend was paused because the read fallback holds the block.
var errBackendPaused = fmt.Errorf("%w: backend paused; the read fallback holds the block", block.ErrUnavailable)

// deferBackendReads wraps remote so that reads under block.WithPeerReads defer,
// as backendDefer describes, their fetches racing fallback, when not nil,
// while pause reports the backend paused.
func deferBackendReads(remote block_store.StoreSource, pause backendPause, fallback block.StoreOps) block_store.StoreSource {
	if remote == nil {
		return nil
	}
	if fallback == nil {
		pause = nil
	}
	return func() block.StoreOps {
		store := remote()
		if store == nil {
			return nil
		}
		return &backendDefer{StoreOps: store, remote: remote, pause: pause, fallback: fallback}
	}
}

// BeginReadOperation opens a read scope on the backend that keeps the
// deferral. The embedded store's own method would return the bare backend
// scope.
func (b *backendDefer) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	scoped, release, err := b.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &backendDefer{StoreOps: scoped, remote: b.remote, pause: b.pause, fallback: b.fallback}, release, nil
}

// GetBlock reads the block, deferring it as GetStoredBlock does.
func (b *backendDefer) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if block.GetPeerReads(ctx) == nil {
		defer recordBackendRead(ctx, time.Now())
		return b.StoreOps.GetBlock(ctx, ref)
	}
	stored, err := b.GetStoredBlock(ctx, ref)
	if stored == nil || err != nil {
		return nil, false, err
	}
	return stored.Data, true, nil
}

// GetStoredBlock reads the block and its refs from the backend, or defers it
// under block.WithPeerReads.
func (b *backendDefer) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Read in place outside PeerReads.
	reads := block.GetPeerReads(ctx)
	if reads == nil {
		defer recordBackendRead(ctx, time.Now())
		return b.StoreOps.GetStoredBlock(ctx, ref)
	}

	// Answer from an earlier round's fetch, or defer the block.
	if reads.Missed(ref) {
		return nil, nil
	}
	if stored := reads.LookupUpper(ref); stored != nil {
		return stored, nil
	}

	// Hand a block the fallback holds, read and verified, to the fallback
	// tier while the backend is paused, deferring nothing, so the caller's
	// attempt completes.
	if stored := b.fallbackRead(ctx, ref); stored != nil {
		return nil, &fallbackServed{ref: ref.MarshalString(), stored: stored}
	}

	// Defer the block to a fetch outside the caller's lock.
	reads.DeferUpper(ref, b.fetch(ref))
	return nil, fmt.Errorf("%w: block %s", errBackendDeferred, ref.MarshalString())
}

// StatBlock stats the block, deferring it as GetStoredBlock does.
func (b *backendDefer) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	if block.GetPeerReads(ctx) == nil {
		defer recordBackendRead(ctx, time.Now())
		return b.StoreOps.StatBlock(ctx, ref)
	}
	stored, err := b.GetStoredBlock(ctx, ref)
	if stored == nil || err != nil {
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: int64(len(stored.Data))}, nil
}

// fallbackRead reads ref from the read fallback while the backend is paused,
// verified as fallbackHit verifies it. It returns nil when the backend is not
// paused, or when the fallback misses, fails or holds bytes that do not hash
// to ref: the caller then defers the block, so a broken fallback row waits
// for the backend rather than failing the read.
func (b *backendDefer) fallbackRead(ctx context.Context, ref *block.BlockRef) *block.StoredBlock {
	// Leave the fallback alone unless the backend is paused.
	if b.pause == nil {
		return nil
	}
	if paused, _ := b.pause(); !paused {
		return nil
	}

	// Read and verify the fallback's block, outside the PeerReads.
	stored, err := readVerified(block.WithPeerReads(ctx, nil), b.fallback, ref)
	if err != nil {
		return nil
	}
	return stored
}

// fallbackServed is the error of a backend read under PeerReads that the
// paused backend left to the read fallback. It carries the block the fallback
// served, verified, for the fallback tier above, which serves and fills it as
// its own hit, so the attempt reads the fallback once. It is errBackendPaused
// to every other tier.
type fallbackServed struct {
	// ref is the marshaled ref of the block.
	ref string
	// stored is the block the fallback served.
	stored *block.StoredBlock
}

// Error returns the error text.
func (e *fallbackServed) Error() string {
	return errBackendPaused.Error() + ": block " + e.ref
}

// Unwrap returns errBackendPaused.
func (e *fallbackServed) Unwrap() error {
	return errBackendPaused
}

// recordBackendRead counts a read the backend served in place since started.
func recordBackendRead(ctx context.Context, started time.Time) {
	block.RecordBackendRead(ctx, time.Since(started))
}

// fetch returns the deferred read of ref from the unscoped backend, outside
// the caller's PeerReads. A backend that is gone meanwhile misses the block. A
// fetch the backend's pause holds while the fallback confirms the block ends
// with errBackendPaused.
func (b *backendDefer) fetch(ref *block.BlockRef) block.PeerFetch {
	return func(ctx context.Context) (*block.StoredBlock, error) {
		// Fetch from the unscoped backend, outside the PeerReads.
		store := b.remote()
		if store == nil {
			return nil, nil
		}
		ctx = block.WithPeerReads(ctx, nil)

		// Race the backend against the fallback's verified read, which only
		// a block that hashes to ref may win.
		var fallback []func(context.Context) (bool, error)
		if b.fallback != nil {
			fallback = append(fallback, func(ctx context.Context) (bool, error) {
				stored, err := readVerified(ctx, b.fallback, ref)
				return stored != nil, err
			})
		}
		var stored *block.StoredBlock
		res := raceAlternates(ctx, b.pause, func(ctx context.Context) (bool, error) {
			var err error
			stored, err = store.GetStoredBlock(ctx, ref)
			return stored != nil, err
		}, existsHit, fallback...)

		// A fallback hit ends the fetch with errBackendPaused, so the next
		// attempt reads the fallback, and never stands for the backend.
		if res.fromAlternate {
			return nil, fmt.Errorf("%w: block %s", errBackendPaused, ref.MarshalString())
		}
		return stored, res.err
	}
}
