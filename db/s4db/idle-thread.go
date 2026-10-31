package s4db

import (
	"context"
	"runtime"
)

// idleThread runs functions one at a time on an OS thread at background
// priority, which the system schedules on the CPU and disk time the rest of
// the host leaves. The system may leave the thread off the CPU for as long as
// the host stays busy, so a function run on it takes no lock a request may
// wait for: a request would wait as long as the thread does.
type idleThread struct {
	// work carries the functions to run.
	work chan func()
}

// newIdleThread starts the thread, which ends with ctx. Its goroutine stays
// locked to the thread, so the runtime ends the thread with it rather than
// give a lowered thread to other goroutines. If the system refuses the lower
// priority, the functions run at normal priority.
func newIdleThread(ctx context.Context) *idleThread {
	t := &idleThread{work: make(chan func())}
	go func() {
		runtime.LockOSThread()
		_ = lowerThreadPriority()
		for {
			select {
			case <-ctx.Done():
				return
			case f := <-t.work:
				f()
			}
		}
	}()
	return t
}

// do runs f on the thread and waits for it to finish. It returns ctx's error
// without running f if ctx ends before the thread takes it.
func (t *idleThread) do(ctx context.Context, f func()) error {
	done := make(chan struct{})
	select {
	case t.work <- func() { f(); close(done) }:
	case <-ctx.Done():
		return context.Canceled
	}
	<-done
	return nil
}
