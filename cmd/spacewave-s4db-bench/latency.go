//go:build !js

package main

import (
	"time"
)

// Latency summarizes a set of durations.
type Latency struct {
	// Count is the number of commits.
	Count int
	// P50 is the median.
	P50 time.Duration
	// P99 is the 99th percentile.
	P99 time.Duration
	// Max is the longest.
	Max time.Duration
}
