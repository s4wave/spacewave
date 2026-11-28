package volume_controller

import (
	"time"

	"github.com/sirupsen/logrus"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// gcReportInterval is the longest a run of back-to-back passes goes
// unreported.
const gcReportInterval = time.Minute

// passReport sums the collection passes since the last report. A collector
// with work left runs its passes back to back, about one a second for as long
// as an adoption or a sweep lasts, so the loop reports the sum of a run instead
// of each pass.
type passReport struct {
	// sum holds the summed counters of the passes, and the flags any of them
	// set.
	sum block_gc.CollectResult
	// passes counts the passes summed.
	passes uint64
	// since is when the first summed pass ended.
	since time.Time
}

// add sums res, a pass that ended at now, into the report.
func (r *passReport) add(res *block_gc.CollectResult, now time.Time) {
	// Start the period at the first pass.
	if r.passes == 0 {
		r.since = now
	}
	r.passes++
	s := &r.sum

	// Sum the counters every pass moves.
	s.Netted += res.Netted
	s.Counted += res.Counted
	s.Queued += res.Queued
	s.Freed += res.Freed
	s.Kept += res.Kept
	s.Leaked += res.Leaked
	s.Lapsed += res.Lapsed
	s.Duration += res.Duration

	// Sum the settle's counters.
	s.Unmarked += res.Unmarked
	s.Seeded += res.Seeded
	s.FrontScanned += res.FrontScanned
	s.Popped += res.Popped
	s.Parked += res.Parked
	s.Swept += res.Swept

	// Keep each transition, the last whole front scan, and whether work is
	// left.
	s.Flipped = s.Flipped || res.Flipped
	s.Settled = s.Settled || res.Settled
	if res.FrontDone {
		s.FrontDone, s.Front = true, res.Front
	}
	s.More = res.More
}

// due reports whether the summed passes are reported now: at a flip or a
// settle, when the run ends with no work left, and once an interval while it
// lasts.
func (r *passReport) due(now time.Time) bool {
	if r.passes == 0 {
		return false
	}
	return r.sum.Flipped || r.sum.Settled || !r.sum.More || now.Sub(r.since) >= gcReportInterval
}

// settling reports whether a summed pass moved the settle.
func (r *passReport) settling() bool {
	s := &r.sum
	return s.FrontScanned != 0 || s.Unmarked != 0 || s.Seeded != 0 || s.Popped != 0 ||
		s.Parked != 0 || s.Swept != 0 || s.Flipped || s.Settled
}

// log writes the summed passes that changed anything, at Info only when one
// moved the settle, and empties the report.
func (r *passReport) log(le *logrus.Entry) {
	// Take the sum and empty the report, then skip passes that changed
	// nothing.
	s := r.sum
	settling, passes := r.settling(), r.passes
	*r = passReport{}
	if s.Netted == 0 && s.Freed == 0 && s.Lapsed == 0 && !settling {
		return
	}

	// Report the counters every pass moves.
	le = le.
		WithField("passes", passes).
		WithField("netted", s.Netted).
		WithField("counted", s.Counted).
		WithField("queued", s.Queued).
		WithField("freed", s.Freed).
		WithField("kept", s.Kept).
		WithField("leaked", s.Leaked).
		WithField("lapsed", s.Lapsed).
		WithField("more", s.More).
		WithField("duration", s.Duration.String())
	if !settling {
		le.Debug("gc passes completed")
		return
	}

	// Add the settle's counters.
	le = le.
		WithField("unmarked", s.Unmarked).
		WithField("seeded", s.Seeded).
		WithField("front-scanned", s.FrontScanned).
		WithField("popped", s.Popped).
		WithField("parked", s.Parked).
		WithField("swept", s.Swept).
		WithField("flipped", s.Flipped).
		WithField("settled", s.Settled)
	if s.FrontDone {
		le = le.WithField("front", s.Front)
	}
	le.Info("gc passes completed")
}
