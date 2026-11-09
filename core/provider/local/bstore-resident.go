package provider_local

import (
	"sync"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
)

// ResidentMarks override the resident marks the placements of an account's
// block stores set, while the process runs: SetResidentMarks applies them to
// every mounted placed store at once, and to each store mounted after. The
// placements in the account settings stay as they are, so clearing the marks
// restores them and a restart starts from them. A zero field keeps the
// placement's value.
type ResidentMarks struct {
	// HighBytes is the resident byte budget: past it, eviction drops the
	// uploaded blocks read least recently. Without LowBytes the low mark is
	// nine tenths of it, as a placement's is.
	HighBytes int64
	// LowBytes is where eviction stops. One past the high mark is nine
	// tenths of the high mark.
	LowBytes int64
	// MaxPendingBytes uploads the oldest queued blocks before they settle
	// while the queued bytes exceed it.
	MaxPendingBytes int64
}

// apply returns policy with the marks set over it.
func (m ResidentMarks) apply(policy block_store_writeback.Policy) block_store_writeback.Policy {
	if m.HighBytes > 0 {
		policy.ResidentHigh, policy.ResidentLow = m.HighBytes, m.HighBytes/10*9
	}
	if m.LowBytes > 0 {
		policy.ResidentLow = m.LowBytes
	}
	if policy.ResidentLow > policy.ResidentHigh {
		policy.ResidentLow = policy.ResidentHigh / 10 * 9
	}
	if m.MaxPendingBytes > 0 {
		policy.MaxPendingBytes = m.MaxPendingBytes
	}
	return policy
}

// residentOverride holds an account's resident marks and the cache settings
// of each mounted placed store they apply over.
type residentOverride struct {
	// mtx guards the fields below and orders the policy writes, so the last
	// marks set are the ones in force.
	mtx sync.Mutex
	// marks are the marks SetResidentMarks set.
	marks ResidentMarks
	// placements maps each mounted placed store to its placement's cache
	// settings.
	placements map[*placementState]*account_settings.BlockStoreCache
}

// SetResidentMarks overrides the resident marks of the account's placed block
// stores, as ResidentMarks describes. A store running an eviction pass
// finishes it at the marks it started under, then checks the new ones; a
// store waiting between passes checks them at once. A pass that drops many
// bytes still drops them in batches the store's batch bounds keep short, so
// lowering the high mark far holds the store's writer no longer at a time.
func (a *ProviderAccount) SetResidentMarks(marks ResidentMarks) {
	r := &a.resident
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.marks = marks
	for p, cache := range r.placements {
		p.applyPolicy(marks.apply(uploadPolicy(cache)))
	}
}

// setPolicy applies the cache settings of the placement under the account's
// resident marks, recording them for SetResidentMarks until unregister.
func (p *placementState) setPolicy(cache *account_settings.BlockStoreCache) {
	r := p.resident
	if r == nil {
		p.applyPolicy(uploadPolicy(cache))
		return
	}
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if r.placements == nil {
		r.placements = make(map[*placementState]*account_settings.BlockStoreCache)
	}
	r.placements[p] = cache
	p.applyPolicy(r.marks.apply(uploadPolicy(cache)))
}

// unregister stops SetResidentMarks applying to the placement, at the end of
// its mount.
func (p *placementState) unregister() {
	if r := p.resident; r != nil {
		r.mtx.Lock()
		delete(r.placements, p)
		r.mtx.Unlock()
	}
}

// applyPolicy sets the store's policy. Reads are recorded from before the
// store sees a resident budget, so no read the budget pass should keep goes
// unrecorded; a budget set while the store runs keeps only the reads after
// it. A changed policy wakes the eviction loop before the store broadcasts
// it, so a loop waiting out evictShortWait checks the new marks at once.
func (p *placementState) applyPolicy(policy block_store_writeback.Policy) {
	p.reads.enabled.Store(policy.ResidentHigh > 0)
	if status, _ := p.wb.GetStatus(); status.Policy != policy && p.policyWake != nil {
		select {
		case p.policyWake <- struct{}{}:
		default:
		}
	}
	p.wb.SetPolicy(policy)
}
