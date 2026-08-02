package volume_redis

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/coord/conformance"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
)

// scriptedRedisConn replies to Do calls from a fixed queue.
type scriptedRedisConn struct {
	mtx     sync.Mutex
	replies []any
}

func (c *scriptedRedisConn) Close() error {
	return nil
}

func (*scriptedRedisConn) Err() error {
	return nil
}

func (c *scriptedRedisConn) Do(command string, _ ...any) (any, error) {
	if command == "" {
		return nil, nil
	}
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if len(c.replies) == 0 {
		return nil, errors.New("scripted redis conn exhausted")
	}
	reply := c.replies[0]
	c.replies = c.replies[1:]
	return reply, nil
}

func (*scriptedRedisConn) Send(string, ...any) error {
	return nil
}

func (*scriptedRedisConn) Flush() error {
	return nil
}

func (*scriptedRedisConn) Receive() (any, error) {
	return nil, nil
}

func newScriptedCoordinator(t *testing.T, replies ...any) *Coordinator {
	t.Helper()
	conn := &scriptedRedisConn{replies: replies}
	pool := &redis.Pool{
		Dial: func() (redis.Conn, error) {
			return conn, nil
		},
	}
	t.Cleanup(func() { pool.Close() })
	return NewCoordinator(pool, "store-id", coord_inmem.NewCoordinator())
}

// stopKeepalive cancels the lease keepalive so tests do not leak its goroutine.
func stopKeepalive(t *testing.T, wl coord.WriteLease) {
	t.Helper()
	l := wl.(*lease)
	l.cancel()
	select {
	case <-l.keepaliveDone:
	case <-time.After(time.Second):
		t.Fatal("keepalive did not stop")
	}
}

func TestCoordinatorKeyedTryAcquireContention(t *testing.T) {
	ctx := context.Background()
	c := newScriptedCoordinator(t, "OK", nil)
	scope := coord.Scope{VolumeID: "volume-a", ParticipantID: "a", Key: "world-1"}

	held, ok, err := c.TryAcquireWriteLease(ctx, scope)
	if err != nil || !ok {
		t.Fatalf("first acquire: ok=%v err=%v", ok, err)
	}
	t.Cleanup(func() { stopKeepalive(t, held) })

	contender, ok, err := c.TryAcquireWriteLease(ctx, scope)
	if err != nil {
		t.Fatalf("contended acquire error: %v", err)
	}
	if ok || contender != nil {
		t.Fatalf("contended acquire = (%v, %v), want (nil, false)", contender, ok)
	}
}

func TestCoordinatorKeyedCapabilityDetectsLossWithoutGenerations(t *testing.T) {
	ctx := context.Background()
	c := newScriptedCoordinator(t, "OK")
	scope := coord.Scope{VolumeID: "volume-a", ParticipantID: "a", Key: "world-1"}

	capability, err := c.Capability(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !capability.Supported || capability.Backend != coord.BackendKindRedis {
		t.Fatalf("unexpected capability: %#v", capability)
	}
	if capability.Generations {
		t.Fatalf("keyed capability declares generations: %#v", capability)
	}
	if !capability.DetectsLoss {
		t.Fatalf("keyed capability does not declare loss detection: %#v", capability)
	}

	if _, err := c.Snapshot(ctx, scope); !errors.Is(err, coord.ErrUnsupported) {
		t.Fatalf("keyed Snapshot error = %v, want ErrUnsupported", err)
	}
	if _, err := c.Watch(ctx, scope, 0); !errors.Is(err, coord.ErrUnsupported) {
		t.Fatalf("keyed Watch error = %v, want ErrUnsupported", err)
	}

	held, ok, err := c.TryAcquireWriteLease(ctx, scope)
	if err != nil || !ok {
		t.Fatalf("acquire: ok=%v err=%v", ok, err)
	}
	t.Cleanup(func() { stopKeepalive(t, held) })
	if _, err := held.Refresh(ctx); !errors.Is(err, coord.ErrUnsupported) {
		t.Fatalf("keyed Refresh error = %v, want ErrUnsupported", err)
	}
	if _, err := held.Publish(ctx, coord.Event{}); !errors.Is(err, coord.ErrUnsupported) {
		t.Fatalf("keyed Publish error = %v, want ErrUnsupported", err)
	}
}

func TestCoordinatorDetectedLossContract(t *testing.T) {
	ctx := context.Background()
	c := newScriptedCoordinator(t, "OK")
	scope := coord.Scope{VolumeID: "volume-loss", Key: "world-loss"}
	capability, err := c.Capability(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	held, ok, err := c.TryAcquireWriteLease(ctx, scope)
	if err != nil || !ok {
		t.Fatalf("acquire: ok=%v err=%v", ok, err)
	}
	lossErr := errors.New("redis hold severed")
	conformance.CheckDetectedLoss(t, capability, held, func() {
		held.(*lease).markLost(lossErr)
	})
	if !errors.Is(held.Err(), lossErr) {
		t.Fatalf("lease error = %v, want %v", held.Err(), lossErr)
	}
}

func TestCoordinatorObjectStoreScopeDelegatesToInner(t *testing.T) {
	ctx := context.Background()
	inner := coord_inmem.NewCoordinator()
	c := NewCoordinator(nil, "store-id", inner)
	scope := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "a",
	}

	capability, err := c.Capability(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if capability.Backend != coord.BackendKindInMemory || !capability.Generations {
		t.Fatalf("delegated capability = %#v", capability)
	}

	held, ok, err := c.TryAcquireWriteLease(ctx, scope)
	if err != nil || !ok {
		t.Fatalf("acquire: ok=%v err=%v", ok, err)
	}
	defer held.Release(ctx)

	if _, ok, err := inner.TryAcquireWriteLease(ctx, scope); err != nil || ok {
		t.Fatalf("inner acquired scope held through delegation: ok=%v err=%v", ok, err)
	}
}
