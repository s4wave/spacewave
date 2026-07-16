//go:build !js

package bldr_buildbudget

import "sync"

// Permit owns one acquired portion of a build budget.
type Permit struct {
	budget   *Budget
	weight   int64
	released *sync.Once
}

// Release returns the permit to its budget.
func (p *Permit) Release() {
	if p == nil || p.budget == nil {
		return
	}
	p.released.Do(func() {
		p.budget.semaphore.Release(p.weight)
	})
}
