package npm

import (
	"context"
	"os"
	"sync"
	"time"
)

type installLock struct {
	path string
	mu   sync.Mutex
	file *os.File
	held bool
}

func newInstallLock(path string) *installLock {
	return &installLock{path: path}
}

func (l *installLock) Lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	const (
		minDelay = 50 * time.Millisecond
		maxDelay = 200 * time.Millisecond
	)
	delay := minDelay
	for {
		locked, err := l.tryLock()
		if err != nil {
			return err
		}
		if locked {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay = min(delay*2, maxDelay)
	}
}
