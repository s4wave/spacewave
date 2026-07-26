package volume_redis

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/s4wave/spacewave/db/volume"
)

const (
	redisWorldEngineLeasePrefix = "spacewave/world-engine-lease/redis/"
	redisWorldEngineLeaseTTL    = 30 * time.Second
	redisWorldEngineLeaseRenew  = redisWorldEngineLeaseTTL / 3

	redisWorldEngineLeaseReleaseScriptSource = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0`
	redisWorldEngineLeaseRenewScriptSource = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0`
)

var (
	redisWorldEngineLeaseReleaseScript = redis.NewScript(
		1,
		redisWorldEngineLeaseReleaseScriptSource,
	)
	redisWorldEngineLeaseRenewScript = redis.NewScript(
		1,
		redisWorldEngineLeaseRenewScriptSource,
	)
	errRedisWorldEngineLeaseLost = errors.New("redis world engine lease lost")
)

// NewRedisWorldEngineLeaseProvider constructs a Redis-backed World Engine
// lease provider. storeID identifies the volume's Redis key namespace.
func NewRedisWorldEngineLeaseProvider(pool *redis.Pool, storeID string) volume.WorldEngineLeaseProvider {
	return &redisWorldEngineLeaseProvider{pool: pool, storeID: storeID}
}

type redisWorldEngineLeaseProvider struct {
	pool    *redis.Pool
	storeID string
}

func (p *redisWorldEngineLeaseProvider) AcquireWorldEngineLease(
	ctx context.Context,
	key string,
) (volume.WorldEngineLease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, volume.ErrWorldEngineLeaseKeyEmpty
	}
	if p.pool == nil {
		return nil, errors.New("redis world engine lease pool cannot be nil")
	}
	if p.storeID == "" {
		return nil, errors.New("redis world engine lease backing store identity cannot be empty")
	}

	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return nil, err
	}
	lockKey := redisWorldEngineLeasePrefix + redisWorldEngineLeaseDigest(p.storeID, key)
	conn, err := p.pool.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	reply, err := conn.Do(
		"SET",
		lockKey,
		value,
		"NX",
		"PX",
		redisWorldEngineLeaseTTL.Milliseconds(),
	)
	if err != nil {
		return nil, err
	}
	if reply == nil {
		return nil, &volume.WorldEngineLeaseHeldError{Key: key}
	}
	if result, err := redis.String(reply, nil); err != nil || result != "OK" {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("redis world engine lease returned unexpected acquisition result")
	}

	leaseCtx, cancel := context.WithCancel(context.Background())
	lease := &redisWorldEngineLease{
		pool:          p.pool,
		key:           lockKey,
		value:         value,
		cancel:        cancel,
		done:          make(chan struct{}),
		keepaliveDone: make(chan struct{}),
	}
	go lease.keepalive(leaseCtx)
	return lease, nil
}

func redisWorldEngineLeaseDigest(storeID, key string) string {
	digest := sha256.Sum256([]byte(storeID + "\x00" + key))
	return hex.EncodeToString(digest[:])
}

type redisWorldEngineLease struct {
	pool          *redis.Pool
	key           string
	value         []byte
	cancel        context.CancelFunc
	done          chan struct{}
	keepaliveDone chan struct{}
	doneOnce      sync.Once
	stateMtx      sync.Mutex
	releaseErr    error
	lossErr       error
	once          sync.Once
}

func (l *redisWorldEngineLease) Done() <-chan struct{} {
	return l.done
}

func (l *redisWorldEngineLease) Err() error {
	l.stateMtx.Lock()
	defer l.stateMtx.Unlock()
	return l.lossErr
}

func (l *redisWorldEngineLease) keepalive(ctx context.Context) {
	ticker := time.NewTicker(redisWorldEngineLeaseRenew)
	defer ticker.Stop()
	l.keepaliveWith(ctx, ticker.C, l.renew)
}

func (l *redisWorldEngineLease) keepaliveWith(
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

func (l *redisWorldEngineLease) markLost(err error) {
	l.stateMtx.Lock()
	l.lossErr = err
	l.stateMtx.Unlock()
	l.doneOnce.Do(func() { close(l.done) })
}

func redisWorldEngineLeaseDoScript(
	conn redis.Conn,
	deadline time.Time,
	script *redis.Script,
	source string,
	args ...any,
) (any, error) {
	evalArgs := make([]any, 2+len(args))
	evalArgs[0] = script.Hash()
	evalArgs[1] = 1
	copy(evalArgs[2:], args)

	timeout := time.Until(deadline)
	if timeout <= 0 {
		return nil, context.DeadlineExceeded
	}
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

func (l *redisWorldEngineLease) renew() error {
	deadline := time.Now().Add(redisWorldEngineLeaseRenew)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	conn, err := l.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	reply, err := redisWorldEngineLeaseDoScript(
		conn,
		deadline,
		redisWorldEngineLeaseRenewScript,
		redisWorldEngineLeaseRenewScriptSource,
		l.key,
		l.value,
		redisWorldEngineLeaseTTL.Milliseconds(),
	)
	if err != nil {
		return err
	}
	renewed, err := redis.Int(reply, nil)
	if err != nil {
		return err
	}
	if renewed != 1 {
		return errRedisWorldEngineLeaseLost
	}
	return nil
}

func (l *redisWorldEngineLease) Release() error {
	l.once.Do(func() {
		l.cancel()
		<-l.keepaliveDone
		if l.Err() != nil {
			l.doneOnce.Do(func() { close(l.done) })
			return
		}

		deadline := time.Now().Add(redisWorldEngineLeaseRenew)
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()

		conn, err := l.pool.GetContext(ctx)
		if err != nil {
			l.releaseErr = err
			l.markLost(err)
			return
		}
		defer conn.Close()
		_, l.releaseErr = redisWorldEngineLeaseDoScript(
			conn,
			deadline,
			redisWorldEngineLeaseReleaseScript,
			redisWorldEngineLeaseReleaseScriptSource,
			l.key,
			l.value,
		)
		if l.releaseErr != nil {
			l.markLost(l.releaseErr)
			return
		}
		l.doneOnce.Do(func() { close(l.done) })
	})
	return l.releaseErr
}

var (
	_ volume.WorldEngineLeaseProvider = (*redisWorldEngineLeaseProvider)(nil)
	_ volume.WorldEngineLease         = (*redisWorldEngineLease)(nil)
)
