package volume_badger

import (
	bdb "github.com/dgraph-io/badger/v4"
	"github.com/sirupsen/logrus"
)

type badgerLogger struct {
	le        *logrus.Entry
	withDebug bool
}

// newBadgerLogger builds a new badger logger
func newBadgerLogger(le *logrus.Entry, withDebug bool) *badgerLogger {
	return &badgerLogger{le: le, withDebug: withDebug}
}

func (l *badgerLogger) Errorf(fmt string, args ...any) {
	l.le.Errorf(fmt, args...)
}

func (l *badgerLogger) Warningf(fmt string, args ...any) {
	l.le.Warnf(fmt, args...)
}

func (l *badgerLogger) Infof(fmt string, args ...any) {
	l.le.Infof(fmt, args...)
}

func (l *badgerLogger) Debugf(fmt string, args ...any) {
	if l.withDebug {
		l.le.Debugf(fmt, args...)
	}
}

// _ is a type assertion
var _ bdb.Logger = ((*badgerLogger)(nil))
