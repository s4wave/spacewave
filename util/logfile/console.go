package logfile

import (
	"io"
	"sync"

	"github.com/sirupsen/logrus"
)

// ConsoleHook is a logrus hook that writes synchronously to a writer
// with its own level threshold, independent of the Logger level.
type ConsoleHook struct {
	writer    io.Writer
	formatter logrus.Formatter
	level     logrus.Level
	mu        sync.Mutex
}

// NewConsoleHook creates a hook that writes synchronously to w.
func NewConsoleHook(w io.Writer, formatter logrus.Formatter, level logrus.Level) *ConsoleHook {
	return &ConsoleHook{
		writer:    w,
		formatter: formatter,
		level:     level,
	}
}

// Levels returns the log levels this hook is interested in.
func (h *ConsoleHook) Levels() []logrus.Level {
	var levels []logrus.Level
	for _, lvl := range logrus.AllLevels {
		if lvl <= h.level {
			levels = append(levels, lvl)
		}
	}
	return levels
}

// Fire formats and writes the entry synchronously.
func (h *ConsoleHook) Fire(entry *logrus.Entry) error {
	data, err := h.formatter.Format(entry)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err = h.writer.Write(data)
	return err
}
