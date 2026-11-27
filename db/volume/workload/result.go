package workload

import (
	"slices"
	"time"
)

// Result reports one replay.
type Result struct {
	// Ops is the number of records replayed.
	Ops int
	// Wall is the replay's elapsed time.
	Wall time.Duration
	// Latency holds each timed operation's latencies in replay order: commit,
	// sync, put, put-batch, and get-block.
	Latency map[Op][]time.Duration
	// CommitErrors counts commits the target rejected. A replay that commits
	// transactions the recording interleaved may conflict where the
	// recording did not.
	CommitErrors int
	// OrderedCommits counts commits made with write ordering only.
	OrderedCommits int
	// BlockBytes is the payload written by block puts.
	BlockBytes int64
	// ValueBytes is the payload written by key-value sets.
	ValueBytes int64
}

// Percentile returns the p-th percentile latency of op, or zero when op was
// never timed.
func (r *Result) Percentile(op Op, p float64) time.Duration {
	samples := slices.Clone(r.Latency[op])
	if len(samples) == 0 {
		return 0
	}
	slices.Sort(samples)
	return samples[int(p*float64(len(samples)-1))]
}
