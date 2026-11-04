package s4db

import (
	"context"
	"slices"
	"sync"
)

// writerLock excludes a handle's writers. It grants the lock in arrival order
// and wakes only the next holder, so a release costs the same however many
// writers wait. Background checkpoint steps go before queued writers: they
// hold the lock briefly, and a checkpoint that waits behind each queued
// transaction falls behind the commits, while the overlay and the file grow
// until it ends. The zero value is unlocked.
type writerLock struct {
	// mtx guards the fields below and is never held across a wait.
	mtx sync.Mutex
	// held is set while a holder owns the lock. A release to a waiter leaves
	// it set.
	held bool
	// first and rest are the waiters in arrival order: checkpoint steps,
	// then writers. Closing a waiter's channel grants it the lock.
	first, rest []chan struct{}
}

// lock waits for the lock, ahead of the writers when first is set. It returns
// ctx's error when ctx ends before the lock is granted.
func (l *writerLock) lock(ctx context.Context, first bool) error {
	// Take a free lock, or join the queue.
	l.mtx.Lock()
	if !l.held {
		l.held = true
		l.mtx.Unlock()
		return nil
	}
	queue := &l.rest
	if first {
		queue = &l.first
	}
	granted := make(chan struct{})
	*queue = append(*queue, granted)
	l.mtx.Unlock()

	// Wait for ownership or cancellation before inspecting the queue again.
	select {
	case <-granted:
		return nil
	case <-ctx.Done():
	}

	// Leave the queue, or pass on a grant that arrived as ctx ended.
	l.mtx.Lock()
	i := slices.Index(*queue, granted)
	if i >= 0 {
		*queue = slices.Delete(*queue, i, i+1)
	}
	l.mtx.Unlock()
	if i < 0 {
		l.unlock()
	}
	return ctx.Err()
}

// tryLock takes the lock when it is free.
func (l *writerLock) tryLock() bool {
	// Claim ownership only when neither a holder nor a granted waiter owns it.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if l.held {
		return false
	}
	l.held = true
	return true
}

// unlock grants the lock to the next waiter, or frees it.
func (l *writerLock) unlock() {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	for _, queue := range []*[]chan struct{}{&l.first, &l.rest} {
		if len(*queue) != 0 {
			close((*queue)[0])
			(*queue)[0] = nil
			*queue = (*queue)[1:]
			if len(*queue) == 0 {
				*queue = nil
			}
			return
		}
	}
	l.held = false
}
