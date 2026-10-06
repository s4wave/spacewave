package s4db

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// watcher waits for commits of other processes on a named event per reader
// slot. Windows reports writes through another handle only when that
// handle closes, so each writer signals the events of the live slots after
// it unlocks.
type watcher struct {
	// f is the database file.
	f *os.File
	// slot is this handle's reader slot.
	slot int
	// prefix starts the event names of the file's slots.
	prefix string
	// event is signaled when another process commits.
	event windows.Handle
	// halt is signaled by stop.
	halt windows.Handle
	// peers holds the opened events of other slots. Only the writer lock
	// holder uses it.
	peers map[int]windows.Handle
}

// newWatcher creates the event of slot of the file f.
func newWatcher(f *os.File, _ string, slot int) (*watcher, error) {
	// Name events by the file's identity, so every path to it agrees.
	id, err := identify(f)
	if err != nil {
		return nil, err
	}
	w := &watcher{
		f:      f,
		slot:   slot,
		prefix: fmt.Sprintf(`Local\s4db-%x-%x-`, id.volume, id.index),
		peers:  make(map[int]windows.Handle),
	}

	// Create this slot's event and the stop event. A slot's event may
	// outlive an earlier holder through other processes' handles; it is
	// auto-reset, so a stale signal costs one empty tail.
	name, err := windows.UTF16PtrFromString(w.name(slot))
	if err != nil {
		return nil, err
	}
	if w.event, err = windows.CreateEvent(nil, 0, 0, name); err != nil {
		return nil, err
	}
	if w.halt, err = windows.CreateEvent(nil, 1, 0, nil); err != nil {
		w.close()
		return nil, err
	}
	return w, nil
}

// name returns the event name of slot.
func (w *watcher) name(slot int) string {
	return fmt.Sprintf("%s%d", w.prefix, slot)
}

// wait blocks until another process commits, reporting false once stopped.
func (w *watcher) wait() bool {
	ev, err := windows.WaitForMultipleObjects([]windows.Handle{w.event, w.halt}, false, windows.INFINITE)
	return err == nil && ev == windows.WAIT_OBJECT_0
}

// notify signals the event of every other live slot. The caller holds the
// writer lock.
func (w *watcher) notify() {
	// Read the slots.
	b := make([]byte, pageSize)
	if _, err := w.f.ReadAt(b, slotPage*pageSize); err != nil {
		return
	}

	// Signal each slot holding a pin, opening its event once.
	for i := range slots {
		if i == w.slot {
			continue
		}
		if _, ok := decodePin(b[i*slotSize:]); !ok {
			continue
		}
		h, ok := w.peers[i]
		if !ok {
			name, err := windows.UTF16PtrFromString(w.name(i))
			if err != nil {
				continue
			}
			if h, err = windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name); err != nil {
				continue
			}
			w.peers[i] = h
		}
		_ = windows.SetEvent(h)
	}
}

// stop wakes wait for the last time.
func (w *watcher) stop() {
	_ = windows.SetEvent(w.halt)
}

// close releases the watcher's events.
func (w *watcher) close() {
	for _, h := range w.peers {
		_ = windows.CloseHandle(h)
	}
	for _, h := range []windows.Handle{w.event, w.halt} {
		if h != 0 {
			_ = windows.CloseHandle(h)
		}
	}
}
