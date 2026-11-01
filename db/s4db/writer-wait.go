package s4db

import "context"

// beginWait publishes this handle's foreground intent before it queues for the
// writer lock. Reader-slot ownership gives each process a distinct intent byte;
// the OS drops that byte on process death, so a dead waiter never blocks reuse.
func (w *writer) beginWait() error {
	// Hold the byte until the last local request has acquired or cancelled.
	l := w.requests.Lock()
	defer l.Unlock()
	if w.waiting.Load() == 0 {
		if _, err := w.db.s.lock(lockForeground+int64(w.db.slot), true); err != nil {
			return err
		}
	}
	w.waiting.Add(1)
	l.Broadcast()
	return nil
}

// endWait clears one foreground request after acquiring or cancelling its wait.
func (w *writer) endWait() {
	// Wake local background work after dropping the last outstanding intent.
	l := w.requests.Lock()
	if w.waiting.Add(-1) == 0 {
		_ = w.db.s.unlock(lockForeground + int64(w.db.slot))
	}
	l.Broadcast()
	l.Unlock()
}

// waitingSlot returns a process with foreground intent, or -1. The caller holds
// the writer lock: a background step must never append past an observed intent.
// The fixed slot table bounds this check independently of the database's size.
func (w *writer) waitingSlot() (int, error) {
	// Own locks are reentrant on Unix, so use the local count for this slot.
	if w.waiting.Load() != 0 {
		return w.db.slot, nil
	}

	// Each remote process holds its intent until it acquires the writer lock.
	for i := range slots {
		if i == w.db.slot {
			continue
		}
		held, err := w.db.s.held(lockForeground + int64(i))
		if err != nil {
			return -1, err
		}
		if held {
			return i, nil
		}
	}
	return -1, nil
}

// waitRequest waits for a foreground intent to clear without holding either
// writer lock. Remote waits use the OS lock wakeup, including process death;
// like the writer's existing file-lock wait, they cannot be context-cancelled.
func (w *writer) waitRequest(ctx context.Context, slot int) error {
	// Local requests broadcast both acquisition and cancellation.
	if slot == w.db.slot {
		return w.requests.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
			return w.waiting.Load() == 0, nil
		})
	}

	// A remote intent is a dedicated byte; release it as soon as it is granted.
	off := lockForeground + int64(slot)
	if _, err := w.db.s.lock(off, true); err != nil {
		return err
	}
	return w.db.s.unlock(off)
}
