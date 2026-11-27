package world_block

import "sync"

// closedCh is a channel that is always closed.
var closedCh = func() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}()

// revWatch wakes the waiters of one object key when a change references it.
//
// A waiter takes its key's channel with wait before it reads the object, then
// waits on the channel, so a change between the read and the wait still wakes
// it. A writer wakes while it holds the transaction's write lock, which the
// waiter's read takes, so a woken waiter reads the finished change. A change
// to key K closes only K's channel. A create wakes the waiters for a missing
// K the same way. Rebuilding the state wakes every waiter, and closing it
// wakes every waiter and every later one.
//
// The map holds one channel per key a waiter is waiting on. A channel whose
// waiters gave up stays until its key changes or the state rebuilds or closes,
// so the map is bounded by the keys waited on during the state's life.
type revWatch struct {
	mtx    sync.Mutex
	keys   map[string]chan struct{}
	closed bool
}

// wait returns the channel the next change to key closes.
func (w *revWatch) wait(key string) <-chan struct{} {
	// A closed watch never wakes again, so its waiters return at once.
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if w.closed {
		return closedCh
	}

	// Share the key's open channel, or open one for its first waiter.
	ch, ok := w.keys[key]
	if !ok {
		if w.keys == nil {
			w.keys = make(map[string]chan struct{})
		}
		ch = make(chan struct{})
		w.keys[key] = ch
	}
	return ch
}

// wake wakes the waiters of each key. An empty key, as a graph change has,
// wakes no one.
func (w *revWatch) wake(keys ...string) {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	for _, key := range keys {
		if ch, ok := w.keys[key]; ok {
			close(ch)
			delete(w.keys, key)
		}
	}
}

// wakeAll wakes every waiter, as when the state moves to another root.
func (w *revWatch) wakeAll() {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	w.wakeAllLocked()
}

// close wakes every waiter, and makes every later wait return at once.
func (w *revWatch) close() {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	w.closed = true
	w.wakeAllLocked()
}

// wakeAllLocked closes every key's channel. Expects mtx to be held.
func (w *revWatch) wakeAllLocked() {
	for _, ch := range w.keys {
		close(ch)
	}
	w.keys = nil
}
