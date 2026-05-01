//go:build tinygo && (scheduler.tasks || scheduler.asyncify)

package store

// defaultVerifyConcurrency returns 1 under TinyGo with a cooperative
// scheduler. Callers that branch on the value still get a sane default
// without pulling in runtime.GOMAXPROCS, which the TinyGo wasm runtimes do
// not implement meaningfully.
func defaultVerifyConcurrency() int {
	return 1
}

// goroutineVerifyExecutor runs each enqueued job on a fresh goroutine.
//
// PackReader.enqueueVerifyLocked is called from inside
// PackReader.bcast.HoldLock and the wrapped job begins with another
// HoldLock. Running the wrapped closure synchronously on the calling
// goroutine (the wasm-unknown inlineVerifyExecutor strategy) self-deadlocks
// on the non-reentrant mutex; under TinyGo's wasm-js-asyncify cooperative
// scheduler that deadlock manifests as the runtime panic
// "//go:wasmexport function did not finish".
//
// This executor breaks the recursion by spawning a goroutine per job. The
// spawned goroutine acquires bcast.HoldLock fresh after the caller's
// HoldLock unwinds, the same way *conc.ConcurrentQueue does for non-TinyGo
// builds. Spawning is cheap on the cooperative scheduler: the new task is
// queued on the runqueue without preempting the caller, and runs at the
// next yield point.
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
