package lanes

// Stats is a snapshot of a pool's lanes and waiting calls.
type Stats struct {
	// Limit is the most lanes all calls may hold at once; zero is paused.
	Limit int
	// Held is the number of lanes calls hold.
	Held int
	// Urgent is the number of urgent calls waiting.
	Urgent int
	// Interactive is the number of interactive calls waiting.
	Interactive int
	// Capped is the number of refill and background calls waiting.
	Capped int
}
