//go:build js

package volume

import (
	"context"
	"errors"
	"sync"

	"github.com/s4wave/spacewave/db/opfs"
)

const worldEngineLeaseWebLockPrefix = "spacewave/world-engine-lease/"

// NewFileWorldEngineLeaseProvider constructs a cross-context WebLock-backed
// lease provider for browser storage.
func NewFileWorldEngineLeaseProvider(_, storeID string) WorldEngineLeaseProvider {
	return &webLockWorldEngineLeaseProvider{storeID: storeID}
}

type webLockWorldEngineLeaseProvider struct {
	storeID string
}

func (p *webLockWorldEngineLeaseProvider) AcquireWorldEngineLease(
	ctx context.Context,
	key string,
) (WorldEngineLease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrWorldEngineLeaseKeyEmpty
	}
	if p.storeID == "" {
		return nil, errors.New("world engine lease backing store identity cannot be empty")
	}

	lockName := worldEngineLeaseWebLockPrefix + worldEngineLeaseDigest(p.storeID, key)
	release, acquired, err := opfs.AcquireWebLockIfAvailable(lockName, true)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, &WorldEngineLeaseHeldError{Key: key}
	}
	return &webLockWorldEngineLease{
		release: release,
		done:    make(chan struct{}),
	}, nil
}

type webLockWorldEngineLease struct {
	release func()
	done    chan struct{}
	once    sync.Once
}

func (l *webLockWorldEngineLease) Done() <-chan struct{} {
	return l.done
}

func (*webLockWorldEngineLease) Err() error {
	return nil
}

func (l *webLockWorldEngineLease) Release() error {
	l.once.Do(func() {
		l.release()
		close(l.done)
	})
	return nil
}

var _ WorldEngineLeaseProvider = (*webLockWorldEngineLeaseProvider)(nil)
var _ WorldEngineLease = (*webLockWorldEngineLease)(nil)
