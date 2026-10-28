package world

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"
)

// errReadsTouched ends a wait's change subscription at the first change that
// touches its reads.
var errReadsTouched = errors.New("reads touched")

// ReadSet records what a view read from the World, so a ChangeSet can be
// checked against it. Reads it cannot describe precisely mark it conservative,
// and any change touches it. A ReadSet can track several WorldStates at once,
// and join finished sets by reference, whose reads become its own.
type ReadSet struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// keys are the object keys read, including keys that were not found.
	keys map[string]struct{}
	// prefixes are the object key prefixes iterated.
	prefixes []string
	// patterns are the graph quad lookup filters.
	patterns []graphPattern
	// endpoints are the object keys that patterns name as subject or object.
	endpoints map[string]struct{}
	// graph is set when the graph was read beyond quad lookups.
	graph bool
	// all is set when the World was read beyond objects and the graph.
	all bool
	// joined are the finished sets whose reads this set holds by reference.
	// Appended to only, so a copy of the slice stays valid unlocked.
	joined []*ReadSet
	// memoChanges and memoTouched cache Touched for the last ChangeSet it
	// checked, so views sharing this set test each change window once. Any
	// recorded read clears memoChanges.
	memoChanges *ChangeSet
	memoTouched bool
	// added closes when a read is recorded and is then replaced.
	added chan struct{}
}

// NewReadSet constructs an empty ReadSet.
func NewReadSet() *ReadSet {
	return &ReadSet{
		keys:      make(map[string]struct{}),
		endpoints: make(map[string]struct{}),
		added:     make(chan struct{}),
	}
}

// Track returns a WorldState that records the reads made through it.
// The seqno of ws identifies the state the reads observed.
func (r *ReadSet) Track(ws WorldState) WorldState {
	return &readSetWorldState{WorldState: ws, reads: r}
}

// Touched checks whether a ChangeSet may change what was read, by this set
// or a set it joined.
func (r *ReadSet) Touched(changes *ChangeSet) bool {
	// A ChangeSet is immutable once delivered, so its answer holds until a read
	// is recorded.
	r.mtx.Lock()
	if changes == r.memoChanges {
		touched := r.memoTouched
		r.mtx.Unlock()
		return touched
	}
	touched := r.touchedLocked(changes)
	joined, added := r.joined, r.added
	r.mtx.Unlock()

	// Check the joined sets unlocked, each through its own memo.
	if !touched {
		touched = slices.ContainsFunc(joined, func(set *ReadSet) bool { return set.Touched(changes) })
	}

	// Keep the answer unless a read was recorded meanwhile.
	r.mtx.Lock()
	if r.added == added {
		r.memoChanges, r.memoTouched = changes, touched
	}
	r.mtx.Unlock()
	return touched
}

// TouchedStrict checks whether a ChangeSet may change what a write read, by
// this set or a set it joined. It is Touched, except that a changed quad also
// touches each object it names: setting or deleting a quad revises those
// objects, and a write that read an object's revision depends on it, though a
// view of the object's body does not.
func (r *ReadSet) TouchedStrict(changes *ChangeSet) bool {
	// Apply the view rule first.
	if r.Touched(changes) {
		return true
	}

	// Match the objects the changed quads revised against the object reads.
	r.mtx.Lock()
	revised := slices.ContainsFunc(changes.Quads, r.revisesLocked)
	joined := r.joined
	r.mtx.Unlock()
	return revised || slices.ContainsFunc(joined, func(set *ReadSet) bool { return set.TouchedStrict(changes) })
}

// touchedLocked checks changes against the reads. The caller holds mtx.
func (r *ReadSet) touchedLocked(changes *ChangeSet) bool {
	// Unknown and conservative reads match every non-empty change.
	if changes.Unknown {
		return true
	}
	if len(changes.Keys) == 0 && len(changes.Quads) == 0 {
		return false
	}
	if r.all {
		return true
	}

	// A changed key matches a read key, a read prefix, or a pattern endpoint.
	if slices.ContainsFunc(changes.Keys, r.touchesKey) {
		return true
	}

	// A changed quad matches a graph read.
	for _, q := range changes.Quads {
		if r.graph {
			return true
		}
		for _, p := range r.patterns {
			if p.matches(q) {
				return true
			}
		}
	}
	return false
}

// WaitChange waits for the first change of w after afterSeqno that touches the
// reads, and returns its seqno. Record afterSeqno before the reads. Reads may
// continue while WaitChange runs: each one restarts the wait from afterSeqno,
// so a change that lands before the read of what it changed still ends it.
func (r *ReadSet) WaitChange(ctx context.Context, w WorldWaitSeqno, afterSeqno uint64) (uint64, error) {
	return r.waitChange(ctx, w, afterSeqno, nil)
}

// waitChange is WaitChange that also ends when a change touches any of the
// shared finished sets. The change feed tests each change against the sets,
// so a change that touches none of them never wakes the wait, and a shared
// set answers every view waiting on it from its memo.
func (r *ReadSet) waitChange(ctx context.Context, w WorldWaitSeqno, afterSeqno uint64, shared []*ReadSet) (uint64, error) {
	// touches checks one ChangeSet against the reads and the shared sets.
	touches := func(cs *ChangeSet) bool {
		return r.Touched(cs) || slices.ContainsFunc(shared, func(set *ReadSet) bool { return set.Touched(cs) })
	}
	for {
		// Capture the read signal before naming the reads, so a read recorded
		// after they are named restarts the subscription.
		r.mtx.Lock()
		added := r.added
		r.mtx.Unlock()

		// Name the reads for the change feed's index.
		interest := r.waitInterest(touches, shared)

		// Wait in the change feed for the first touching change since
		// afterSeqno.
		watchCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		var seqno uint64
		go func() {
			done <- WatchInterestedChanges(watchCtx, w, afterSeqno, interest, func(s uint64, _ *ChangeSet) error {
				seqno = s
				return errReadsTouched
			})
		}()

		// End at the touching change, or restart from afterSeqno after a new
		// read so the feed tests the earlier changes against it too.
		var err error
		select {
		case err = <-done:
		case <-added:
			cancel()
			if err = <-done; errors.Is(err, context.Canceled) && ctx.Err() == nil {
				continue
			}
		}
		cancel()
		if errors.Is(err, errReadsTouched) {
			return seqno, nil
		}
		return 0, err
	}
}

// waitInterest names the reads for a change feed's index, and passes the
// joined and shared sets by reference, for the feed to name each once.
func (r *ReadSet) waitInterest(touches ChangeFilter, shared []*ReadSet) *ChangeInterest {
	interest := &ChangeInterest{filter: touches}
	interest.shared = slices.Concat(r.nameReads(interest), shared)
	return interest
}

// nameReads adds the object keys and quad fields the set's own reads may be
// touched through to interest, or marks it as needing every change when a read
// has no such name: an iterated prefix, a graph read beyond quad lookups, a
// read beyond objects and the graph, or a quad lookup naming no field. It
// returns the sets r joined, whose reads it does not name; the caller must not
// append to the returned slice.
func (r *ReadSet) nameReads(interest *ChangeInterest) []*ReadSet {
	// Hold the reads while naming them.
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if r.all || r.graph || len(r.prefixes) != 0 {
		interest.all = true
		return r.joined
	}

	// Name the read keys, the pattern endpoints, and a field of each pattern.
	interest.keys = slices.AppendSeq(interest.keys, maps.Keys(r.keys))
	interest.keys = slices.AppendSeq(interest.keys, maps.Keys(r.endpoints))
	for _, p := range r.patterns {
		term, ok := p.term()
		if !ok {
			interest.all = true
			return r.joined
		}
		interest.terms = append(interest.terms, term)
	}
	return r.joined
}

// touchesKey checks a changed object key. The caller holds mtx.
func (r *ReadSet) touchesKey(key string) bool {
	// Match the key itself and the iterated prefixes.
	if r.readsObjectLocked(key) {
		return true
	}

	// Match a graph read naming the key as a subject or object.
	if r.graph {
		return true
	}
	_, ok := r.endpoints[key]
	return ok
}

// revisesLocked checks whether a changed quad revised an object that was read
// by key or under an iterated prefix. The caller holds mtx.
func (r *ReadSet) revisesLocked(q GraphQuad) bool {
	for _, value := range [2]string{q.GetSubject(), q.GetObj()} {
		if key, err := GraphValueToKey(value); err == nil && key != "" && r.readsObjectLocked(key) {
			return true
		}
	}
	return false
}

// readsObjectLocked checks whether an object was read by its key or under an
// iterated prefix. The caller holds mtx.
func (r *ReadSet) readsObjectLocked(key string) bool {
	if _, ok := r.keys[key]; ok {
		return true
	}
	return slices.ContainsFunc(r.prefixes, func(prefix string) bool { return strings.HasPrefix(key, prefix) })
}

// addKeys records object key reads.
func (r *ReadSet) addKeys(keys ...string) {
	r.mtx.Lock()
	for _, key := range keys {
		r.keys[key] = struct{}{}
	}
	r.signalLocked()
	r.mtx.Unlock()
}

// addPrefix records an object key prefix read.
func (r *ReadSet) addPrefix(prefix string) {
	r.mtx.Lock()
	r.prefixes = append(r.prefixes, prefix)
	r.signalLocked()
	r.mtx.Unlock()
}

// addPatterns records graph quad lookups.
func (r *ReadSet) addPatterns(filters ...GraphQuad) {
	r.mtx.Lock()
	for _, q := range filters {
		// Keep the filter for changed quads.
		p := newGraphPattern(q)
		r.patterns = append(r.patterns, p)

		// Index the object keys it names for changed objects.
		for _, value := range []string{p.subject, p.obj} {
			if key, err := GraphValueToKey(value); err == nil && key != "" {
				r.endpoints[key] = struct{}{}
			}
		}
	}
	r.signalLocked()
	r.mtx.Unlock()
}

// markGraph records a graph read that quad patterns cannot describe.
func (r *ReadSet) markGraph() {
	r.mtx.Lock()
	r.graph = true
	r.signalLocked()
	r.mtx.Unlock()
}

// markAll records a read that object keys and graph reads cannot describe.
func (r *ReadSet) markAll() {
	r.mtx.Lock()
	r.all = true
	r.signalLocked()
	r.mtx.Unlock()
}

// join records every read of other, a finished set, by reference: a change
// that touches other touches r. Copying the reads instead would cost each
// set built from a large one all of its keys. Joining r itself, or a set r
// already joined directly, does nothing.
func (r *ReadSet) join(other *ReadSet) {
	// Skip r itself and a set r already joined.
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if other == r || slices.Contains(r.joined, other) {
		return
	}

	// Join the set and wake the waits.
	r.joined = append(r.joined, other)
	r.signalLocked()
}

// signalLocked wakes the waits checking the reads and drops the cached Touched
// answer. The caller holds mtx.
func (r *ReadSet) signalLocked() {
	r.memoChanges = nil
	close(r.added)
	r.added = make(chan struct{})
}
