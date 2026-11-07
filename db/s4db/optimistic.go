package s4db

import (
	"bytes"
)

// readSet records what an optimistic write transaction read: keys, and the
// prefixes it iterated, which cover keys inserted into them later.
type readSet struct {
	// keys holds the keys read one at a time.
	keys map[string]struct{}
	// prefixes holds the iterated prefixes.
	prefixes [][]byte
	// all is set when the transaction read the whole store, by iterating
	// every key or counting them.
	all bool
}

// newReadSet returns an empty read set.
func newReadSet() *readSet {
	return &readSet{keys: make(map[string]struct{})}
}

// addKey records a read of key.
func (r *readSet) addKey(key []byte) {
	r.keys[string(key)] = struct{}{}
}

// addPrefix records an iteration of prefix.
func (r *readSet) addPrefix(prefix []byte) {
	if len(prefix) == 0 {
		r.all = true
		return
	}
	r.prefixes = append(r.prefixes, bytes.Clone(prefix))
}

// changedSince reports whether a commit after snap through base changed a
// key reads holds, or whether base cannot tell.
//
// The memtable holds each key's newest change after the last flush with the
// commit that made it, from this handle's commits and the records the tail
// applies alike, so a key changed after snap has an entry newer than snap
// unless a flush past snap dropped it. A flush since snap therefore
// conflicts, as does any commit after snap for a transaction that read
// everything.
func changedSince(snap, base *state, reads *readSet) bool {
	// Nothing after the snapshot changes nothing.
	if base.seq == snap.seq {
		return false
	}
	if base.flushed > snap.seq || reads.all {
		return true
	}

	// Test each key read, then each scanned prefix.
	after := snap.seq
	for key := range reads.keys {
		if e, ok := base.mem.get([]byte(key)); ok && e.seq > after {
			return true
		}
	}
	for _, prefix := range reads.prefixes {
		changed := false
		base.mem.tree.Ascend(mentry{key: prefix}, func(e mentry) bool {
			if !bytes.HasPrefix(e.key, prefix) {
				return false
			}
			changed = e.seq > after
			return !changed
		})
		if changed {
			return true
		}
	}
	return false
}
