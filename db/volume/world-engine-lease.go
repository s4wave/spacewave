package volume

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
)

var (
	// ErrWorldEngineLeaseKeyEmpty is returned when a world engine lease key is empty.
	ErrWorldEngineLeaseKeyEmpty = errors.New("world engine lease key cannot be empty")
)

// WorldEngineLeaseHeldError reports that another holder owns a world engine key.
type WorldEngineLeaseHeldError struct {
	// Key identifies the world engine lease that is already held.
	Key string
}

// Error returns the already-held lease error.
func (e *WorldEngineLeaseHeldError) Error() string {
	return "world engine lease already held for key " + e.Key
}

// WorldEngineLease is a held world engine lease.
type WorldEngineLease interface {
	// Done returns a channel closed when the lease is released or lost.
	Done() <-chan struct{}
	// Err returns the loss error after Done is closed, or nil for a held or
	// cleanly released lease.
	Err() error
	// Release releases the lease.
	Release() error
}

// WorldEngineLeaseProvider acquires keyed leases owned by a Volume.
type WorldEngineLeaseProvider interface {
	// AcquireWorldEngineLease attempts to acquire a world engine lease without blocking.
	AcquireWorldEngineLease(ctx context.Context, key string) (WorldEngineLease, error)
}

// NewInMemoryWorldEngineLeaseProvider constructs a single-process keyed lease provider.
func NewInMemoryWorldEngineLeaseProvider() WorldEngineLeaseProvider {
	return &inMemoryWorldEngineLeaseProvider{held: make(map[string]struct{})}
}

func worldEngineLeaseDigest(storeID, key string) string {
	digest := sha256.Sum256([]byte(storeID + "\x00" + key))
	return hex.EncodeToString(digest[:])
}

type inMemoryWorldEngineLeaseProvider struct {
	mtx  sync.Mutex
	held map[string]struct{}
}

func (p *inMemoryWorldEngineLeaseProvider) AcquireWorldEngineLease(
	ctx context.Context,
	key string,
) (WorldEngineLease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrWorldEngineLeaseKeyEmpty
	}

	p.mtx.Lock()
	defer p.mtx.Unlock()
	if _, ok := p.held[key]; ok {
		return nil, &WorldEngineLeaseHeldError{Key: key}
	}
	p.held[key] = struct{}{}
	return &inMemoryWorldEngineLease{
		provider: p,
		key:      key,
		done:     make(chan struct{}),
	}, nil
}

type inMemoryWorldEngineLease struct {
	provider *inMemoryWorldEngineLeaseProvider
	key      string
	done     chan struct{}
	once     sync.Once
}

func (l *inMemoryWorldEngineLease) Done() <-chan struct{} {
	return l.done
}

func (*inMemoryWorldEngineLease) Err() error {
	return nil
}

func (l *inMemoryWorldEngineLease) Release() error {
	l.once.Do(func() {
		l.provider.mtx.Lock()
		delete(l.provider.held, l.key)
		l.provider.mtx.Unlock()
		close(l.done)
	})
	return nil
}
