//go:build !tinygo

package block_store_s3

import "time"

// GateState is a snapshot of a request gate.
type GateState struct {
	// Name identifies the gate: the service endpoint.
	Name string
	// Limit is the adapted limit of requests in flight.
	Limit int
	// PausedUntil is when the current pause ends, or zero.
	PausedUntil time.Time
	// InFlight is the number of requests in flight.
	InFlight int
	// QueuedUrgent is the number of urgent requests waiting.
	QueuedUrgent int
	// QueuedInteractive is the number of interactive requests waiting.
	QueuedInteractive int
	// QueuedCapped is the number of refill and background requests waiting.
	QueuedCapped int
	// Throttles counts the throttled answers.
	Throttles uint64
	// Retries counts the retries of transient faults.
	Retries uint64
}
