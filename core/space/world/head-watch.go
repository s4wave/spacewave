package space_world

import (
	"context"
	"slices"
	"sync"

	"github.com/s4wave/spacewave/db/world"
)

// headWatch is one WatchHead registration, guarded by the adapter's bcast.
type headWatch struct {
	// reporting is the head fn is running with, or zero.
	reporting world.Head
}

// WatchHead calls fn on its own goroutine with the reportable head, then with
// later ones, until release is called. Every head is durable and its retained
// roots are held. A slow fn skips the heads it fell behind on, and the adapter
// decides candidates without waiting for fn: their committers publish their
// heads from the Admission. The head fn runs with stays live until it returns.
func (a *Adapter) WatchHead(_ context.Context, fn func(world.Head)) (func(), error) {
	// Register, so gatherLive sees the head each report runs with.
	w := &headWatch{}
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		a.watches = append(a.watches, w)
	})

	// Report each reportable head that moved until release.
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		var last world.Head
		for {
			// End on release even while heads keep moving.
			select {
			case <-stop:
				return
			default:
			}

			// Take the head and mark it reporting in one step, so it stays
			// live from reportable to the engine's publication.
			var wake <-chan struct{}
			var head world.Head
			a.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
				wake, head = getWaitCh(), a.reportable
				if head.Root.GetEmpty() || (head.Root.EqualsRef(last.Root) && head.Seqno == last.Seqno) {
					head = world.Head{}
				}
				w.reporting = head
			})
			if !head.Root.GetEmpty() {
				fn(head)
				last = head
				a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
					w.reporting = world.Head{}
				})
				continue
			}
			select {
			case <-stop:
				return
			case <-wake:
			}
		}
	}()

	// Release stops and joins the reports, then removes the registration.
	// fn must not call it.
	return sync.OnceFunc(func() {
		close(stop)
		<-done
		a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			a.watches = slices.DeleteFunc(a.watches, func(o *headWatch) bool { return o == w })
		})
	}), nil
}
