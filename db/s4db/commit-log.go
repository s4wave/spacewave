package s4db

import (
	"bytes"
	"cmp"
	"slices"
	"sync"
	"unsafe"
)

// commitLogBytes bounds the memory of the commit log. At hundreds of commits
// a second of tens of keys each it reaches back tens of seconds, far longer
// than an optimistic transaction runs; an older snapshot conflicts.
const commitLogBytes = 8 << 20

// keyHeader is the memory a logged key holds besides its bytes, which it
// shares with the commit's overlay entry.
const keyHeader = int64(unsafe.Sizeof([]byte(nil)))

// commitLog holds the keys the recent commits of this handle's states
// changed, so an optimistic write transaction can tell at commit whether a
// commit after its snapshot changed a key it read. It covers every commit
// after from through last. A tail that adopts another process's checkpoint
// past commits this handle never applied restarts it at the checkpoint, and
// trimming raises from past the dropped commits.
type commitLog struct {
	// mu guards the fields below. Tails add from any goroutine.
	mu sync.Mutex
	// from is the commit the log covers after.
	from uint64
	// last is the last record the log covers.
	last uint64
	// entries holds the changed keys of each commit after from, in order.
	entries []logEntry
	// bytes estimates the memory of entries.
	bytes int64
}

// logEntry is the changed keys of one commit.
type logEntry struct {
	// seq is the commit.
	seq uint64
	// keys are the keys it set or deleted.
	keys [][]byte
	// size estimates the entry's memory.
	size int64
}

// newCommitLog returns a log that covers the commits after seq.
func newCommitLog(seq uint64) *commitLog {
	return &commitLog{from: seq, last: seq}
}

// add logs the changed keys of the commit records among rs, which follow
// last in order. A record the log already covers is skipped, so tails that
// apply the same records add them once.
func (l *commitLog) add(rs ...*record) {
	// Hold the log while it grows.
	l.mu.Lock()
	defer l.mu.Unlock()

	// Log each new commit's keys.
	for _, r := range rs {
		if r.seq <= l.last {
			continue
		}
		l.last = r.seq
		if r.kind != kindCommit || len(r.ops) == 0 {
			continue
		}
		e := logEntry{seq: r.seq, keys: make([][]byte, len(r.ops))}
		for i, o := range r.ops {
			e.keys[i] = o.key
			e.size += keyHeader + int64(len(o.key))
		}
		l.entries = append(l.entries, e)
		l.bytes += e.size
	}

	// Drop the oldest commits past the bound.
	n := 0
	for l.bytes > commitLogBytes && n < len(l.entries) {
		l.bytes -= l.entries[n].size
		l.from = l.entries[n].seq
		n++
	}
	if n != 0 {
		clear(l.entries[:n])
		l.entries = l.entries[n:]
	}
}

// jump restarts the log after seq, the commit of a checkpoint another
// process wrote, when the log does not reach it: the commits before it were
// never applied here.
func (l *commitLog) jump(seq uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if seq > l.last {
		l.from, l.last = seq, seq
		clear(l.entries)
		l.entries, l.bytes = l.entries[:0], 0
	}
}

// changed reports whether a commit after snap through base changed a key
// reads holds, or whether the log cannot tell because it does not cover
// those commits.
func (l *commitLog) changed(snap, base uint64, reads *readSet) bool {
	// Hold the log while reading it.
	l.mu.Lock()
	defer l.mu.Unlock()

	// A snapshot before the log, or a base past it, cannot be validated.
	if snap < l.from || base > l.last {
		return true
	}

	// Test the keys of each commit after the snapshot.
	i, _ := slices.BinarySearchFunc(l.entries, snap+1, func(e logEntry, seq uint64) int {
		return cmp.Compare(e.seq, seq)
	})
	for _, e := range l.entries[i:] {
		if e.seq > base {
			break
		}
		if reads.all {
			return true
		}
		if slices.ContainsFunc(e.keys, reads.has) {
			return true
		}
	}
	return false
}

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

// has reports whether the transaction read key.
func (r *readSet) has(key []byte) bool {
	if _, ok := r.keys[string(key)]; ok {
		return true
	}
	for _, prefix := range r.prefixes {
		if bytes.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
