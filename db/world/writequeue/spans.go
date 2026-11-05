package writequeue

import (
	"context"
	"sync"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// bodySpansKey holds the spans of the body running under a context.
type bodySpansKey struct{}

// bodySpans records where one write body spent its time under the writer
// lock: the block reads it made, and the named parts the application
// reports with RecordBodyTime, such as waiting for its SQL writer. A slow
// body's log prints them so the cause of a long hold is visible.
type bodySpans struct {
	// reads counts the body's block reads; an outer counter the writer
	// attached is shared, so before marks where the body started.
	reads *block.ReadCounter
	// before is reads at the start of the body.
	before block.ReadCounterSnapshot
	// mtx guards parts.
	mtx sync.Mutex
	// parts sums the time reported for each named part.
	parts map[string]time.Duration
}

// RecordBodyTime adds d to the named part of the World write body running
// under ctx, such as "sql-wait" for the time spent waiting for an
// application's own writer. Outside a queued body it does nothing.
func RecordBodyTime(ctx context.Context, part string, d time.Duration) {
	// Skip a context outside a queued body.
	s, _ := ctx.Value(bodySpansKey{}).(*bodySpans)
	if s == nil {
		return
	}

	// Add d to the part's sum.
	s.mtx.Lock()
	if s.parts == nil {
		s.parts = make(map[string]time.Duration, 2)
	}
	s.parts[part] += d
	s.mtx.Unlock()
}

// attach starts the spans of a body running under ctx and returns the
// context the body runs under.
func (s *bodySpans) attach(ctx context.Context) context.Context {
	// Count block reads on the writer's counter when it has one, else on
	// a new one.
	if s.reads = block.ContextReadCounter(ctx); s.reads == nil {
		ctx, s.reads = block.WithReadCounter(ctx)
	}
	s.before = s.reads.Snapshot()
	return context.WithValue(ctx, bodySpansKey{}, s)
}

// fields returns the spans of a body that took elapsed as log fields: the
// block reads, each reported part, and the rest of elapsed as "other".
func (s *bodySpans) fields(elapsed time.Duration) logrus.Fields {
	// Report the reads made since the body started.
	after := s.reads.Snapshot()
	readTime := after.BlockReadTime - s.before.BlockReadTime
	fields := logrus.Fields{
		"reads":      after.BlockReadCount - s.before.BlockReadCount,
		"read-bytes": after.BlockReadBytes - s.before.BlockReadBytes,
		"read-time":  readTime.Round(time.Millisecond).String(),
	}

	// Report each part and what no span covers.
	other := elapsed - readTime
	s.mtx.Lock()
	for part, d := range s.parts {
		fields[part] = d.Round(time.Millisecond).String()
		other -= d
	}
	s.mtx.Unlock()
	fields["other"] = other.Round(time.Millisecond).String()
	return fields
}
