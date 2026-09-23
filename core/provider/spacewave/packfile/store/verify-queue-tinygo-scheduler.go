//go:build tinygo && !scheduler.none

package store

// defaultVerifyConcurrency returns 1 under TinyGo with any goroutine
// scheduler, and during package analysis that sets no scheduler tag. Callers
// that branch on the value still get a sane default without pulling in
// runtime.GOMAXPROCS, which the TinyGo wasm runtimes do not implement
// meaningfully.
func defaultVerifyConcurrency() int {
	return 1
}

// goroutineVerifyExecutor runs each enqueued job on a fresh goroutine.
//
// TinyGo goroutine schedulers can run verification in background tasks.
// PackReader prepares jobs under its broadcast lock and enqueues them only
// after releasing that lock, so this executor matches the production queue
// shape without relying on recursive lock avoidance.
type goroutineVerifyExecutor struct{}

// Enqueue spawns one goroutine per job and returns immediately.
func (goroutineVerifyExecutor) Enqueue(jobs ...func()) (queued, running int) {
	count := 0
	for _, job := range jobs {
		if job == nil {
			continue
		}
		j := job
		go j()
		count++
	}
	return 0, count
}

// newDefaultVerifyExecutor returns the goroutine-based executor.
func newDefaultVerifyExecutor(_ int) verifyExecutor {
	return goroutineVerifyExecutor{}
}
