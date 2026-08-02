package volume_redis

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/s4wave/spacewave/db/coord"
)

const (
	redisLeasePrefix = "spacewave/coord/lease/"
	redisLeaseTTL    = 30 * time.Second
	redisLeaseRenew  = redisLeaseTTL / 3
	// redisLeaseRetryDelay paces keyed acquisition retries: redis reports a
	// held key only at probe time and cannot broadcast its release here.
	redisLeaseRetryDelay = 250 * time.Millisecond
)

// Coordinator combines an inner coordinator for ObjectStore scopes with
// TTL-guarded redis keys for cross-process keyed exclusion.
type Coordinator struct {
	pool    *redis.Pool
	storeID string
	inner   coord.Coordinator
}

// NewCoordinator builds a redis coordinator. Scopes without a Key delegate
// to inner. storeID identifies the volume's redis key namespace.
func NewCoordinator(pool *redis.Pool, storeID string, inner coord.Coordinator) *Coordinator {
	return &Coordinator{pool: pool, storeID: storeID, inner: inner}
}

// Capability reports keyed redis lease support, delegating ObjectStore scopes.
func (c *Coordinator) Capability(ctx context.Context, scope coord.Scope) (*coord.Capability, error) {
	if scope.Key == "" {
		return c.inner.Capability(ctx, scope)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &coord.Capability{
		Supported:     true,
		Backend:       coord.BackendKindRedis,
		VolumeID:      scope.VolumeID,
		ObjectStoreID: scope.ObjectStoreID,
		DetectsLoss:   true,
	}, nil
}

// Snapshot delegates ObjectStore scopes; keyed scopes carry no generations.
func (c *Coordinator) Snapshot(ctx context.Context, scope coord.Scope) (*coord.Snapshot, error) {
	if scope.Key == "" {
		return c.inner.Snapshot(ctx, scope)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, coord.ErrUnsupported
}

// Watch delegates ObjectStore scopes; keyed scopes carry no event stream.
func (c *Coordinator) Watch(ctx context.Context, scope coord.Scope, afterGeneration uint64) (coord.Watch, error) {
	if scope.Key == "" {
		return c.inner.Watch(ctx, scope, afterGeneration)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, coord.ErrUnsupported
}

// TryAcquireWriteLease attempts to set the keyed redis lease without blocking.
func (c *Coordinator) TryAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, bool, error) {
	if scope.Key == "" {
		return c.inner.TryAcquireWriteLease(ctx, scope)
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if c.pool == nil {
		return nil, false, errors.New("redis coordinator pool cannot be nil")
	}
	if c.storeID == "" {
		return nil, false, errors.New("redis coordinator backing store identity cannot be empty")
	}

	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return nil, false, err
	}
	lockKey := redisLeasePrefix + redisLeaseDigest(c.storeID, scope)
	conn, err := c.pool.GetContext(ctx)
	if err != nil {
		return nil, false, err
	}
	defer conn.Close()

	reply, err := conn.Do(
		"SET",
		lockKey,
		value,
		"NX",
		"PX",
		redisLeaseTTL.Milliseconds(),
	)
	if err != nil {
		return nil, false, err
	}
	if reply == nil {
		return nil, false, nil
	}
	if result, err := redis.String(reply, nil); err != nil || result != "OK" {
		if err != nil {
			return nil, false, err
		}
		return nil, false, errors.New("redis lease returned unexpected acquisition result")
	}

	leaseCtx, cancel := context.WithCancel(context.Background())
	l := &lease{
		pool:          c.pool,
		key:           lockKey,
		value:         value,
		cancel:        cancel,
		done:          make(chan struct{}),
		keepaliveDone: make(chan struct{}),
		releaseDone:   make(chan struct{}),
	}
	go l.keepalive(leaseCtx)
	return l, true, nil
}

// WaitAcquireWriteLease waits until the keyed redis lease is available.
func (c *Coordinator) WaitAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, error) {
	if scope.Key == "" {
		return c.inner.WaitAcquireWriteLease(ctx, scope)
	}

	for {
		l, ok, err := c.TryAcquireWriteLease(ctx, scope)
		if err != nil {
			return nil, err
		}
		if ok {
			return l, nil
		}

		timer := time.NewTimer(redisLeaseRetryDelay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// redisLeaseDigest names the redis lease key for one backing store and scope.
func redisLeaseDigest(storeID string, scope coord.Scope) string {
	digest := sha256.Sum256([]byte(
		storeID + "\x00" + scope.VolumeID + "\x00" + scope.ObjectStoreID + "\x00" + scope.Key,
	))
	return hex.EncodeToString(digest[:])
}

// _ is a type assertion
var _ coord.Coordinator = (*Coordinator)(nil)
