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
	joined readSets
	// reruns end when state outside the World that the reads depend on
	// changes, such as a value a process keeps in memory. A wait ends when one
	// of them, or one of a joined or shared set's, ends.
	reruns []context.Context
	// memoChanges and memoTouched cache Touched for the last ChangeSet it
	// checked, so views sharing this set test each change window once. Any
	// recorded read clears memoChanges.
	memoChanges *ChangeSet
	memoTouched bool
	// version counts the recorded reads, so Touched keeps its answer only when
	// no read was recorded while it checked unlocked.
	version uint64
	// added ends when a read is recorded, or is nil when no wait took it
	// since the last read. Most reads come while a view evaluates, with no wait
	// to wake, so the context is made only for a wait. A context rather than
	// a channel lets a wait end its subscription on the read through
	// context.AfterFunc, which starts no goroutine until the read comes.
	added context.Context
	// endAdded ends added.
	endAdded context.CancelFunc
}

// NewReadSet constructs an empty ReadSet.
func NewReadSet() *ReadSet {
	return &ReadSet{
		keys:      make(map[string]struct{}),
		endpoints: make(map[string]struct{}),
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
	joined, version := r.joined.list, r.version
	r.mtx.Unlock()

	// Check the joined sets unlocked, each through its own memo.
	if !touched {
		touched = slices.ContainsFunc(joined, func(set *ReadSet) bool { return set.Touched(changes) })
	}

	// Keep the answer unless a read was recorded meanwhile.
	r.mtx.Lock()
	if r.version == version {
		r.memoChanges, r.memoTouched = changes, touched
	}
	r.mtx.Unlock()
	return touched
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
// reads, and returns its seqno, or for a rerun context to end, and returns
// afterSeqno. Record afterSeqno before the reads. Reads may continue while
// WaitChange runs: each one restarts the wait from afterSeqno, so a change
// that lands before the read of what it changed still ends it.
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
		added := r.addedLocked()
		r.mtx.Unlock()

		// End at once when state outside the World the reads depend on
		// changed already.
		reruns := r.appendReruns(nil)
		for _, set := range shared {
			reruns = set.appendReruns(reruns)
		}
		if slices.ContainsFunc(reruns, ended) {
			return afterSeqno, nil
		}

		// Name the reads for the change feed's index.
		interest := r.waitInterest(touches, shared)

		// Wait in the change feed for the first touching change since
		// afterSeqno, on this goroutine: a recorded read ends the
		// subscription through its context instead of a second goroutine
		// selecting on the signal, since every waiting view would pay for one.
		watchCtx, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(added, cancel)
		stops := make([]func() bool, len(reruns))
		for i, rerun := range reruns {
			stops[i] = context.AfterFunc(rerun, cancel)
		}
		var seqno uint64
		err := WatchInterestedChanges(watchCtx, w, afterSeqno, interest, func(s uint64, _ *ChangeSet) error {
			seqno = s
			return errReadsTouched
		})
		stop()
		for _, stop := range stops {
			stop()
		}
		cancel()

		// End at the touching change or an ended rerun context, or restart
		// from afterSeqno after a new read so the feed tests the earlier
		// changes against it too.
		if errors.Is(err, errReadsTouched) {
			return seqno, nil
		}
		if ctx.Err() == nil && slices.ContainsFunc(reruns, ended) {
			return afterSeqno, nil
		}
		if added.Err() != nil && ctx.Err() == nil && errors.Is(err, context.Canceled) {
			continue
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
		return r.joined.list
	}

	// Name the read keys, the pattern endpoints, and a field of each pattern,
	// sized once: a view's set can read thousands of keys, and growing the
	// list by doubling copies it each time.
	interest.keys = slices.Grow(interest.keys, len(r.keys)+len(r.endpoints))
	interest.keys = slices.AppendSeq(interest.keys, maps.Keys(r.keys))
	interest.keys = slices.AppendSeq(interest.keys, maps.Keys(r.endpoints))
	interest.terms = slices.Grow(interest.terms, len(r.patterns))
	for _, p := range r.patterns {
		term, ok := p.term()
		if !ok {
			interest.all = true
			return r.joined.list
		}
		interest.terms = append(interest.terms, term)
	}
	return r.joined.list
}

// touchesKey checks a changed object key. The caller holds mtx.
func (r *ReadSet) touchesKey(key string) bool {
	// Match the key itself and the iterated prefixes.
	if _, ok := r.keys[key]; ok {
		return true
	}
	for _, prefix := range r.prefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}

	// Match a graph read naming the key as a subject or object.
	if r.graph {
		return true
	}
	_, ok := r.endpoints[key]
	return ok
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

// addRerun records a context whose end changes what was read, as a changed
// read would.
func (r *ReadSet) addRerun(rerun context.Context) {
	r.mtx.Lock()
	r.reruns = append(r.reruns, rerun)
	r.signalLocked()
	r.mtx.Unlock()
}

// appendReruns appends the rerun contexts of r and of the sets it joined to
// out. A set without any appends nothing and allocates nothing.
func (r *ReadSet) appendReruns(out []context.Context) []context.Context {
	// Take r's own contexts and its joined sets under the lock.
	r.mtx.Lock()
	out = append(out, r.reruns...)
	joined := r.joined.list
	r.mtx.Unlock()

	// Add each joined set's contexts outside it.
	for _, set := range joined {
		out = set.appendReruns(out)
	}
	return out
}

// ended reports whether ctx has ended.
func ended(ctx context.Context) bool { return ctx.Err() != nil }

// join records every read of other, a finished set, by reference: a change
// that touches other touches r. Copying the reads instead would cost each
// set built from a large one all of its keys. Joining r itself, or a set r
// already joined directly, does nothing.
func (r *ReadSet) join(other *ReadSet) {
	// Skip r itself and a set r already joined.
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if other == r || !r.joined.add(other) {
		return
	}

	// Wake the waits.
	r.signalLocked()
}

// readSetsScanned is the length up to which readSets finds a set by scanning
// its list, which costs less than building and probing the index.
const readSetsScanned = 32

// readSets is a list of distinct read sets that finds a set by scanning a
// short list and through an index past readSetsScanned. The list is appended
// to only, so a copy of it stays valid unlocked.
type readSets struct {
	// list holds the sets in the order they were added.
	list []*ReadSet
	// has indexes list once it passes readSetsScanned, and is nil before.
	has map[*ReadSet]struct{}
}

// add appends set unless the list holds it, and reports whether it did.
func (s *readSets) add(set *ReadSet) bool {
	// Find the set by scan or index.
	if s.has == nil {
		if slices.Contains(s.list, set) {
			return false
		}
		if len(s.list) == readSetsScanned {
			s.has = make(map[*ReadSet]struct{}, 2*readSetsScanned)
			for _, held := range s.list {
				s.has[held] = struct{}{}
			}
		}
	} else if _, ok := s.has[set]; ok {
		return false
	}

	// Append it, indexing it once the index exists.
	if s.has != nil {
		s.has[set] = struct{}{}
	}
	s.list = append(s.list, set)
	return true
}

// signalLocked wakes the waits checking the reads and drops the cached Touched
// answer. The caller holds mtx.
func (r *ReadSet) signalLocked() {
	r.memoChanges = nil
	r.version++
	if r.added != nil {
		r.endAdded()
		r.added, r.endAdded = nil, nil
	}
}

// addedLocked returns the context the next recorded read ends. The caller
// holds mtx.
func (r *ReadSet) addedLocked() context.Context {
	if r.added == nil {
		r.added, r.endAdded = context.WithCancel(context.Background())
	}
	return r.added
}
