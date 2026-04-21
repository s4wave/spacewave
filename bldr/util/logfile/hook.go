package logfile

import (
	"io"

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
	done      chan struct{}
	drained   chan struct{}
}

// NewFileHook creates a hook that writes to the given writer.
// Exported for testing with bytes.Buffer.
func NewFileHook(w io.Writer, level logrus.Level, format string) *FileHook {
	var formatter logrus.Formatter
	if format == "json" {
		formatter = &logrus.JSONFormatter{}
	} else {
		formatter = &logrus.TextFormatter{
			DisableColors:    true,
			DisableTimestamp: false,
		}
	}

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
func (h *FileHook) Fire(entry *logrus.Entry) error {
	cp := entry.Dup()
	cp.Level = entry.Level
	cp.Message = entry.Message

	h.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
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
			// drain remaining
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
func (h *FileHook) Close() {
	close(h.done)
	<-h.drained
	if c, ok := h.writer.(io.Closer); ok {
		_ = c.Close()
	}
}
