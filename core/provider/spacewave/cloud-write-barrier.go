package provider_spacewave

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/s4wave/spacewave/core/sobject"
)

// CloudWriteBarrierEnv turns off the cloud host's write barrier when set to
// "off" at startup.
//
// The barrier keeps one mount's writes of the verified state cache from
// landing after the next mount loaded it. Its invariants:
//
//   - Every write of the verified state cache by a cloudSOHost runs under its
//     acceptMu, taken through lockAccept. The cache writer's record write is a
//     plain set, so this lock is the only thing ordering a host's writes.
//   - A closed host writes nothing: lockAccept refuses it after close, so no
//     persist or publication registration starts.
//   - The tracker run calls close before it leaves its run slot, and close
//     waits for the acceptMu holder in progress. When the run leaves the slot,
//     each of the host's writes has committed or will never start.
//   - Each run of an object takes its run slot before it loads the cache and
//     waits there for the run before it to leave. The keyed routine waits only
//     when the same routine restarts: a key removed and added again starts its
//     new run at once, so the slot, not keyed, orders the runs. The next
//     mount's load therefore reads every write the old mount made.
//   - close runs before the run detaches the host from the sync controller, so
//     a closed host never registers its publication again.
//   - A failed persist in a local write may have committed. The write then
//     returns sobject.ErrStateWriteUnknown and ends the host, so the next mount
//     settles it from what the cache holds.
//   - A closed host's write returns sobject.ErrMountReloading, and the mount
//     reports itself released when its run ends, so the World remounts and
//     writes through the next run.
//   - Code that deletes an object's cache, such as a reseed, ends the object's
//     run and holds its run slot while it deletes, so no host writes the cache
//     during the delete and no run loads it half deleted.
//
// With the barrier off, a host keeps writing after its run ended, a persist
// failure is a plain error, runs and deletes do not wait on the run slot, and
// the mount never reports itself released, as before the barrier.
const CloudWriteBarrierEnv = "SPACEWAVE_CLOUD_WRITE_BARRIER"

// cloudWriteBarrierOff selects CloudWriteBarrierEnv's old behavior. It is read
// once, here, at startup.
var cloudWriteBarrierOff = os.Getenv(CloudWriteBarrierEnv) == "off"

// errHostClosed is returned for a write to a cloud host whose tracker run
// ended. The write stored nothing. It wraps sobject.ErrMountReloading, so the
// World retires the engine on the ended mount and writes through the next.
var errHostClosed = fmt.Errorf("cloud shared object host closed: %w", sobject.ErrMountReloading)

// lockAccept takes acceptMu for a write of the host's accepted state. It
// returns errHostClosed, holding nothing, once the host is closed.
func (h *cloudSOHost) lockAccept(ctx context.Context) (func(), error) {
	// Serialize with the host's other acceptance and persistence.
	release, err := h.acceptMu.Lock(ctx)
	if err != nil {
		return nil, err
	}

	// Refuse a write once the host's run has ended.
	if h.closed {
		release()
		return nil, errHostClosed
	}
	return release, nil
}

// close waits for the write that holds acceptMu, then refuses every later
// write. The tracker run calls it before it returns.
func (h *cloudSOHost) close() {
	// Keep the old behavior when the barrier is off.
	if cloudWriteBarrierOff {
		return
	}

	// Mark the host closed once its current writer finishes.
	release, _ := h.acceptMu.Lock(context.Background())
	h.closed = true
	release()
}

// persistUnknown reports a failed persist of a local write. A commit error
// can follow a stored write, so the write's outcome is unknown: the host ends,
// and the next mount settles the write from the cache. With the barrier off
// it returns err.
func (h *cloudSOHost) persistUnknown(err error) error {
	// Keep the old behavior when the barrier is off.
	if cloudWriteBarrierOff {
		return err
	}

	// End the host so the next mount reads what the persist left.
	if h.ctxCancel != nil {
		h.ctxCancel()
	}
	return errors.Join(sobject.ErrStateWriteUnknown, err)
}

// cloudRunHandoff orders the tracker runs of each shared object, so a run
// loads the verified state cache only after the run before it has closed its
// host. Its zero value is ready to use.
type cloudRunHandoff struct {
	// mtx guards last.
	mtx sync.Mutex
	// last maps a shared object ID to the done channel of its newest slot.
	last map[string]chan struct{}
}

// cloudRunSlot is one holder's turn in an object's run order. A nil slot,
// taken with the barrier off, waits for nothing.
type cloudRunSlot struct {
	// h is the hand-off that issued the slot.
	h *cloudRunHandoff
	// soID is the shared object the slot orders.
	soID string
	// prev is closed once the previous holder leaves, or nil when there is none.
	prev chan struct{}
	// done is closed once this holder and every holder before it have left.
	done chan struct{}
	// once makes release idempotent.
	once sync.Once
}

// take queues a slot for soID behind the newest one. Call release exactly
// once, after the holder's last write of the object's cache.
func (h *cloudRunHandoff) take(soID string) *cloudRunSlot {
	// Keep the old behavior when the barrier is off.
	if cloudWriteBarrierOff {
		return nil
	}

	// Queue the slot behind the newest one.
	s := &cloudRunSlot{h: h, soID: soID, done: make(chan struct{})}
	h.mtx.Lock()
	if h.last == nil {
		h.last = make(map[string]chan struct{})
	}
	s.prev = h.last[soID]
	h.last[soID] = s.done
	h.mtx.Unlock()
	return s
}

// wait returns once the previous holder has left the slot order, or with
// ctx's error first. The slot must still be released after an error.
func (s *cloudRunSlot) wait(ctx context.Context) error {
	if s == nil || s.prev == nil {
		return nil
	}
	select {
	case <-s.prev:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// release leaves the slot order. The next holder proceeds only after every
// earlier holder has left too, so a holder that stopped waiting early does not
// let a later one pass a holder still writing.
func (s *cloudRunSlot) release() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		if s.prev == nil {
			s.finish()
			return
		}
		select {
		case <-s.prev:
			s.finish()
		default:
			go func() {
				<-s.prev
				s.finish()
			}()
		}
	})
}

// finish signals the next holder and forgets the slot once it is the newest.
func (s *cloudRunSlot) finish() {
	close(s.done)
	s.h.mtx.Lock()
	if s.h.last[s.soID] == s.done {
		delete(s.h.last, s.soID)
	}
	s.h.mtx.Unlock()
}
