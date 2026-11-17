package provider_local

import "sync"

// seenRefsCap bounds each seenRefs set. A marshaled ref is 52 bytes, so a
// full set holds about 64 Ki * (52 + 16 + ~60 map bytes), near 8 MiB.
const seenRefsCap = 1 << 16

// seenRefs remembers the last seenRefsCap refs added, keyed by their
// marshaled string, and forgets the oldest first. The once-per-block
// warnings use it, so a ref forgotten after seenRefsCap newer distinct refs
// warns once more: one more log line, and for a failed read one more stack
// walk of at most peerSiteDepth frames.
type seenRefs struct {
	mu   sync.Mutex
	keys map[string]struct{}
	ring []string
	next int
}

// newSeenRefs returns an empty set.
func newSeenRefs() *seenRefs {
	return &seenRefs{keys: make(map[string]struct{})}
}

// add remembers key and reports whether the set did not already hold it.
func (s *seenRefs) add(key string) bool {
	// Report a key the set already holds.
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.keys[key]; ok {
		return false
	}

	// Remember key, in place of the oldest key once the set is full.
	if len(s.ring) < seenRefsCap {
		s.ring = append(s.ring, key)
	} else {
		delete(s.keys, s.ring[s.next])
		s.ring[s.next] = key
		s.next = (s.next + 1) % seenRefsCap
	}
	s.keys[key] = struct{}{}
	return true
}
