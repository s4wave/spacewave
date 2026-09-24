//go:build tinygo && scheduler.none

package store

// defaultVerifyConcurrency returns 1 under TinyGo wasm-unknown
// (-scheduler=none) so callers that branch on the value still get a sane
// default.
func defaultVerifyConcurrency() int {
	return 1
}

// inlineVerifyExecutor runs each enqueued job synchronously on the calling
// goroutine. It exists for TinyGo wasm-unknown builds, which compile with
// -scheduler=none and cannot spawn goroutines. PackReader prepares verify jobs
// while holding its broadcast lock, then enqueues them after the lock is
// released, so inline execution can safely reenter PackReader state.
type inlineVerifyExecutor struct{}

// Enqueue runs each job inline and returns zero queue depth.
func (inlineVerifyExecutor) Enqueue(jobs ...func()) (queued, running int) {
	for _, job := range jobs {
		if job == nil {
			continue
		}
		job()
	}
	return 0, 0
}

// newDefaultVerifyExecutor returns the inline executor.
func newDefaultVerifyExecutor(_ int) verifyExecutor {
	return inlineVerifyExecutor{}
}
