package block

import (
	"context"
	"slices"
	"sync"
)

// SupportsRootRetention reports the destination's root ownership capability.
func (s *BufferedStore) SupportsRootRetention() bool { return SupportsRootRetention(s.inner) }

// SetRetainedRoot fences prepared blocks before publishing their durable root.
func (s *BufferedStore) SetRetainedRoot(ctx context.Context, name string, ref *BlockRef) error {
	if _, err := s.Sync(ctx); err != nil {
		return err
	}
	return SetRetainedRoot(ctx, s.inner, name, ref)
}

// PinRoot acquires a reader pin on ref. A root still pending here is not in
// the inner store, so no sweep can collect it: the pin waits in the buffer and
// becomes an inner pin when the drain writes the root. A pin released before
// then costs the inner store nothing.
func (s *BufferedStore) PinRoot(ctx context.Context, ref *BlockRef) (func(), error) {
	// Defer the pin while its root is pending.
	key, err := marshalRefKey(ref)
	if err != nil {
		return nil, err
	}
	pin := &bufferedPin{ref: ref.Clone()}
	var deferred bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if s.pending[key] != nil {
			s.pins[key] = append(s.pins[key], pin)
			deferred = true
		}
	})
	if deferred {
		return sync.OnceFunc(func() { s.unpin(key, pin) }), nil
	}

	// Pin a root the inner store already holds.
	return PinRoot(ctx, s.inner, ref)
}

// bufferedPin is a reader pin taken while its root was pending.
type bufferedPin struct {
	// ref is the pinned root.
	ref *BlockRef
	// release drops the inner pin, or is nil until the drain acquires it.
	release func()
	// released records that the holder released the pin before the drain
	// acquired its inner pin.
	released bool
}

// unpin releases pin, the inner pin when the drain acquired one.
func (s *BufferedStore) unpin(key string, pin *bufferedPin) {
	var release func()
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if pin.release != nil {
			release = pin.release
			return
		}
		pin.released = true
		pins := slices.DeleteFunc(s.pins[key], func(p *bufferedPin) bool { return p == pin })
		if len(pins) == 0 {
			delete(s.pins, key)
		} else {
			s.pins[key] = pins
		}
	})
	if release != nil {
		release()
	}
}

// takeWrittenPinsLocked removes the deferred pins of the roots batch wrote. A
// written tombstone removed its root, so its pins resolve to nothing. Caller
// holds bcast.
func (s *BufferedStore) takeWrittenPinsLocked(batch *drainBatch) []*bufferedPin {
	var written []*bufferedPin
	for _, p := range batch.pending {
		key, _ := marshalRefKey(p.ref)
		pins := s.pins[key]
		if len(pins) == 0 {
			continue
		}
		delete(s.pins, key)
		if p.tombstone {
			for _, pin := range pins {
				pin.release = func() {}
			}
			continue
		}
		written = append(written, pins...)
	}
	return written
}

// acquirePins takes the inner pins of roots the drain wrote. The caller holds
// drainMu, so a later root release waits until the pins hold.
func (s *BufferedStore) acquirePins(ctx context.Context, pins []*bufferedPin) error {
	for _, pin := range pins {
		release, err := PinRoot(ctx, s.inner, pin.ref)
		if err != nil {
			return err
		}
		var released bool
		s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			released = pin.released
			if !released {
				pin.release = release
			}
		})
		if released {
			release()
		}
	}
	return nil
}

// OpenStage opens a stage on the destination. Reads go through this buffer, so
// a staged build sees blocks still pending here; writes go to the stage.
func (s *BufferedStore) OpenStage(ctx context.Context) (StoreOps, func(), error) {
	stage, release, err := OpenStage(ctx, s.inner)
	if err != nil {
		return nil, nil, err
	}
	return NewStoreRW(s, stage), release, nil
}

// ReleaseRoots fences prepared blocks so their staging edges exist before
// they are released.
func (s *BufferedStore) ReleaseRoots(ctx context.Context, refs []*BlockRef) error {
	if _, err := s.Sync(ctx); err != nil {
		return err
	}
	return ReleaseRoots(ctx, s.inner, refs)
}

// MarkRootsComplete forwards the durable World proof.
func (s *BufferedStore) MarkRootsComplete(ctx context.Context, roots []*BlockRef) error {
	return MarkRootsComplete(ctx, s.inner, roots)
}

// RootComplete checks the underlying World proof.
func (s *BufferedStore) RootComplete(ctx context.Context, ref *BlockRef) (bool, error) {
	return RootComplete(ctx, s.inner, ref)
}

// _ is a type assertion
var _ RootRetainer = (*BufferedStore)(nil)
