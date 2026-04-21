package logrw

import (
	"io"

	"github.com/sirupsen/logrus"
)

// LogReader wraps a io.Reader with a logger.
type LogReader struct {
	le         *logrus.Entry
	underlying io.Reader
}

func NewLogReader(le *logrus.Entry, underlying io.Reader) *LogReader {
	return &LogReader{
		le:         le,
		underlying: underlying,
	}
}

// Read reads data from the connection.
// Read can be made to time out and return an error after a fixed
// time limit; see SetDeadline and SetReadDeadline.
func (c *LogReader) Read(b []byte) (n int, err error) {
	n, err = c.underlying.Read(b)
	if err != nil && !(err == io.EOF && n != 0) { //nolint:staticcheck
		c.le.Warnf("read(...) => error %v", err.Error())
	} else {
		c.le.Debugf("read(...) => %v", b[:n])
	}
	return
}

// _ is a type assertion
var _ io.Reader = ((*LogReader)(nil))
