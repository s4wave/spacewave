//go:build !js

package storebench

import (
	"os"
	"path/filepath"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"time"
)

// maxStallDumps bounds the goroutine dumps one load writes.
const maxStallDumps = 32

// stallTraceDelay is how long after the first stall the flight recorder's
// window is written, so the trace spans the stall's start and its middle.
const stallTraceDelay = 8 * time.Second

// startFlight starts the execution trace flight recorder when stall dumps
// are on. The caller stops the returned recorder after the window. The
// recorder is nil when dumps are off.
func (l *load) startFlight() *trace.FlightRecorder {
	// Skip the recorder when dumps are off.
	if l.conf.StallDumpDir == "" || l.conf.StallThreshold <= 0 {
		return nil
	}

	// Keep the trace of the last windows of the load in memory.
	fr := trace.NewFlightRecorder(trace.FlightRecorderConfig{MinAge: 3 * stallTraceDelay, MaxBytes: 512 << 20})
	if err := fr.Start(); err != nil {
		l.le.WithError(err).Warn("start flight recorder")
		return nil
	}
	l.flight = fr
	return fr
}

// writeFlight writes the flight recorder's window to the stall dump
// directory.
func (l *load) writeFlight() {
	// Write the window to a file in the dump directory.
	f, err := os.Create(filepath.Join(l.conf.StallDumpDir, "stall-trace.out"))
	if err != nil {
		l.le.WithError(err).Warn("create stall trace")
		return
	}
	defer f.Close()
	if _, err := l.flight.WriteTo(f); err != nil {
		l.le.WithError(err).Warn("write stall trace")
	}
	l.le.Info("wrote stall trace")
}

// watchStall starts a timer that records a stall when the write still runs
// after the configured stall threshold. The caller stops the
// returned timer when the write ends. It returns nil when dumps are off.
func (l *load) watchStall(seq uint64) *time.Timer {
	if l.conf.StallDumpDir == "" || l.conf.StallThreshold <= 0 {
		return nil
	}
	return time.AfterFunc(l.conf.StallThreshold, func() { l.dumpStall(seq) })
}

// dumpStall records that write seq stalled: the first stall writes the
// flight recorder's window, and with StallStacks each stall up to the bound
// writes every goroutine's stack.
func (l *load) dumpStall(seq uint64) {
	// Skip dumps past the bound and outside the measured window.
	if !l.measuring.Load() {
		return
	}
	n := l.stallDumps.Add(1)
	if n == 1 && l.flight != nil {
		time.AfterFunc(stallTraceDelay, l.writeFlight)
	}
	if !l.conf.StallStacks || n > maxStallDumps {
		l.le.WithField("write", seq).Info("write stalled")
		return
	}

	// Write the goroutine stacks to a file named by the dump and the write.
	name := "stall-" + strconv.FormatUint(n, 10) + "-write-" + strconv.FormatUint(seq, 10) + ".txt"
	f, err := os.Create(filepath.Join(l.conf.StallDumpDir, name))
	if err != nil {
		l.le.WithError(err).Warn("create stall dump")
		return
	}
	defer f.Close()
	if err := pprof.Lookup("goroutine").WriteTo(f, 2); err != nil {
		l.le.WithError(err).Warn("write stall dump")
	}
	l.le.WithField("dump", name).Info("write stalled; dumped goroutines")
}
