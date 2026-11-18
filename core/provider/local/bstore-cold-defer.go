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
type backendDefer struct {
	// StoreOps is the backend store.
	block.StoreOps
	// remote is the unscoped backend store a deferred read fetches from
	// after its read scope ends.
	remote block_store.StoreSource
}

// errBackendDeferred is the error of a backend read deferred to the caller's
// PeerReads. It is block.ErrUnavailable, so the read ends there and the tiers
// below are not asked meanwhile.
var errBackendDeferred = fmt.Errorf("%w: deferred to a backend fetch", block.ErrUnavailable)

// deferBackendReads wraps remote so that reads under block.WithPeerReads defer,
// as backendDefer describes.
func deferBackendReads(remote block_store.StoreSource) block_store.StoreSource {
	if remote == nil {
		return nil
	}
	return func() block.StoreOps {
		store := remote()
		if store == nil {
			return nil
		}
		return &backendDefer{StoreOps: store, remote: remote}
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
	return &backendDefer{StoreOps: scoped, remote: b.remote}, release, nil
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

// recordBackendRead counts a read the backend served in place since started.
func recordBackendRead(ctx context.Context, started time.Time) {
	block.RecordBackendRead(ctx, time.Since(started))
}

// fetch returns the deferred read of ref from the unscoped backend, outside
// the caller's PeerReads. A backend that is gone meanwhile misses the block.
func (b *backendDefer) fetch(ref *block.BlockRef) block.PeerFetch {
	return func(ctx context.Context) (*block.StoredBlock, error) {
		store := b.remote()
		if store == nil {
			return nil, nil
		}
		return store.GetStoredBlock(block.WithPeerReads(ctx, nil), ref)
	}
}
