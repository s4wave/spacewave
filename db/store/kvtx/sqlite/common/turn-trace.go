package common

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// TurnTraceEnv names the file the writer-turn trace appends to. Unset, the
// store traces nothing. It is a measuring aid for benches, not a production
// interface.
const TurnTraceEnv = "SPACEWAVE_SQLITE_TURN_TRACE"

// turnTraceMin is the shortest writer-turn hold or wait the trace records.
const turnTraceMin = 100 * time.Millisecond

// turnTrace appends one line per long writer-turn hold, long foreground wait
// and checkpoint to the file TurnTraceEnv names: the event, its start in Unix
// nanoseconds, its duration in nanoseconds, the priority class, and the
// holder's frames or the checkpoint's busy, frames and copied counts.
type turnTrace struct {
	mtx sync.Mutex
	f   *os.File
}

// traceTurns is the process's trace, nil when TurnTraceEnv is unset.
var traceTurns = openTurnTrace()

// openTurnTrace opens the trace file TurnTraceEnv names, nil when unset or
// unopenable.
func openTurnTrace() *turnTrace {
	// Trace only when asked.
	path := os.Getenv(TurnTraceEnv)
	if path == "" {
		return nil
	}

	// Append to the file.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) // #nosec G703 G304 -- the operator names the trace file.
	if err != nil {
		return nil
	}
	return &turnTrace{f: f}
}

// line appends one event.
func (t *turnTrace) line(event string, start time.Time, took time.Duration, class, detail string) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	_, _ = fmt.Fprintf(t.f, "%s %d %d %s %s\n", event, start.UnixNano(), took.Nanoseconds(), class, detail)
}

// prioClass names a writer-turn priority.
func prioClass(prio int) string {
	if prio == writerBackground {
		return "background"
	}
	return "foreground"
}

// lock takes the writer turn at prio through take and returns its release. With tracing on, it
// records a foreground wait and a hold of at least turnTraceMin with the
// holder's frames.
func (t *turnTrace) lock(take func() (func(), error), prio int) (func(), error) {
	// Without tracing, only take the turn.
	if t == nil {
		return take()
	}

	// Note the holder's frames and take the turn.
	var pcs [24]uintptr
	n := runtime.Callers(3, pcs[:])
	asked := time.Now()
	release, err := take()
	if err != nil {
		return nil, err
	}
	got := time.Now()

	// Record a long foreground wait.
	if wait := got.Sub(asked); wait >= turnTraceMin && prio == writerForeground {
		t.line("wait", asked, wait, prioClass(prio), holderFrames(pcs[:n]))
	}

	// Record a long hold at its release.
	return func() {
		if held := time.Since(got); held >= turnTraceMin {
			t.line("hold", got, held, prioClass(prio), holderFrames(pcs[:n]))
		}
		release()
	}, nil
}

// checkpoint records a checkpoint of the given mode.
func (t *turnTrace) checkpoint(mode string, start time.Time, busy, frames, copied int) {
	if t != nil {
		t.line(mode, start, time.Since(start), "-", fmt.Sprintf("busy=%d frames=%d copied=%d", busy, frames, copied))
	}
}

// holderFrames names the first spacewave and cayley functions above the
// store, innermost first, joined by "<", or the innermost frame when the store
// itself holds the turn.
func holderFrames(pcs []uintptr) string {
	// Keep the project's frames, and the innermost in case none is.
	frames := runtime.CallersFrames(pcs)
	var names []string
	first := ""
	for len(names) < 8 {
		fr, more := frames.Next()
		fn := fr.Function
		if first == "" {
			first = fn[strings.LastIndex(fn, "/")+1:]
		}
		if (strings.Contains(fn, "spacewave/") || strings.Contains(fn, "cayley/")) && !strings.Contains(fn, "sqlite/common.") {
			names = append(names, fn[strings.LastIndex(fn, "/")+1:])
		}
		if !more {
			break
		}
	}

	// Join the kept frames.
	if len(names) == 0 {
		return first
	}
	return strings.Join(names, "<")
}
