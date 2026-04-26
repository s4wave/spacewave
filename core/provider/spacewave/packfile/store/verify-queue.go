package store

// verifyExecutor enqueues verify/publish jobs onto a worker pool.
//
// The shape mirrors github.com/aperturerobotics/util/conc.ConcurrentQueue so
// that the production implementation can be a *conc.ConcurrentQueue. A
// build-tagged stub provides an inline executor for environments without
// goroutine support (TinyGo wasm-unknown with -scheduler=none).
type verifyExecutor interface {
	// Enqueue submits zero or more jobs and returns the current queued/running
	// counters after the submission.
	Enqueue(jobs ...func()) (queued, running int)
}
