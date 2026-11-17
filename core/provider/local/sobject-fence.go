package provider_local

import (
	"sync"

	"github.com/pkg/errors"
)

// ErrSharedObjectsFenced is returned by a create or join of a shared object on
// an account whose Spaces a transfer is moving.
var ErrSharedObjectsFenced = errors.New("the account's Spaces are being transferred; create or join a Space after the transfer")

// sobjectFences counts, per account id, the fences held on the account. It
// lives on the Provider, so a fence outlives the account's mounts and holds
// no reference that would keep the account's volume open.
type sobjectFences struct {
	// mtx guards held.
	mtx sync.Mutex
	// held counts the fences per account id; an account with none has no
	// entry.
	held map[string]int
}

// FenceSharedObjects refuses every create and join of a shared object on the
// account with ErrSharedObjectsFenced until release runs. A transfer fences
// its source for the run, so no Space appears there that the run's list
// misses. The fence lives in memory: a restart drops it with the run, which
// takes it again when it resumes.
func (a *ProviderAccount) FenceSharedObjects() (release func()) {
	// Count the fence on the account.
	f, accountID := &a.t.p.fences, a.t.accountInfo.GetProviderAccountId()
	f.mtx.Lock()
	if f.held == nil {
		f.held = make(map[string]int)
	}
	f.held[accountID]++
	f.mtx.Unlock()

	// Release it once, dropping the count at zero.
	return sync.OnceFunc(func() {
		f.mtx.Lock()
		if f.held[accountID]--; f.held[accountID] == 0 {
			delete(f.held, accountID)
		}
		f.mtx.Unlock()
	})
}

// refuseFenced returns ErrSharedObjectsFenced while a fence is held on the
// account.
func (a *ProviderAccount) refuseFenced() error {
	// Read the account's fence count.
	f := &a.t.p.fences
	f.mtx.Lock()
	n := f.held[a.t.accountInfo.GetProviderAccountId()]
	f.mtx.Unlock()
	if n != 0 {
		return ErrSharedObjectsFenced
	}
	return nil
}
