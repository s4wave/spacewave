package provider_local

import (
	"context"
	"sync/atomic"

	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/net/hash"
)

// EvictBudgetFunc returns the eviction budget in force, read at every batch
// of a bounded pass; false means none, and nothing drops.
type EvictBudgetFunc func() (block_store_writeback.EvictBudget, bool)

// evictBounded is set once SetEvictBudget is called, and never cleared: from
// then on every eviction pass of the process is bounded, and a pass without a
// budget drops nothing.
var evictBounded atomic.Bool

// evictBudget holds the budget source SetEvictBudget set, or nil.
var evictBudget atomic.Pointer[EvictBudgetFunc]

// SetEvictBudget makes every eviction pass of the process bounded, as
// block_store_writeback.Store.EvictBounded describes, reading its budget from
// budget at each batch. It is a construction option: call it before the
// provider mounts a placed store, so no pass runs unbounded. Once called, the
// unbounded queue-order pass cannot run in the process: a nil budget, a
// budget that returns false, or a placed store whose backend cannot fetch and
// verify its packs drops nothing. Calling it again replaces the source; the
// bound stays.
//
// The unbounded pass remains for processes that never call it: Spacewave's
// own local accounts whose placements set a resident budget.
func SetEvictBudget(budget EvictBudgetFunc) {
	evictBounded.Store(true)
	if budget == nil {
		evictBudget.Store(nil)
		return
	}
	evictBudget.Store(&budget)
}

// EvictIsBounded reports whether SetEvictBudget was called, so no eviction
// pass of the process runs unbounded.
func EvictIsBounded() bool {
	return evictBounded.Load()
}

// readEvictBudget reads the budget in force.
func readEvictBudget() (block_store_writeback.EvictBudget, bool) {
	fn := evictBudget.Load()
	if fn == nil {
		return block_store_writeback.EvictBudget{}, false
	}
	return (*fn)()
}

// packVerifyStore is a backend store that can fetch and verify its packs:
// the S3 pack store.
type packVerifyStore interface {
	// EvictLocate names the pack the local index places each block in.
	EvictLocate(ctx context.Context, hashes []*hash.Hash) ([]block_store_s3.PackLocation, error)
	// VerifyPack fetches pack id whole from the bucket and verifies it.
	VerifyPack(ctx context.Context, id string, size int64) ([]block_store_s3.VerifiedValue, error)
	// PackIdentity names the bucket objects of pack id without their bodies.
	PackIdentity(ctx context.Context, id string) (string, error)
	// EvictHalt names a path by which the store itself could remove a pack
	// a pass verified, or returns "".
	EvictHalt() string
}

// packVerifier adapts a packVerifyStore to the writeback verifier.
type packVerifier struct {
	store packVerifyStore
}

// LocatePacks names the pack the local index places each block in.
func (v packVerifier) LocatePacks(ctx context.Context, hashes []*hash.Hash) ([]block_store_writeback.EvictPackLocation, error) {
	// Convert the store's locations.
	locs, err := v.store.EvictLocate(ctx, hashes)
	if err != nil {
		return nil, err
	}
	out := make([]block_store_writeback.EvictPackLocation, len(locs))
	for i, loc := range locs {
		out[i] = block_store_writeback.EvictPackLocation{ID: loc.ID, Size: loc.Size}
	}
	return out, nil
}

// VerifyPack fetches and verifies a pack. A canceled pass ends with its
// context's error, not a refused pack.
func (v packVerifier) VerifyPack(ctx context.Context, id string, size int64) ([]block_store_writeback.EvictVerifiedValue, string, error) {
	// End the pass when canceled, and give a failed pack's reason.
	values, err := v.store.VerifyPack(ctx, id, size)
	if ctx.Err() != nil {
		return nil, "", ctx.Err()
	}
	var failed *block_store_s3.PackVerifyError
	if errors.As(err, &failed) {
		return nil, failed.Reason, nil
	}
	if err != nil {
		return nil, "", err
	}

	// Convert the verified values.
	out := make([]block_store_writeback.EvictVerifiedValue, len(values))
	for i, value := range values {
		out[i] = block_store_writeback.EvictVerifiedValue{Hash: value.Hash, Size: value.Size}
	}
	return out, "", nil
}

// PackIdentity names the pack's bucket objects.
func (v packVerifier) PackIdentity(ctx context.Context, id string) (string, error) {
	return v.store.PackIdentity(ctx, id)
}

// evictBound returns the bound of a pass to remote, and false when remote
// cannot fetch and verify its packs, so a bounded process drops nothing. A
// pass under the bound drops nothing while remote could delete its own packs:
// by compaction and reclaim, or by the pack sweep while it holds a pack. A
// remote delete that runs outside the store, through its own object client,
// is not visible here and must be off for the run by configuration.
func evictBound(remote backendStore) (block_store_writeback.EvictBound, bool) {
	store, ok := remote.(packVerifyStore)
	if !ok {
		return block_store_writeback.EvictBound{}, false
	}
	return block_store_writeback.EvictBound{
		Budget:        readEvictBudget,
		Verifier:      packVerifier{store: store},
		RemoteDeletes: store.EvictHalt,
	}, true
}
