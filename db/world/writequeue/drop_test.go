package writequeue

import (
	"context"
	"errors"
	"testing"
)

// TestDropReplayKeepsName checks that a write whose writer heard success, then
// dropped past its attempt limit, fails with its operation's name. The replay
// copy once lost the name, so the failure log dereferenced nil and crashed the
// process during a shutdown that refused the replay.
func TestDropReplayKeepsName(t *testing.T) {
	q := &Queue{}
	w := &queuedWrite{
		ctx:      context.Background(),
		name:     func() string { return "op" },
		acked:    true,
		refusals: batchAttemptLimit - 1,
		done:     make(chan struct{}),
	}
	c := &committingBatch{writes: []*queuedWrite{w}}
	q.dropLocked(c, errors.New("dropped"))
	if !c.dropped || len(q.pending) != 0 {
		t.Fatalf("dropped %v pending %d", c.dropped, len(q.pending))
	}
}
