package volume_redis

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/s4wave/spacewave/db/coord"
)

const (
	redisLeaseReleaseScriptSource = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0`
	redisLeaseRenewScriptSource = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0`
)

var (
	redisLeaseReleaseScript = redis.NewScript(1, redisLeaseReleaseScriptSource)
	redisLeaseRenewScript   = redis.NewScript(1, redisLeaseRenewScriptSource)
	errRedisLeaseLost       = errors.New("redis lease lost")
)

// lease holds one TTL-guarded redis key, renewed by a keepalive goroutine
// until Release or renewal failure.
type lease struct {
	pool          *redis.Pool
	key           string
	value         []byte
	cancel        context.CancelFunc
	done          chan struct{}
	keepaliveDone chan struct{}
	releaseDone   chan struct{}
	mtx           sync.Mutex
	released      bool
	doneClosed    bool
	releaseErr    error
	lossErr       error
}

// Done returns a channel closed when the lease is released or its renewal
// fails.
func (l *lease) Done() <-chan struct{} {
	return l.done
}

// Err returns the renewal error that lost the lease, or nil for a held or
// cleanly released lease.
func (l *lease) Err() error {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	return l.lossErr
}

// Refresh returns ErrUnsupported: keyed scopes carry no generations.
func (l *lease) Refresh(ctx context.Context) (*coord.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, coord.ErrUnsupported
}

// Publish returns ErrUnsupported: keyed scopes carry no generations.
func (l *lease) Publish(ctx context.Context, event coord.Event) (*coord.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, coord.ErrUnsupported
}

func (l *lease) keepalive(ctx context.Context) {
	ticker := time.NewTicker(redisLeaseRenew)
	defer ticker.Stop()
	l.keepaliveWith(ctx, ticker.C, l.renew)
}

func (l *lease) keepaliveWith(
	ctx context.Context,
	renewCh <-chan time.Time,
	renew func() error,
) {
	defer close(l.keepaliveDone)
	for {
		select {
		case <-ctx.Done():
			return
		case <-renewCh:
			if err := renew(); err != nil {
				l.markLost(err)
				return
			}
		}
	}
}

// markLost records the renewal error and closes the lease's done channel once.
func (l *lease) markLost(err error) {
	// Record the loss error and close done under the lease lock.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if l.released || l.lossErr != nil {
		return
	}
	l.lossErr = err
	if !l.doneClosed {
		l.doneClosed = true
		close(l.done)
	}
}

// redisLeaseDoScript evaluates a script by hash, falling back to EVAL on NOSCRIPT.
func redisLeaseDoScript(
	conn redis.Conn,
	deadline time.Time,
	script *redis.Script,
	source string,
	args ...any,
) (any, error) {
	// Build the EVALSHA arguments and check the deadline.
	evalArgs := make([]any, 2+len(args))
	evalArgs[0] = script.Hash()
	evalArgs[1] = 1
	copy(evalArgs[2:], args)

	// Compute the remaining time before the deadline.
	timeout := time.Until(deadline)
	if timeout <= 0 {
		return nil, context.DeadlineExceeded
	}

	// Retry with the full source when the server does not hold the script hash.
	reply, err := redis.DoWithTimeout(conn, timeout, "EVALSHA", evalArgs...)
	if redisErr, ok := err.(redis.Error); ok &&
		strings.HasPrefix(string(redisErr), "NOSCRIPT ") {
		evalArgs[0] = source
		timeout = time.Until(deadline)
		if timeout <= 0 {
			return nil, context.DeadlineExceeded
		}
		return redis.DoWithTimeout(conn, timeout, "EVAL", evalArgs...)
	}
	return reply, err
}

// renew extends the lease's TTL with the compare-and-set script.
func (l *lease) renew() error {
	// Borrow a connection bounded by the renewal window.
	deadline := time.Now().Add(redisLeaseRenew)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	// Borrow a connection bounded by the renewal window.
	conn, err := l.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Run the renewal script and require a renewed count of one.
	reply, err := redisLeaseDoScript(
		conn,
		deadline,
		redisLeaseRenewScript,
		redisLeaseRenewScriptSource,
		l.key,
		l.value,
		redisLeaseTTL.Milliseconds(),
	)
	if err != nil {
		return err
	}
	renewed, err := redis.Int(reply, nil)
	if err != nil {
		return err
	}
	if renewed != 1 {
		return errRedisLeaseLost
	}
	return nil
}

// Release stops the keepalive and deletes the redis key when this holder
// still owns it. The compare-and-delete round trip is bounded by the renewal
// window rather than the caller's context, so Release completes regardless
// of cancellation.
func (l *lease) Release(context.Context) error {
	// Join the earlier release when this lease is already released.
	l.mtx.Lock()
	if l.released {
		releaseDone := l.releaseDone
		l.mtx.Unlock()
		<-releaseDone
		l.mtx.Lock()
		defer l.mtx.Unlock()
		return l.releaseErr
	}
	l.released = true
	l.mtx.Unlock()

	// Stop the keepalive loop and wait for it to exit.
	l.cancel()
	<-l.keepaliveDone

	// Check whether the lease was lost before the release.
	l.mtx.Lock()
	lost := l.lossErr != nil
	l.mtx.Unlock()

	// Delete the redis key when the lease was still held.
	var releaseErr error
	if !lost {
		deadline := time.Now().Add(redisLeaseRenew)
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()

		conn, err := l.pool.GetContext(ctx)
		if err != nil {
			releaseErr = err
		} else {
			_, releaseErr = redisLeaseDoScript(
				conn,
				deadline,
				redisLeaseReleaseScript,
				redisLeaseReleaseScriptSource,
				l.key,
				l.value,
			)
			_ = conn.Close()
		}
	}

	// Record the release result and close the lease's done channels.
	l.mtx.Lock()
	l.releaseErr = releaseErr
	if releaseErr != nil && l.lossErr == nil {
		l.lossErr = releaseErr
	}
	if !l.doneClosed {
		l.doneClosed = true
		close(l.done)
	}
	close(l.releaseDone)
	l.mtx.Unlock()
	return releaseErr
}

// _ is a type assertion
var _ coord.WriteLease = (*lease)(nil)
