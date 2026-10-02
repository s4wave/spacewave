package block

import (
	"context"
	"sync"
	"time"
)

type accessLogContextKey struct{}

// AccessLogger records the first read of each block, in order. It is safe for
// concurrent use.
type AccessLogger struct {
	// start is when the log started.
	start time.Time
	// mtx guards the fields below.
	mtx sync.Mutex
	// seen holds the keys of the logged refs.
	seen map[string]struct{}
	// accesses are the logged reads in first-use order.
	accesses []*Access
}

// NewAccessLogger starts an empty access log.
func NewAccessLogger() *AccessLogger {
	return &AccessLogger{start: time.Now(), seen: make(map[string]struct{})}
}

// WithAccessLogger returns a context whose block reads record to logger.
func WithAccessLogger(ctx context.Context, logger *AccessLogger) context.Context {
	return context.WithValue(ctx, accessLogContextKey{}, logger)
}

// GetAccessLogger returns the access logger of the context, or nil.
func GetAccessLogger(ctx context.Context) *AccessLogger {
	logger, _ := ctx.Value(accessLogContextKey{}).(*AccessLogger)
	return logger
}

// Snapshot returns the reads logged so far.
func (l *AccessLogger) Snapshot() *AccessLog {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	return &AccessLog{Accesses: append([]*Access(nil), l.accesses...)}
}

// record logs the read of ref unless it was read before.
func (l *AccessLogger) record(ref *BlockRef, size int) {
	// Key the ref by its stable encoding.
	key, err := ref.MarshalKey()
	if err != nil {
		return
	}

	// Append the first read of the ref.
	elapsed := time.Since(l.start).Microseconds()
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if _, ok := l.seen[string(key)]; ok {
		return
	}
	l.seen[string(key)] = struct{}{}
	l.accesses = append(l.accesses, &Access{
		Ref:       ref.Clone(),
		Size:      uint32(max(size, 0)), //nolint:gosec // blocks are far below 4 GiB.
		ElapsedUs: uint64(max(elapsed, 0)),
	})
}

// recordAccessLog records a found block read to the context's access logger.
func recordAccessLog(ctx context.Context, ref *BlockRef, size int) {
	if logger := GetAccessLogger(ctx); logger != nil {
		logger.record(ref, size)
	}
}
