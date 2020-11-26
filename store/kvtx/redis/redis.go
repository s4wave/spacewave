package store_kvtx_redis

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/gomodule/redigo/redis"
)

// Store is a redis database key-value store.
type Store struct {
	ctx      context.Context
	pool     *redis.Pool
	writeMtx sync.Mutex
}

// NewStore constructs a new key-value store from a Redis pool.
// If logger is set, wraps conn with a logging connection.
func NewStore(ctx context.Context, pool *redis.Pool) *Store {
	return &Store{ctx: ctx, pool: pool}
}

// Connect connects to a redis store. Uses a client pool.
func Connect(
	ctx context.Context,
	rawurl string,
	options ...redis.DialOption,
) (*Store, error) {
	pool := &redis.Pool{
		MaxIdle:         2,
		IdleTimeout:     60 * time.Second,
		MaxConnLifetime: 15 * time.Minute,

		Dial: func() (redis.Conn, error) {
			return redis.DialURL(rawurl, options...)
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}

	conn, err := pool.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.Err(); err != nil {
		return nil, err
	}
	return NewStore(ctx, pool), nil
}

// SetContext sets the context used to get clients for the next transaction.
func (s *Store) SetContext(ctx context.Context) {
	s.ctx = ctx
}

// GetPool returns the redis pool.
func (s *Store) GetPool() *redis.Pool {
	return s.pool
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (s *Store) NewTransaction(write bool) (kvtx.Tx, error) {
	conn, err := s.buildConn(s.ctx, false)
	if err != nil {
		return nil, err
	}
	if write {
		s.writeMtx.Lock()
	}
	return s.newTx(conn, write), nil
}

// buildConn builds a new connetion.
func (s *Store) buildConn(ctx context.Context, write bool) (redis.Conn, error) {
	conn, err := s.pool.GetContext(s.ctx)
	if err != nil {
		return nil, err
	}
	if err := conn.Err(); err != nil {
		return nil, err
	}
	// Note: redigo is smart, and automatically cancels the MULTI if the transaction fails.
	// it may be possible to send multi and defer reading the reply.
	if write {
		_, err = conn.Do("MULTI")
		if err != nil {
			return nil, err
		}
	}
	return conn, err
}

// Execute executes the given store.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (s *Store) Execute(ctx context.Context) error {
	return nil
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
