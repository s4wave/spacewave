package volume_redis

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
)

func TestRedisWorldEngineLeaseRenewalLossSignalsDone(t *testing.T) {
	lease := &redisWorldEngineLease{
		cancel:        func() {},
		done:          make(chan struct{}),
		keepaliveDone: make(chan struct{}),
	}
	renewCh := make(chan time.Time, 1)
	renewErr := errors.New("renewal failed")

	go lease.keepaliveWith(context.Background(), renewCh, func() error {
		return renewErr
	})
	renewCh <- time.Time{}

	select {
	case <-lease.Done():
	case <-time.After(time.Second):
		t.Fatal("lease loss did not close Done")
	}
	if !errors.Is(lease.Err(), renewErr) {
		t.Fatalf("lease error = %v, want %v", lease.Err(), renewErr)
	}
}

type timeoutRedisConn struct {
	closed    chan struct{}
	closeOnce sync.Once
	timeoutCh chan time.Duration
}

func (c *timeoutRedisConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (*timeoutRedisConn) Err() error {
	return nil
}

func (c *timeoutRedisConn) Do(command string, _ ...any) (any, error) {
	if command == "" {
		return nil, nil
	}
	<-c.closed
	return nil, errors.New("redis connection closed")
}

func (c *timeoutRedisConn) DoWithTimeout(
	timeout time.Duration,
	_ string,
	_ ...any,
) (any, error) {
	c.timeoutCh <- timeout
	return nil, context.DeadlineExceeded
}

func (*timeoutRedisConn) Send(string, ...any) error {
	return nil
}

func (*timeoutRedisConn) Flush() error {
	return nil
}

func (*timeoutRedisConn) Receive() (any, error) {
	return nil, nil
}

func (*timeoutRedisConn) ReceiveWithTimeout(time.Duration) (any, error) {
	return nil, context.DeadlineExceeded
}

func newTimeoutRedisLease(t *testing.T) (
	*redisWorldEngineLease,
	*timeoutRedisConn,
	*redis.Pool,
) {
	t.Helper()
	conn := &timeoutRedisConn{
		closed:    make(chan struct{}),
		timeoutCh: make(chan time.Duration, 2),
	}
	pool := &redis.Pool{
		Dial: func() (redis.Conn, error) {
			return conn, nil
		},
	}
	lease := &redisWorldEngineLease{
		pool:          pool,
		key:           "lease-key",
		value:         []byte("lease-value"),
		cancel:        func() {},
		done:          make(chan struct{}),
		keepaliveDone: make(chan struct{}),
	}
	t.Cleanup(func() {
		_ = conn.Close()
		pool.Close()
	})
	return lease, conn, pool
}

func TestRedisWorldEngineLeaseHangingRenewalSignalsLoss(t *testing.T) {
	lease, conn, _ := newTimeoutRedisLease(t)
	renewCh := make(chan time.Time, 1)

	go lease.keepaliveWith(context.Background(), renewCh, lease.renew)
	renewCh <- time.Now()

	select {
	case <-lease.Done():
	case <-time.After(time.Second):
		t.Fatal("hanging lease renewal did not close Done")
	}
	if !errors.Is(lease.Err(), context.DeadlineExceeded) {
		t.Fatalf("lease error = %v, want context deadline exceeded", lease.Err())
	}
	select {
	case timeout := <-conn.timeoutCh:
		if timeout <= 0 || timeout >= redisWorldEngineLeaseTTL {
			t.Fatalf("renewal timeout = %s, want between zero and lease TTL", timeout)
		}
	default:
		t.Fatal("renewal did not use a bounded Redis command timeout")
	}
}

func TestRedisWorldEngineLeaseReleaseTimeoutClosesDone(t *testing.T) {
	lease, conn, _ := newTimeoutRedisLease(t)
	close(lease.keepaliveDone)

	err := lease.Release()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("release error = %v, want context deadline exceeded", err)
	}
	select {
	case <-lease.Done():
	default:
		t.Fatal("timed-out release did not close Done")
	}
	select {
	case timeout := <-conn.timeoutCh:
		if timeout <= 0 || timeout >= redisWorldEngineLeaseTTL {
			t.Fatalf("release timeout = %s, want between zero and lease TTL", timeout)
		}
	default:
		t.Fatal("release did not use a bounded Redis command timeout")
	}
}
