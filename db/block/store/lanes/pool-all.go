package lanes

import "context"

// WithAll visits each lane while retaining its lease, runs settled after the
// visits, and then returns the leases. Calls keep their normal admission order.
// The pool must admit background calls throughout this operation.
func (p *Pool[T]) WithAll(visit func(*Lease[T]), settled func()) {
	// Retain every visited lane until all visits and their background work finish.
	held := make([]*Lease[T], 0, p.n)
	for range p.n {
		lease, _ := p.Take(context.Background(), nil)
		visit(lease)
		held = append(held, lease)
	}
	settled()

	// Restore the lanes to normal admission.
	for _, lease := range held {
		p.Put(lease)
	}
}
