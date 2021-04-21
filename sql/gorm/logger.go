package sql_gorm

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm/logger"
)

// Logger implements the gorm logger interface.
type Logger struct {
	le   *logrus.Entry
	mode logger.LogLevel
}

// NewLogger constructs a new logger.
func NewLogger(le *logrus.Entry) *Logger {
	return &Logger{le: le, mode: logger.Info}
}

// LogMode sets the log level.
func (l *Logger) LogMode(ll logger.LogLevel) logger.Interface {
	return &Logger{
		le:   l.le,
		mode: ll,
	}
}

// Info logs an info message
func (l *Logger) Info(_ context.Context, fmt string, args ...interface{}) {
	l.le.Infof(fmt, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(_ context.Context, fmt string, args ...interface{}) {
	l.le.Warnf(fmt, args...)
}

// Error logs an error message
func (l *Logger) Error(_ context.Context, fmt string, args ...interface{}) {
	l.le.Errorf(fmt, args...)
}

var (
	slowThreshold = time.Millisecond * 200
	traceStr      = "[%.3fms] [rows:%v] %s"
	traceWarnStr  = "%s: [%.3fms] [rows:%v] %s"
	traceErrStr   = "%s: [%.3fms] [rows:%v] %s"
)

// Trace traces an operation.
func (l *Logger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.mode > 0 {
		le := l.le
		elapsed := time.Since(begin)
		switch {
		case err != nil && l.mode >= logger.Error:
			sql, rows := fc()
			if rows == -1 {
				le.Errorf(traceErrStr, err, float64(elapsed.Nanoseconds())/1e6, "-", sql)
			} else {
				le.Errorf(traceErrStr, err, float64(elapsed.Nanoseconds())/1e6, rows, sql)
			}
		case elapsed > slowThreshold && l.mode >= logger.Warn:
			sql, rows := fc()
			slowLog := fmt.Sprintf("SLOW SQL >= %v", slowThreshold)
			if rows == -1 {
				le.Warnf(traceWarnStr, slowLog, float64(elapsed.Nanoseconds())/1e6, "-", sql)
			} else {
				le.Warnf(traceWarnStr, slowLog, float64(elapsed.Nanoseconds())/1e6, rows, sql)
			}
		case l.mode >= logger.Info:
			sql, rows := fc()
			if rows == -1 {
				le.Debugf(traceStr, float64(elapsed.Nanoseconds())/1e6, "-", sql)
			} else {
				le.Debugf(traceStr, float64(elapsed.Nanoseconds())/1e6, rows, sql)
			}
		}
	}
}

// _ is a type assertion
var _ logger.Interface = ((*Logger)(nil))
