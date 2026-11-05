package trace

import (
	"cmp"
	"io"
	"math"
	"slices"
	"strings"
	"time"

	parser "golang.org/x/exp/trace"
)

// Options selects the tasks Analyze records.
type Options struct {
	// TaskPrefix keeps only tasks whose name has this prefix, plus their
	// descendants. Empty keeps every task.
	TaskPrefix string
}

// taskRecord is the begin and end of one task instance.
type taskRecord struct {
	name     string
	parent   parser.TaskID
	start    parser.Time
	end      parser.Time
	hasStart bool
	hasEnd   bool
}

// complete reports whether the trace contains both ends of the task.
func (t *taskRecord) complete() bool {
	return t.hasStart && t.hasEnd
}

// duration returns the task's wall time. Only valid for complete tasks.
func (t *taskRecord) duration() time.Duration {
	return t.end.Sub(t.start)
}

// Analyze streams a trace and returns task timing statistics.
//
// Memory grows with the number of recorded tasks, not the number of events,
// so a filtered analysis of a large trace stays small.
func Analyze(r io.Reader, opts Options) (*TraceResult, error) {
	// Open the trace reader.
	tr, err := parser.NewReader(r)
	if err != nil {
		return nil, err
	}

	// Read every event once, keeping the matching tasks and counting logs.
	tasks := make(map[parser.TaskID]*taskRecord)
	logGroups := make(map[[2]string]int)
	var firstTime, lastTime parser.Time
	first := true
	for {
		ev, err := tr.ReadEvent()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if t := ev.Time(); first {
			firstTime, lastTime, first = t, t, false
		} else if t > lastTime {
			lastTime = t
		}

		switch ev.Kind() {
		case parser.EventTaskBegin, parser.EventTaskEnd:
			t := ev.Task()
			if t.ID == parser.BackgroundTask {
				continue
			}
			rec := tasks[t.ID]
			if rec == nil {
				// A task is kept when it matches the filter or its
				// parent was kept. Parents begin before children.
				_, parentKept := tasks[t.Parent]
				if opts.TaskPrefix != "" && !parentKept && !strings.HasPrefix(t.Type, opts.TaskPrefix) {
					continue
				}
				rec = &taskRecord{name: t.Type, parent: t.Parent}
				tasks[t.ID] = rec
			}
			if ev.Kind() == parser.EventTaskBegin {
				rec.start, rec.hasStart = ev.Time(), true
			} else {
				rec.end, rec.hasEnd = ev.Time(), true
			}
		case parser.EventLog:
			log := ev.Log()
			logGroups[[2]string{log.Category, log.Message}]++
		}
	}

	// Summarize the recorded tasks.
	result := &TraceResult{
		Duration:     lastTime.Sub(firstTime),
		TaskCount:    len(tasks),
		Logs:         groupLogs(logGroups),
		TypeStats:    typeStats(tasks),
		CriticalPath: buildCriticalPath(tasks),
	}
	for _, rec := range tasks {
		if !rec.complete() {
			result.IncompleteCount++
		}
	}
	return result, nil
}

// groupLogs returns log counts sorted by category and message.
func groupLogs(groups map[[2]string]int) []*LogStats {
	logs := make([]*LogStats, 0, len(groups))
	for key, count := range groups {
		logs = append(logs, &LogStats{Category: key[0], Message: key[1], Count: count})
	}
	slices.SortFunc(logs, func(a, b *LogStats) int {
		if n := cmp.Compare(a.Category, b.Category); n != 0 {
			return n
		}
		return cmp.Compare(a.Message, b.Message)
	})
	return logs
}

// typeStats groups tasks by name and returns them sorted by total time.
func typeStats(tasks map[parser.TaskID]*taskRecord) []*TaskTypeStats {
	// Group the tasks by name.
	type taskGroup struct {
		stats     *TaskTypeStats
		durations []time.Duration
	}
	groups := make(map[string]*taskGroup)
	for _, rec := range tasks {
		g := groups[rec.name]
		if g == nil {
			g = &taskGroup{stats: &TaskTypeStats{Name: rec.name}}
			groups[rec.name] = g
		}
		g.stats.Count++
		if rec.complete() {
			g.stats.Complete++
			g.durations = append(g.durations, rec.duration())
		}
	}

	// Compute each group's timing and sort by total time.
	stats := make([]*TaskTypeStats, 0, len(groups))
	for _, g := range groups {
		if len(g.durations) != 0 {
			slices.Sort(g.durations)
			for _, d := range g.durations {
				g.stats.Total += d
			}
			g.stats.Min = g.durations[0]
			g.stats.Max = g.durations[len(g.durations)-1]
			g.stats.P50 = percentile(g.durations, 0.50)
			g.stats.P99 = percentile(g.durations, 0.99)
		}
		stats = append(stats, g.stats)
	}
	slices.SortFunc(stats, func(a, b *TaskTypeStats) int {
		return cmp.Compare(b.Total, a.Total)
	})
	return stats
}

// percentile returns the value at the given percentile from a sorted slice.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := max(int(math.Ceil(p*float64(len(sorted))))-1, 0)
	return sorted[min(idx, len(sorted)-1)]
}

// buildCriticalPath builds the tree under the slowest complete root task. A
// root task has no recorded parent.
func buildCriticalPath(tasks map[parser.TaskID]*taskRecord) *TaskNode {
	// Find the slowest root task and index every task's children.
	var slowestID parser.TaskID
	var slowest *taskRecord
	children := make(map[parser.TaskID][]parser.TaskID)
	for id, rec := range tasks {
		if _, ok := tasks[rec.parent]; ok {
			children[rec.parent] = append(children[rec.parent], id)
			continue
		}
		if rec.complete() && (slowest == nil || rec.duration() > slowest.duration()) {
			slowestID, slowest = id, rec
		}
	}
	if slowest == nil {
		return nil
	}
	return buildTaskNode(tasks, children, slowestID, 0)
}

// buildTaskNode builds the subtree of complete tasks rooted at id, with
// children sorted by start time.
func buildTaskNode(
	tasks map[parser.TaskID]*taskRecord,
	children map[parser.TaskID][]parser.TaskID,
	id parser.TaskID,
	parentDur time.Duration,
) *TaskNode {
	// Build this node with its share of the parent.
	rec := tasks[id]
	dur := rec.duration()
	node := &TaskNode{Name: rec.name, Duration: dur, TaskID: id}
	if parentDur > 0 {
		node.ParentPct = float64(dur) / float64(parentDur) * 100
	}

	// Add the complete children in start order.
	kids := slices.DeleteFunc(slices.Clone(children[id]), func(child parser.TaskID) bool {
		return !tasks[child].complete()
	})
	slices.SortFunc(kids, func(a, b parser.TaskID) int {
		return cmp.Compare(tasks[a].start, tasks[b].start)
	})
	for _, child := range kids {
		node.Children = append(node.Children, buildTaskNode(tasks, children, child, dur))
	}
	return node
}
