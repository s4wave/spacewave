//go:build !js

package spacewave_cli

import (
	"context"
	"strconv"
	"time"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// collectProgressEvery is the least time between two progress lines of an
// offline collection.
const collectProgressEvery = 30 * time.Second

// settleStatusLimit bounds the front entries and the holes the report
// counts.
const settleStatusLimit = 100_000

// collectReport records what an offline collection did and what the settle's
// flip checks found after the collection. These results say why an Open
// generation did not flip.
type collectReport struct {
	// path is the volume file.
	path string
	// frees reports whether the collector freed.
	frees bool
	// grace is the collector's grace period.
	grace time.Duration
	// passes is the number of passes run.
	passes int
	// res sums the passes' accounting, with Front from the last pass that
	// finished a scan of the whole front.
	res block_gc.CollectResult
	// status is what the flip's checks found after the passes, nil when
	// unread.
	status *block_gc.SettleStatus
	// load is the writer run beside the passes, nil without one.
	load *volumeLoad
	// le logs the passes' progress, nil for none.
	le *logrus.Entry
	// logged is when the last progress line was written.
	logged time.Time
}

// newCollectReport returns an empty report of a collection of the volume at
// path with conf.
func newCollectReport(path string, conf *store_kvtx.Config) *collectReport {
	grace, _ := time.ParseDuration(conf.GetCollectorGraceDur())
	if grace <= 0 {
		grace = block_gc.DefaultGrace
	}
	return &collectReport{path: path, frees: conf.GetCollectorFrees(), grace: grace}
}

// add adds one pass's accounting.
func (r *collectReport) add(res block_gc.CollectResult) {
	// Sum the netting's counts.
	r.passes++
	sum := &r.res
	sum.Netted += res.Netted
	sum.Counted += res.Counted
	sum.Lapsed += res.Lapsed

	// Sum the free's counts.
	sum.Queued += res.Queued
	sum.Freed += res.Freed
	sum.Kept += res.Kept
	sum.Leaked += res.Leaked
	sum.Underflows += res.Underflows

	// Sum the settle's counts.
	sum.Parked += res.Parked
	sum.Requeued += res.Requeued
	sum.Unmarked += res.Unmarked
	sum.Popped += res.Popped
	sum.Seeded += res.Seeded
	sum.FrontScanned += res.FrontScanned
	sum.Swept += res.Swept

	// Sum the store's work, and keep the pace the last pass left.
	sum.Slices += res.Slices
	sum.StoreTime += res.StoreTime
	sum.ScanTime += res.ScanTime
	sum.PaceShift = res.PaceShift

	// Keep the settle's events and the last full scan of the front.
	sum.Flipped = sum.Flipped || res.Flipped
	sum.Unadopted = sum.Unadopted || res.Unadopted
	sum.Settled = sum.Settled || res.Settled
	if res.FrontDone {
		sum.Front, sum.FrontDone = res.Front, true
	}
	sum.More = res.More
}

// progress logs the passes' totals so far, at most once in
// collectProgressEvery, so a collection that runs for hours shows where it is
// before its report prints.
func (r *collectReport) progress() {
	if r.le == nil || time.Since(r.logged) < collectProgressEvery {
		return
	}
	r.logged = time.Now()
	r.le.WithFields(logrus.Fields{
		"passes":        r.passes,
		"netted":        r.res.Netted,
		"queued":        r.res.Queued,
		"parked":        r.res.Parked,
		"freed":         r.res.Freed,
		"front-scanned": r.res.FrontScanned,
		"popped":        r.res.Popped,
		"swept":         r.res.Swept,
		"slices":        r.res.Slices,
		"store-time":    r.res.StoreTime.Round(time.Millisecond).String(),
		"scan-time":     r.res.ScanTime.Round(time.Millisecond).String(),
		"pace-shift":    r.res.PaceShift,
	}).Info("collecting the volume")
}

// readStatus reads what the flip's checks find on vol.
func (r *collectReport) readStatus(ctx context.Context, vol *volume_s4db.Volume) error {
	status, err := vol.SettleStatus(ctx, settleStatusLimit)
	if err != nil {
		return err
	}
	r.status = &status
	return nil
}

// fields formats the report in the command's field table.
func (r *collectReport) fields() [][2]string {
	// List the passes' totals.
	res := r.res
	front := "no pass scanned the whole front"
	if res.FrontDone {
		front = strconv.FormatUint(res.Front, 10)
	}
	fields := [][2]string{
		{"Volume", r.path},
		{"Frees", strconv.FormatBool(r.frees)},
		{"Passes", strconv.Itoa(r.passes)},
		{"Netted pages", strconv.FormatUint(res.Netted, 10)},
		{"Counted blocks", strconv.FormatUint(res.Counted, 10)},
		{"Queued to free", strconv.FormatUint(res.Queued, 10)},
		{"Freed", strconv.FormatUint(res.Freed, 10)},
		{"Kept", strconv.FormatUint(res.Kept, 10)},
		{"Parked older blocks", strconv.FormatUint(res.Parked, 10)},
		{"Requeued parked blocks", strconv.FormatUint(res.Requeued, 10)},
		{"Leaked", strconv.FormatUint(res.Leaked, 10)},
		{"Underflows refused", strconv.FormatUint(res.Underflows, 10)},
		{"Lapsed leases", strconv.FormatUint(res.Lapsed, 10)},
		{"Unmarked run entries added", strconv.FormatUint(res.Unmarked, 10)},
		{"Front seeded", strconv.FormatUint(res.Seeded, 10)},
		{"Front scanned", strconv.FormatUint(res.FrontScanned, 10)},
		{"Front popped", strconv.FormatUint(res.Popped, 10)},
		{"Front left at the last full scan", front},
		{"Flipped", strconv.FormatBool(res.Flipped)},
		{"Returned to unadopted", strconv.FormatBool(res.Unadopted)},
		{"Swept", strconv.FormatUint(res.Swept, 10)},
		{"Settled", strconv.FormatBool(res.Settled)},
		{"Write slices", strconv.FormatUint(res.Slices, 10)},
		{"Writer wait and commit time", res.StoreTime.String()},
		{"Queue scan time", res.ScanTime.String()},
		{"Pace shift at the end", strconv.FormatUint(uint64(res.PaceShift), 10)},
		{"Duration", res.Duration.String()},
	}

	// Add the state the passes left, and what the flip waits on.
	if st := r.status; st != nil {
		fields = append(fields,
			[2]string{"Adoption", st.State.GetAdoption().String()},
			[2]string{"Generation", st.State.GetGeneration().String()},
		)
		fields = append(fields, settleStatusFields(st, r.frees, true)...)
	}

	// Add the writer's commits beside the passes' totals.
	if r.load != nil {
		fields = append(fields, r.load.fields()...)
	}
	return fields
}

// settleStatusFields formats, while the generation st read is Open, each of
// the settle's flip checks and the first that holds the flip, given whether
// the collector frees, with a sample of the front and the holes. Without
// graced, st was read with a grace period other than the collector's, so
// the grace clause is left out.
func settleStatusFields(st *block_gc.SettleStatus, frees, graced bool) [][2]string {
	// Only an Open generation waits on the flip.
	if st.State.GetGeneration() != block_gc.Generation_GENERATION_OPEN {
		return nil
	}

	// List the grace clause when st read the collector's grace.
	var fields [][2]string
	if graced {
		fields = [][2]string{
			{"Freeable", strconv.FormatBool(st.Freeable)},
			{"Grace left", st.GraceLeft.Round(time.Second).String()},
		}
	}

	// List each other check, in the order the flip takes them.
	fields = append(fields, [][2]string{
		{"Release in progress", strconv.FormatBool(st.State.GetRelease() != nil)},
		{"Rewrite owes a recount", strconv.FormatBool(st.Rewritten)},
		{"Unmarked run complete", strconv.FormatBool(st.RunComplete)},
		{"Front entries", settleCount(st.Front, st.FrontMore)},
	}...)
	for cause, count := range st.FrontCauses {
		if count.Count != 0 {
			fields = append(fields, [2]string{
				"Front " + block_gc.FrontCause(cause).String(),
				strconv.Itoa(count.Count) + " (sample " + count.Sample.MarshalString() + ")",
			})
		}
	}

	// Report the remaining flip checks after the front proof.
	fields = append(fields, [][2]string{
		{"Recorded holes", settleCount(st.Holes, st.HolesMore)},
		{"Ripe block queued, not parked", strconv.FormatBool(st.RipeQueued)},
		{"Flip held by", flipHeldBy(st, frees, graced)},
	}...)
	fields = appendRefSample(fields, "Front sample", st.FrontSample)
	return appendRefSample(fields, "Hole sample", st.HoleSample)
}

// settleCount formats a count read up to settleStatusLimit.
func settleCount(n int, more bool) string {
	if more {
		return "over " + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// flipHeldBy reports the first of the flip's checks that holds the Open
// generation st describes, in the order Collector.flip takes the checks,
// given whether the collector frees, since the settle runs only with frees,
// and whether st read the collector's grace period. The checks read what
// the Volume stores. A collector also seeds the front from the roots once
// before its first flip. The collector keeps the seeding state in memory,
// and st cannot read that state, so with every stored check clear the flip
// still waits on that seeding.
func flipHeldBy(st *block_gc.SettleStatus, frees, graced bool) string {
	switch state := st.State; {
	case !frees:
		return "frees are off, and the settle runs only with frees"
	case state.GetAdoption() != block_gc.Adoption_ADOPTION_ADOPTED:
		return "the Volume is " + state.GetAdoption().String() + "; an adoption must count it first"
	case graced && st.GraceLeft > 0:
		return "the grace period after the adoption, " + st.GraceLeft.Round(time.Second).String() + " left; pass a shorter --grace"
	case state.GetRelease() != nil:
		return "a paged release is in progress"
	case st.Rewritten:
		return "a list rewrite owes a recount; the next pass returns the Volume to unadopted, and the next volume-collect --frees recounts it"
	case !st.RunComplete:
		return "the unmarked run is not complete"
	case st.Front != 0:
		return "the front: blocks without a verified child list; a pass takes off the ones nothing holds, and debug ref-repair walks the rest"
	case st.Holes != 0:
		return "recorded holes: blocks lost with their lists that a put must store again"
	case st.RipeQueued:
		return "a ripe block queued to free, which the next free parks"
	default:
		return "nothing stored; the next pass with frees on seeds the front from the roots, and flips if that adds no block"
	}
}
