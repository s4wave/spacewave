package pace

import "time"

// Config explicitly selects optional eviction pacing. Its zero value does
// not pause, rate-limit or back off, retaining the unpaced store contract.
type Config struct {
	// Paused holds every pass until its context ends.
	Paused bool
	// BytesPerSecond caps removal bandwidth; zero leaves it uncapped.
	BytesPerSecond float64
	// FlushMax is the mean flush time that triggers backoff; zero disables it.
	FlushMax time.Duration
}
