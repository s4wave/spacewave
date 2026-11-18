package exec

import (
	"bytes"
	"sync"

	"github.com/sirupsen/logrus"
)

// LogWriter logs each line written to it at a fixed level. It logs within
// Write, so a line is in the log once Write returns: exec.Cmd.Wait returns only
// after its copy goroutines finish, so every complete line the child wrote is
// logged by then. Flush logs a trailing line that has no newline.
type LogWriter struct {
	le    *logrus.Entry
	level logrus.Level

	mtx sync.Mutex
	buf []byte
}

// NewLogWriter constructs a LogWriter logging to le at level.
func NewLogWriter(le *logrus.Entry, level logrus.Level) *LogWriter {
	return &LogWriter{le: le, level: level}
}

// Write logs each complete line in p and keeps a partial line for later.
func (w *LogWriter) Write(p []byte) (int, error) {
	// Log every complete line now and keep the partial tail for the next write.
	w.mtx.Lock()
	defer w.mtx.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.le.Log(w.level, string(bytes.TrimSuffix(w.buf[:i], []byte{'\r'})))
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

// Flush logs a buffered partial line.
func (w *LogWriter) Flush() {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if len(w.buf) != 0 {
		w.le.Log(w.level, string(w.buf))
		w.buf = nil
	}
}
