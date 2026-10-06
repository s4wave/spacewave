//go:build !js

package webrtc_test

import (
	"sync"

	"github.com/sirupsen/logrus"
)

// logWatch is a logrus hook that reports the first entry with a message and a
// transport address, so a test can wait for a state the transport logs.
type logWatch struct {
	// msg is the message to match.
	msg string
	// transportIP is the transport-ip field to match.
	transportIP string
	// once guards closing seen.
	once sync.Once
	// seen is closed when a matching entry is logged.
	seen chan struct{}
}

// newLogWatch constructs a watch for msg logged by the transport at
// transportIP.
func newLogWatch(msg, transportIP string) *logWatch {
	return &logWatch{msg: msg, transportIP: transportIP, seen: make(chan struct{})}
}

// Levels returns the levels the watch observes.
func (w *logWatch) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire closes seen when entry matches.
func (w *logWatch) Fire(entry *logrus.Entry) error {
	if entry.Message == w.msg && entry.Data["transport-ip"] == w.transportIP {
		w.once.Do(func() { close(w.seen) })
	}
	return nil
}

// _ is a type assertion
var _ logrus.Hook = (*logWatch)(nil)
