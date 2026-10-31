//go:build !js

package main

import (
	"cmp"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/db/s4db"
)

// Report is the result of one run.
type Report struct {
	// Seconds is the wall time of the writes.
	Seconds float64
	// Ops is the keys written.
	Ops int
	// Commits is the commits made.
	Commits int
	// Idle is the commit latency in windows where no job ran.
	Idle Latency
	// Flush is the commit latency in windows where a flush ended.
	Flush Latency
	// Merge is the commit latency in windows where a merge ended.
	Merge Latency
	// Busy is the commit latency in windows where a job held a reservation
	// without ending.
	Busy Latency
	// Windows counts the sample windows of each class.
	Windows map[string]int
	// Slowest are the slowest commits, slowest first, each with the window it
	// ended in.
	Slowest []slowCommit
	// BackgroundWaitMax is the engine's own longest wait of a commit on
	// background work.
	BackgroundWaitMax time.Duration
	// Stats is the engine's report at the end.
	Stats s4db.Stats
}

// slowestCount is how many of the slowest commits the report lists.
const slowestCount = 10

// slowCommit is one of the slowest commits and the window it ended in.
type slowCommit struct {
	commit
	// class is the window's class, and prev and cur its samples.
	class     string
	prev, cur sample
}

// classify assigns each commit to the window between the samples around its
// end, classed by what the engine did in it.
func (a *BenchArgs) classify(samples []sample, commits []commit) *Report {
	// Walk the windows and the commits together, both in time order.
	byClass := map[string][]time.Duration{}
	windows := map[string]int{}
	slow := make([]slowCommit, 0, len(commits))
	i := 0
	for w := 1; w < len(samples); w++ {
		// Class the window and take the commits that ended in it.
		prev, cur := samples[w-1], samples[w]
		class := windowClass(prev, cur)
		windows[class]++
		for i < len(commits) && commits[i].at <= cur.at {
			byClass[class] = append(byClass[class], commits[i].dur)
			slow = append(slow, slowCommit{commit: commits[i], class: class, prev: prev, cur: cur})
			i++
		}
	}

	// Commits after the last sample count as idle.
	for i < len(commits) {
		byClass["idle"] = append(byClass["idle"], commits[i].dur)
		slow = append(slow, slowCommit{commit: commits[i], class: "idle"})
		i++
	}

	// Keep the slowest.
	slices.SortFunc(slow, func(x, y slowCommit) int { return cmp.Compare(y.dur, x.dur) })
	return &Report{
		Idle:    summarize(byClass["idle"]),
		Flush:   summarize(byClass["flush"]),
		Merge:   summarize(byClass["merge"]),
		Busy:    summarize(byClass["busy"]),
		Windows: windows,
		Slowest: slow[:min(len(slow), slowestCount)],
	}
}

// windowClass names the window between two samples: merge when a run above
// level 0 changed or level 0 shrank, flush when level 0 grew, busy when a job
// held reserved chunks, else idle.
func windowClass(prev, cur sample) string {
	// Read a level's count, zero past the list.
	levels := max(len(prev.runs), len(cur.runs))
	at := func(runs []int, l int) int {
		if l < len(runs) {
			return runs[l]
		}
		return 0
	}

	// A change below level 0 or a shrunk level 0 ends a merge.
	for l := 1; l < levels; l++ {
		if at(prev.runs, l) != at(cur.runs, l) {
			return "merge"
		}
	}
	if at(cur.runs, 0) < at(prev.runs, 0) {
		return "merge"
	}

	// A grown level 0 ends a flush; a reservation means a job runs.
	if at(cur.runs, 0) > at(prev.runs, 0) {
		return "flush"
	}
	if prev.reserved != 0 || cur.reserved != 0 {
		return "busy"
	}
	return "idle"
}

// summarize returns the quantiles of durs, sorting them in place.
func summarize(durs []time.Duration) Latency {
	// Report an empty set as zero, else sort and read the ranks.
	if len(durs) == 0 {
		return Latency{}
	}
	slices.Sort(durs)
	rank := func(q float64) time.Duration {
		i := int(q*float64(len(durs))+0.999999) - 1
		return durs[min(max(i, 0), len(durs)-1)]
	}
	return Latency{Count: len(durs), P50: rank(0.5), P99: rank(0.99), Max: durs[len(durs)-1]}
}

// emit prints the report or writes it to its file.
func (a *BenchArgs) emit(r *Report) error {
	// Encode the report.
	var arena fastjson.Arena
	data := r.value(&arena).MarshalTo(nil)
	data = append(data, '\n')

	// Print it or write its file.
	if a.ReportPath == "" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(a.ReportPath, data, 0o644)
}

// value builds the report's JSON object. Durations are strings in Go's
// duration syntax, counts and bytes numbers.
func (r *Report) value(arena *fastjson.Arena) *fastjson.Value {
	// The run.
	obj := arena.NewObject()
	obj.Set("seconds", arena.NewNumberFloat64(r.Seconds))
	obj.Set("ops", intValue(arena, int64(r.Ops)))
	obj.Set("commits", intValue(arena, int64(r.Commits)))

	// The latencies by window class.
	obj.Set("idle", r.Idle.value(arena))
	obj.Set("flush", r.Flush.value(arena))
	obj.Set("merge", r.Merge.value(arena))
	obj.Set("busy", r.Busy.value(arena))
	obj.Set("backgroundWaitMax", arena.NewString(r.BackgroundWaitMax.String()))

	// The window counts, in a fixed order.
	windows := arena.NewObject()
	for _, class := range []string{"idle", "flush", "merge", "busy"} {
		windows.Set(class, intValue(arena, int64(r.Windows[class])))
	}
	obj.Set("windows", windows)

	// The slowest commits.
	slowest := arena.NewArray()
	for i, sc := range r.Slowest {
		slowest.SetArrayItem(i, sc.value(arena))
	}
	obj.Set("slowest", slowest)
	obj.Set("stats", statsValue(arena, r.Stats))
	return obj
}

// value builds a slow commit's JSON object: when it ended, how long it took,
// and its window's class and samples.
func (sc slowCommit) value(arena *fastjson.Arena) *fastjson.Value {
	// The commit and its window.
	obj := arena.NewObject()
	obj.Set("at", arena.NewString(sc.at.String()))
	obj.Set("dur", arena.NewString(sc.dur.String()))
	obj.Set("class", arena.NewString(sc.class))
	obj.Set("before", sc.prev.value(arena))
	obj.Set("after", sc.cur.value(arena))
	return obj
}

// value builds a sample's JSON object.
func (s sample) value(arena *fastjson.Arena) *fastjson.Value {
	// The runs per level, the reservation and the file size.
	obj := arena.NewObject()
	runs := arena.NewArray()
	for i, n := range s.runs {
		runs.SetArrayItem(i, intValue(arena, int64(n)))
	}
	obj.Set("at", arena.NewString(s.at.String()))
	obj.Set("runs", runs)
	obj.Set("reserved", intValue(arena, s.reserved))
	obj.Set("fileBytes", intValue(arena, s.fileBytes))
	return obj
}

// value builds a latency summary's JSON object.
func (l Latency) value(arena *fastjson.Arena) *fastjson.Value {
	// The count and the ranks.
	obj := arena.NewObject()
	obj.Set("count", intValue(arena, int64(l.Count)))
	obj.Set("p50", arena.NewString(l.P50.String()))
	obj.Set("p99", arena.NewString(l.P99.String()))
	obj.Set("max", arena.NewString(l.Max.String()))
	return obj
}

// statsValue builds the engine statistics' JSON object.
func statsValue(arena *fastjson.Arena, st s4db.Stats) *fastjson.Value {
	// The byte and key counts.
	obj := arena.NewObject()
	obj.Set("fileBytes", intValue(arena, st.FileBytes))
	obj.Set("liveBytes", intValue(arena, st.LiveBytes))
	obj.Set("deadBytes", intValue(arena, st.DeadBytes))
	obj.Set("freeBytes", intValue(arena, st.FreeBytes))
	obj.Set("pendingBytes", intValue(arena, st.PendingBytes))

	// The keys and the write accounting.
	obj.Set("keyCount", intValue(arena, st.KeyCount))
	obj.Set("memtableBytes", intValue(arena, st.MemtableBytes))
	obj.Set("written", intValue(arena, st.Written))
	obj.Set("userBytes", intValue(arena, st.UserBytes))

	// The chunks by kind.
	chunks := arena.NewObject()
	chunks.Set("free", intValue(arena, st.Chunks.Free))
	chunks.Set("log", intValue(arena, st.Chunks.Log))
	chunks.Set("value", intValue(arena, st.Chunks.Value))
	chunks.Set("run", intValue(arena, st.Chunks.Run))

	// The chunks a job holds or a pin keeps, and the manifest's.
	chunks.Set("manifest", intValue(arena, st.Chunks.Manifest))
	chunks.Set("reserved", intValue(arena, st.Chunks.Reserved))
	chunks.Set("pending", intValue(arena, st.Chunks.Pending))
	obj.Set("chunks", chunks)

	// The runs per level.
	runs := arena.NewArray()
	for i, n := range st.Runs {
		runs.SetArrayItem(i, intValue(arena, int64(n)))
	}
	obj.Set("runs", runs)
	return obj
}

// intValue returns n as a JSON number.
func intValue(arena *fastjson.Arena, n int64) *fastjson.Value {
	return arena.NewNumberString(strconv.FormatInt(n, 10))
}
