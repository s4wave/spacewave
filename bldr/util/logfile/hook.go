// Package logfile provides file-based logging for bldr via logrus hooks.
//
// Each log file destination is described by a spec string that can be
// passed via the --log-file CLI flag or the BLDR_LOG_FILE environment
// variable.
//
// # Spec Syntax
//
// A spec string uses semicolon-separated key=value pairs:
//
//	level=DEBUG;format=json;path=.bldr/logs/{ts}.log
//
// Keys:
//   - level: logrus level threshold (default: DEBUG)
//   - format: "text" or "json" (default: text)
//   - path: file path, supports timestamp templates
//
// A short form with just the path is also accepted:
//
//	.bldr/logs/{ts}.log
//
// The special value "none" disables file logging (useful to override
// dev mode auto-enable via BLDR_LOG_FILE=none).
//
// # Timestamp Templates
//
// Templates in the path are expanded once at process start:
//   - {ts}: YYYYMMDD-HHMMSS (e.g., 20260222-143052)
//   - {YYYY}: 4-digit year
//   - {MM}: 2-digit month (zero-padded)
//   - {DD}: 2-digit day (zero-padded)
//   - {HH}: 2-digit hour (zero-padded, 24h)
//   - {mm}: 2-digit minute (zero-padded)
//   - {ss}: 2-digit second (zero-padded)
//
// # Dev Mode
//
// In dev mode (--build-type dev), file logging is auto-enabled with the
// default spec "level=DEBUG;path=.bldr/logs/{ts}.log". Set
// BLDR_LOG_FILE=none or --log-file none to disable.
package logfile

import (
	"io"
	"sync"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/sirupsen/logrus"
)

// FileHook is a logrus hook that writes log entries to a file.
type FileHook struct {
	writer    io.Writer
	formatter logrus.Formatter
	level     logrus.Level
	bcast     broadcast.Broadcast
	buf       []*logrus.Entry
	// closed is set under bcast once Close starts. Entries fired afterward
	// are dropped.
	closed    bool
	closeOnce sync.Once
	done      chan struct{}
	drained   chan struct{}
}

// NewFileHook creates a hook that writes to the given writer.
// Exported for testing with bytes.Buffer.
func NewFileHook(w io.Writer, level logrus.Level, format string) *FileHook {
	// Choose the formatter for the file hook records.
	var formatter logrus.Formatter
	if format == "json" {
		formatter = &logrus.JSONFormatter{}
	} else {
		formatter = &logrus.TextFormatter{
			DisableColors:    true,
			DisableTimestamp: false,
		}
	}

	// Start the file hook with its asynchronous writer.
	h := &FileHook{
		writer:    w,
		formatter: formatter,
		level:     level,
		done:      make(chan struct{}),
		drained:   make(chan struct{}),
	}
	go h.writeLoop()
	return h
}

// Levels returns the log levels this hook is interested in.
func (h *FileHook) Levels() []logrus.Level {
	var levels []logrus.Level
	for _, lvl := range logrus.AllLevels {
		if lvl <= h.level {
			levels = append(levels, lvl)
		}
	}
	return levels
}

// Fire is called by logrus when a log entry is fired.
// It copies the entry to avoid races with logrus reusing the entry buffer.
// Entries fired after Close are dropped: the hook stays attached to the
// logger (logrus cannot detach a single hook) but nothing drains it.
func (h *FileHook) Fire(entry *logrus.Entry) error {
	// Copy the log record before logrus reuses its entry.
	cp := entry.Dup()
	cp.Level = entry.Level
	cp.Message = entry.Message

	// Queue the copied record while the file hook is open.
	h.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if h.closed {
			return
		}
		h.buf = append(h.buf, cp)
		bcast()
	})
	return nil
}

// writeLoop is the background goroutine that drains buffered entries.
func (h *FileHook) writeLoop() {
	defer close(h.drained)
	for {
		var entries []*logrus.Entry
		var waitCh <-chan struct{}
		h.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			entries = h.buf
			h.buf = nil
			waitCh = getWaitCh()
		})

		for _, entry := range entries {
			data, err := h.formatter.Format(entry)
			if err == nil {
				_, _ = h.writer.Write(data)
			}
		}

		select {
		case <-h.done:
			// drain remaining entries
			h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				entries = h.buf
				h.buf = nil
			})
			for _, entry := range entries {
				data, err := h.formatter.Format(entry)
				if err == nil {
					_, _ = h.writer.Write(data)
				}
			}
			return
		case <-waitCh:
		}
	}
}

// Close signals the writer goroutine to drain and stop, then waits
// for completion. If the writer implements io.Closer, it is closed.
// Close is idempotent.
func (h *FileHook) Close() {
	h.closeOnce.Do(func() {
		h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			h.closed = true
		})
		close(h.done)
		<-h.drained
		if c, ok := h.writer.(io.Closer); ok {
			_ = c.Close()
		}
	})
}
