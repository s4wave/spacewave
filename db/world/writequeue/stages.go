package writequeue

import (
	"context"
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
	// Parts sums the named parts of the admit and durable stages that the
	// engine and the transaction reported, by part name.
	Parts map[string]float64
	// Pages and PageBytes sum the World pages the writes' commits stored and
	// their length, each write counting its share of its batch's commit.
	Pages, PageBytes float64
}

// commitParts collects what the engine and the transaction report about one
// batch's commit: the time of each named part of its admission and durable
// wait, and the World pages it stored.
type commitParts struct {
	mtx       sync.Mutex
	times     map[string]time.Duration
	pages     uint64
	pageBytes uint64
}

// commitPartsKey is the context key of the commitParts of the batch whose
// admission or durable wait runs under the context.
type commitPartsKey struct{}

// withCommitParts returns ctx carrying p.
func withCommitParts(ctx context.Context, p *commitParts) context.Context {
	return context.WithValue(ctx, commitPartsKey{}, p)
}

// RecordCommitTime adds d to the named part of the commit of the batch whose
// admission or durable wait runs under ctx. Outside one it does nothing.
func RecordCommitTime(ctx context.Context, part string, d time.Duration) {
	if p := CommitTimeRecorder(ctx); p != nil {
		p(part, d)
	}
}

// CommitTimeRecorder returns a function adding a duration to a named part of
// the commit of the batch whose admission runs under ctx, or nil outside one.
// A caller that times parts after ctx ends keeps the function.
func CommitTimeRecorder(ctx context.Context) func(part string, d time.Duration) {
	p, _ := ctx.Value(commitPartsKey{}).(*commitParts)
	if p == nil {
		return nil
	}
	return func(part string, d time.Duration) {
		p.mtx.Lock()
		if p.times == nil {
			p.times = make(map[string]time.Duration, 8)
		}
		p.times[part] += d
		p.mtx.Unlock()
	}
}

// RecordCommitPages adds the World pages a publication of the batch admitted
// under ctx stored, and their length. Outside an admission it does nothing.
func RecordCommitPages(ctx context.Context, pages int, bytes uint64) {
	p, _ := ctx.Value(commitPartsKey{}).(*commitParts)
	if p == nil {
		return
	}
	p.mtx.Lock()
	p.pages += uint64(pages)
	p.pageBytes += bytes
	p.mtx.Unlock()
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
		c := *t
		c.Parts = make(map[string]float64, len(t.Parts))
		for part, secs := range t.Parts {
			c.Parts[part] = secs
		}
		out[op] = c
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
	if len(samples) == 0 {
		return
	}

	// Read the commit's parts. Each write waited for every part; the pages
	// are shared among the writes.
	c.parts.mtx.Lock()
	parts := make(map[string]float64, len(c.parts.times))
	for part, d := range c.parts.times {
		parts[part] = max(d, 0).Seconds()
	}
	pages := float64(c.parts.pages) / float64(len(samples))
	pageBytes := float64(c.parts.pageBytes) / float64(len(samples))
	c.parts.mtx.Unlock()

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
		if t.Parts == nil {
			t.Parts = make(map[string]float64, len(parts))
		}
		for part, secs := range parts {
			t.Parts[part] += secs
		}
		t.Pages += pages
		t.PageBytes += pageBytes
	}
}
