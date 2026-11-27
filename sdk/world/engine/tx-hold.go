package sdk_world_engine

import (
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// slowWriteHold is the time a write transaction may stay open, from
// NewTransaction to its Commit or Discard, before the client logs its opener.
// The server holds the World writer for that whole span. Tests set
// slowWriteHold.
var slowWriteHold = 10 * time.Second

// openerFrames is the number of caller frames a slow write logs.
const openerFrames = 6

// worldPackages are the function name prefixes skipped when identifying an
// opener, so the log starts at the code that asked for the write.
var worldPackages = []string{
	"github.com/s4wave/spacewave/sdk/world.",
	"github.com/s4wave/spacewave/sdk/world/engine.",
	"github.com/s4wave/spacewave/db/world.",
}

// writeHold records where and when a write transaction opened. The server
// holds the World writer from open to finish, and its slow-commit log cannot
// see the client's code, so the client logs the opener.
type writeHold struct {
	// opened is when NewTransaction returned the writer.
	opened time.Time
	// callers are the program counters of the opening call stack.
	callers []uintptr
}

// newWriteHold captures the call stack above the function that calls it.
func newWriteHold() *writeHold {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(3, pcs)
	return &writeHold{opened: time.Now(), callers: pcs[:n]}
}

// finish logs the opener when the write stayed open past slowWriteHold. A nil
// writeHold is a read transaction and logs nothing.
func (h *writeHold) finish(outcome string) {
	if h == nil {
		return
	}
	held := time.Since(h.opened)
	if held <= slowWriteHold {
		return
	}
	logrus.WithFields(logrus.Fields{
		"held":    held,
		"outcome": outcome,
		"opener":  h.opener(),
	}).Warn("slow World write transaction")
}

// opener lists the first openerFrames frames outside the World packages,
// innermost first, as function:line separated by " < ".
func (h *writeHold) opener() string {
	// List frames until openerFrames are listed or the stack ends.
	var b strings.Builder
	named := 0
	frames := runtime.CallersFrames(h.callers)
	for named < openerFrames {
		frame, more := frames.Next()
		if !isWorldFrame(frame.Function) {
			if named != 0 {
				b.WriteString(" < ")
			}
			b.WriteString(frame.Function + ":" + strconv.Itoa(frame.Line))
			named++
		}
		if !more {
			break
		}
	}
	return b.String()
}

// isWorldFrame reports whether fn belongs to a World package.
func isWorldFrame(fn string) bool {
	for _, prefix := range worldPackages {
		if strings.HasPrefix(fn, prefix) {
			return true
		}
	}
	return false
}
