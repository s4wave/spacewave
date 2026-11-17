package s4db

import "context"

// beginWait announces one foreground request to this handle's background work.
func (w *writer) beginWait() error {
	// Publish the local foreground announcement and wake yielded work.
	l := w.requests.Lock()
	w.waiting.Add(1)
	l.Broadcast()
	l.Unlock()
	return nil
}

// endWait removes an acquired or cancelled foreground request.
func (w *writer) endWait() {
	// Remove the announcement and notify local completion waiters.
	l := w.requests.Lock()
	w.waiting.Add(-1)
	w.served.Add(1)
	l.Broadcast()
	l.Unlock()
}

// waitingSlot reports local foreground demand; writable peers cannot exist.
func (w *writer) waitingSlot() (int, error) {
	if w.waiting.Load() != 0 {
		return w.db.slot, nil
	}
	return -1, nil
}

// waitRequest waits outside writer for a local acquisition or cancellation.
func (w *writer) waitRequest(ctx context.Context, _ int, served uint64) error {
	return w.requests.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return w.served.Load() != served, nil
	})
}
