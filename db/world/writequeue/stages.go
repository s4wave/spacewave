package writequeue

import (
	"sync"
	"time"
)

// Stage is a part of a committed World write's time, from when it queued to
// its durable result.
type Stage int

const (
	// StageWait runs from the write's queueing to the start of its last body
	// run: the batches ahead of it, and any peer fetch or rerun.
	StageWait Stage = iota
	// StageBody is its checks and body, under the writer lock.
	StageBody
	// StageBatch runs from its body's end to its batch's admission: the
	// bodies after it in the batch.
	StageBatch
	// StageAdmit is the engine's admission of its batch, under the writer
	// lock: the block writes of the commit.
	StageAdmit
	// StageDurable runs from the admission to the durable result: the
	// publication, with the writer lock released.
	StageDurable
	// stageCount is the number of stages.
	stageCount
)

// String names the stage as a metric label.
func (s Stage) String() string {
	switch s {
	case StageWait:
		return "wait"
	case StageBody:
		return "body"
	case StageBatch:
		return "batch"
	case StageAdmit:
		return "admit"
	case StageDurable:
		return "durable"
	default:
		return "unknown"
	}
}

// StageTotals is the summed stage times of one operation's committed writes.
type StageTotals struct {
	// Writes counts the writes.
	Writes uint64
	// Seconds sums each stage's time, indexed by Stage.
	Seconds [stageCount]float64
}

// stageStats holds the stage totals of the process's queues, by operation.
var stageStats struct {
	mtx sync.Mutex
	ops map[string]*StageTotals
}

// GetStageTotals returns a copy of the stage totals of every queue in the
// process since it started, by operation name.
func GetStageTotals() map[string]StageTotals {
	stageStats.mtx.Lock()
	defer stageStats.mtx.Unlock()
	out := make(map[string]StageTotals, len(stageStats.ops))
	for op, t := range stageStats.ops {
		out[op] = *t
	}
	return out
}

// observeStages adds the stage times of c's writes, durable at now. A write
// that ended before the commit, by failing or changing nothing, is not
// counted; one answered at admission is, since its commit's cost is the
// same.
func (q *Queue) observeStages(c *committingBatch, now time.Time) {
	// Measure each write's stages.
	type sample struct {
		op     string
		stages [stageCount]time.Duration
	}
	samples := make([]sample, 0, len(c.writes))
	for _, w := range c.writes {
		if (w.ended() && !w.acked) || w.bodyStarted.IsZero() || c.admitted.IsZero() {
			continue
		}
		var s sample
		s.op = w.name()
		s.stages[StageWait] = w.bodyStarted.Sub(w.queued)
		s.stages[StageBody] = w.bodyEnded.Sub(w.bodyStarted)
		s.stages[StageBatch] = c.admitStarted.Sub(w.bodyEnded)
		s.stages[StageAdmit] = c.admitted.Sub(c.admitStarted)
		s.stages[StageDurable] = now.Sub(c.admitted)
		samples = append(samples, s)
	}

	// Add them to the totals.
	stageStats.mtx.Lock()
	defer stageStats.mtx.Unlock()
	if stageStats.ops == nil {
		stageStats.ops = make(map[string]*StageTotals)
	}
	for _, s := range samples {
		t := stageStats.ops[s.op]
		if t == nil {
			t = &StageTotals{}
			stageStats.ops[s.op] = t
		}
		t.Writes++
		for i, d := range s.stages {
			t.Seconds[i] += max(d, 0).Seconds()
		}
	}
}
