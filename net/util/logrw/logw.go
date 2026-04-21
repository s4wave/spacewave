package logrw

import (
	"io"

	"github.com/sirupsen/logrus"
)

// LogWriter wraps a io.Writer with a logger.
type LogWriter struct {
	le         *logrus.Entry
	underlying io.Writer
}

func NewLogWriter(le *logrus.Entry, underlying io.Writer) *LogWriter {
	return &LogWriter{
		le:         le,
		underlying: underlying,
	}
}

// Write writes data to the connection.
// Write can be made to time out and return an error after a fixed
// time limit; see SetDeadline and SetWriteDeadline.
func (c *LogWriter) Write(b []byte) (n int, err error) {
	c.le.Debugf("write(%d): %v", len(b), b)
	n, err = c.underlying.Write(b)
	if err != nil {
		c.le.Warnf("write(%d) => errored %v", len(b), err.Error())
	}
	return
}

// _ is a type assertion
var _ io.Writer = ((*LogWriter)(nil))
