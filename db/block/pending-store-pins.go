package block

import (
	"context"
	"errors"
	"time"
)

// pinRetryMin and pinRetryMax bound the waits of RunPins between passes that
// left a recorded root unpinned.
const (
	pinRetryMin = 250 * time.Millisecond
	pinRetryMax = 30 * time.Second
)

// pinUnkept pins root in the inner store when no tier holds it, copying it
// in, and returns the release, or nil when it took no pin. The local tier
// answers first, and its error is returned. A root no tier holds yet, such as
// a block the swap itself puts, is left to the swap. The whole-store lookup
// never waits for a peer or past childProbeWait, since a caller may hold a
// root set's lock behind it; while ctx lives its error is unknown, not a
// failure, and pinUnkept reports it as unknown.
func (s *PendingStore) pinUnkept(ctx context.Context, root *BlockRef) (unpin func(), unknown bool, err error) {
	// Look the root up in the local tier, then in the whole store.
	local := s.StoreOps
	if tiers, ok := s.StoreOps.(LocalStoreGetter); ok && tiers.GetLocalStore() != nil {
		local = tiers.GetLocalStore()
	}
	found, err := local.GetBlockExists(ctx, root)
	if err == nil && !found && local != s.StoreOps {
		probe, cancel := childProbe(ctx)
		found, err = s.StoreOps.GetBlockExists(probe, root)
		cancel()
		if err != nil && ctx.Err() == nil {
			return nil, true, nil
		}
	}
	if err != nil || found {
		return nil, false, err
	}

	// Pin it.
	unpin, err = PinRoot(ctx, s.StoreOps, root)
	if errors.Is(err, ErrNotFound) {
		return nil, false, nil
	}
	return unpin, false, err
}

// recordUnfetched records root for RunPins. Taking the next seq makes a
// DropUnfetched whose mark was taken before keep it.
func (s *PendingStore) recordUnfetched(root *BlockRef) {
	// Record it under the next seq.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.seq++
	s.unfetched[pendingKey(root)] = unfetchedRoot{ref: root.Clone(), at: s.seq}
}

// wakePins wakes RunPins without waiting.
func (s *PendingStore) wakePins() {
	select {
	case s.pinWake <- struct{}{}:
	default:
	}
}

// Unfetched returns how many roots wait for RunPins to pin them.
func (s *PendingStore) Unfetched() int {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return len(s.unfetched)
}

// DropUnfetched drops the recorded roots recorded at or before mark, a
// SweepMark the caller took before it gathered named, that named does not
// list, with no lookup, so the set holds only roots its writer still names
// and those recorded since. A dropped root is never pinned.
func (s *PendingStore) DropUnfetched(named []*BlockRef, mark uint64) {
	// Index the named roots, then drop the others recorded by mark.
	keep := make(map[string]struct{}, len(named))
	for _, ref := range named {
		keep[pendingKey(ref)] = struct{}{}
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for key, r := range s.unfetched {
		if _, ok := keep[key]; !ok && r.at <= mark {
			delete(s.unfetched, key)
		}
	}
}

// RunPins pins the roots Flush recorded until ctx ends, so a root whose tier
// was away when its Flush ran is copied into the inner store once the tier
// answers. One pass runs at a time, after each Flush that recorded a root is
// released, and again with backoff from pinRetryMin to pinRetryMax while a
// pass leaves a root unpinned. It runs off every caller's path: a Flush only
// records and wakes it. Each pin is released once taken, since the swap's
// root holds the root once the inner store holds it.
func (s *PendingStore) RunPins(ctx context.Context) error {
	// Wait for a wake or the retry, then run a pass and plan the next.
	var retry <-chan time.Time
	var delay time.Duration
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.pinWake:
		case <-retry:
		}
		if s.pinPass(ctx) {
			retry, delay = nil, 0
			continue
		}
		delay = min(max(2*delay, pinRetryMin), pinRetryMax)
		retry = time.After(delay)
	}
}

// pinPass tries each recorded root once, outside the lock, and reports
// whether none is left. A root it pins, or finds a tier holds, leaves the
// set; one whose lookup cannot answer or fails stays.
func (s *PendingStore) pinPass(ctx context.Context) bool {
	// Snapshot the set.
	s.mtx.RLock()
	roots := make([]unfetchedRoot, 0, len(s.unfetched))
	for _, r := range s.unfetched {
		roots = append(roots, r)
	}
	s.mtx.RUnlock()

	// Try each, dropping those settled unless recorded again since.
	for _, r := range roots {
		unpin, unknown, err := s.pinUnkept(ctx, r.ref)
		if unknown || err != nil {
			continue
		}
		if unpin != nil {
			unpin()
		}
		key := pendingKey(r.ref)
		s.mtx.Lock()
		if s.unfetched[key].at == r.at {
			delete(s.unfetched, key)
		}
		s.mtx.Unlock()
	}
	return s.Unfetched() == 0
}
