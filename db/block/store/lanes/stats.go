package lanes

// Stats is a snapshot of a pool's lanes and waiting calls.
type Stats struct {
	// Limit is the most lanes all calls may hold at once.
	// Zero pauses the pool.
	Limit int
	// Held is the number of lanes calls hold.
	Held int
	// Urgent is the number of urgent calls waiting.
	Urgent int
	// Interactive is the number of interactive calls waiting.
	Interactive int
	// Refill is the number of refill calls waiting.
	Refill int
	// Background is the number of background calls waiting.
	Background int
}
