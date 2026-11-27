package viewcache

import (
	"context"
	"errors"
	"maps"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// KeySide selects the end of an edge a KeyIndex keys by.
type KeySide uint8

const (
	// KeySubject keys an edge by its subject.
	KeySubject KeySide = iota
	// KeyObject keys an edge by its object.
	KeyObject
)

// keyIndexWait bounds how long Absent waits for a published index behind
// the reader's revision to reach it. A patch moves the index within a cache
// catch-up of the World, milliseconds, while the walk it saves costs a cold
// read of the World's graph, seconds after a start. Absent never waits for
// the first build, which takes seconds: the reader walks meanwhile.
const keyIndexWait = 50 * time.Millisecond

// keyIndexRetryFirst and keyIndexRetryMax bound the wait before a key index
// pass that ran out of time runs again: it starts at the first and doubles up
// to the second.
const (
	keyIndexRetryFirst = 5 * time.Second
	keyIndexRetryMax   = time.Minute
)

// KeyIndex holds the object keys that have at least one edge of one predicate
// on one side, such as every account with a current lobby membership, so a
// view can answer that a key has no such edge from memory instead of walking
// the World's graph. It is built once from one lookup of the predicate, then
// patched from the changes each cache catch-up passes, on its own goroutine:
// no reader and no catch-up waits on a build or a patch.
//
// Each published set is immutable and labeled with a revision at: every key's
// membership holds at some revision at or after at. A reader at revision S
// acts on a set only when at >= S, and records the lookup it skipped in its
// view's reads, so a change after S that gives the key an edge reruns the
// view. Readers take no lock.
type KeyIndex struct {
	// c is the cache whose catch-ups patch the index.
	c *Cache
	// filter matches every edge of the predicate.
	filter world.GraphQuad
	// side is the end of an edge the index keys by.
	side KeySide
	// set is the latest published set, nil until the first build ends.
	set atomic.Pointer[keySet]
	// first ends when the first set is published, for a caller that waits
	// for the first build. Absent does not wait for the first build.
	first chan struct{}
	// running is set while the index's one worker builds or patches it.
	running atomic.Bool
	// pending is set when a catch-up passed while the worker ran, so it runs
	// once more.
	pending atomic.Bool
}

// keySet is one published set of a KeyIndex. It is read-only.
type keySet struct {
	// at labels the revision every key's membership holds at or after.
	at uint64
	// keys holds the keys with an edge of the predicate on the index's side.
	keys map[string]struct{}
	// next ends when a newer set replaces this one.
	next chan struct{}
}

// KeyIndex returns the cache's index of the keys with an edge of predicate on
// side, and starts building it on the first call. Every call with the same
// predicate and side shares one index. A nil cache returns a nil index, whose
// Absent always reports false.
func (c *Cache) KeyIndex(predicate string, side KeySide) *KeyIndex {
	// A nil cache keeps no index.
	if c == nil {
		return nil
	}

	// Share one index per predicate and side.
	id := keyIndexID{predicate: predicate, side: side}
	c.mtx.Lock()
	index := c.keys[id]
	if index == nil {
		filter := world.NewGraphQuad("", predicate, "", "") //nolint:quadscan // the build lists every edge of the predicate, once per process.
		index = &KeyIndex{c: c, filter: filter, side: side, first: make(chan struct{})}
		if c.keys == nil {
			c.keys = make(map[keyIndexID]*KeyIndex)
		}
		c.keys[id] = index
	}
	c.mtx.Unlock()

	// Start the first build.
	index.kick()
	return index
}

// keyIndexID identifies one KeyIndex of a cache.
type keyIndexID struct {
	predicate string
	side      KeySide
}

// Absent reports whether key has no edge of the index's predicate on its side
// at a revision at or past seqno, the revision the reader's snapshot reads.
// When it reports true it records the lookup the reader skipped in the view
// evaluation ctx carries, so the view reruns when a change gives the key an
// edge. When it reports false the reader reads the World as it would without
// the index: the key has an edge, the first build has not ended, or the index
// stayed behind seqno for keyIndexWait.
func (k *KeyIndex) Absent(ctx context.Context, seqno uint64, key string) bool {
	// Find a set that reaches the reader's revision.
	if k == nil || key == "" {
		return false
	}
	set, ok := k.reach(ctx, seqno)
	if !ok {
		return false
	}

	// Answer for a key it lacks, recording the lookup the reader skips.
	if _, has := set.keys[key]; has {
		return false
	}
	world.AddViewPatterns(ctx, k.edges(key))
	return true
}

// reach returns the published set once it stands at or past seqno, waiting at
// most keyIndexWait for a patch to reach it. Before the first build ends it
// returns at once.
func (k *KeyIndex) reach(ctx context.Context, seqno uint64) (*keySet, bool) {
	// Answer from the published set when it already reaches seqno, and
	// leave a reader before the first build to walk.
	set := k.set.Load()
	if set == nil {
		return nil, false
	}
	if set.at >= seqno {
		return set, true
	}

	// Ask the cache to reach seqno, then wait for each newer set.
	k.c.catchUp(seqno)
	timer := time.NewTimer(keyIndexWait)
	defer timer.Stop()
	for set.at < seqno {
		select {
		case <-set.next:
		case <-timer.C:
			return nil, false
		case <-ctx.Done():
			return nil, false
		}
		set = k.set.Load()
	}
	return set, true
}

// Size returns the keys in the published set and the revision it is labeled
// with, both zero before the first build ends.
func (k *KeyIndex) Size() (keys int, at uint64) {
	if set := k.set.Load(); set != nil {
		return len(set.keys), set.at
	}
	return 0, 0
}

// edges returns the lookup of key's edges of the index's predicate.
func (k *KeyIndex) edges(key string) world.GraphQuad {
	if k.side == KeyObject {
		return world.NewGraphQuadWithKeys("", k.filter.GetPredicate(), key, "")
	}
	return world.NewGraphQuadWithKeys(key, k.filter.GetPredicate(), "", "")
}

// edgeKey returns the key an edge holds on the index's side.
func (k *KeyIndex) edgeKey(q world.GraphQuad) (string, error) {
	if k.side == KeyObject {
		return iriKey(q.GetObj())
	}
	return iriKey(q.GetSubject())
}

// kick runs the worker, or has the running one pass once more.
func (k *KeyIndex) kick() {
	k.pending.Store(true)
	if k.running.CompareAndSwap(false, true) {
		go k.run()
	}
}

// run builds or patches the index until no catch-up is pending. Its reads
// are background reads, which a throttled store serves last. A pass that ran
// out of time, as one waiting on a throttled store does, runs again after a
// backoff that doubles from keyIndexRetryFirst to keyIndexRetryMax. Any other
// failure ends the run without retrying, so a lasting error, such as a
// closing engine, never loops hot. The next catch-up starts it again.
func (k *KeyIndex) run() {
	wait := keyIndexRetryFirst
	for {
		for k.pending.Swap(false) {
			ctx, cancel := context.WithTimeout(block.WithBackground(context.Background()), backgroundReadTimeout)
			err := k.pass(ctx)
			cancel()
			if err == nil {
				wait = keyIndexRetryFirst
				continue
			}
			le := logrus.WithError(err).WithField("predicate", k.filter.GetPredicate())
			if !errors.Is(err, context.DeadlineExceeded) {
				le.Warn("key index pass failed")
				k.running.Store(false)
				return
			}

			// Wait out the slow store, then pass again.
			le.WithField("wait", wait.String()).Debug("key index pass ran out of time, retrying")
			k.pending.Store(true)
			// The wait backs off a slow store. It polls no state.
			//gostyle:timer deadline
			time.Sleep(wait)
			wait = min(2*wait, keyIndexRetryMax)
		}
		k.running.Store(false)

		// A kick between the last check and the store above found the
		// worker still marked and started none. Take up the pending
		// kick here.
		if !k.pending.Load() || !k.running.CompareAndSwap(false, true) {
			return
		}
	}
}

// pass patches the published set from the changes the cache passed since its
// label, or builds it from one lookup of the predicate when there is no set or
// the cache's history no longer records those changes.
func (k *KeyIndex) pass(ctx context.Context) error {
	// Build without a set or with changes the history no longer records.
	set := k.set.Load()
	if set == nil {
		return k.build(ctx)
	}
	changes, through, ok := k.c.changesSince(set.at)
	if !ok {
		return k.build(ctx)
	}
	if through <= set.at {
		return nil
	}

	// Collect the keys the changes may have given or taken an edge: a changed
	// edge of the predicate, or a changed object the set holds, since a
	// deleted object loses its edges.
	check := make(map[string]struct{})
	for _, q := range changes.Quads {
		if !world.MatchGraphQuad(k.filter, q) {
			continue
		}
		key, err := k.edgeKey(q)
		if err != nil {
			continue
		}
		check[key] = struct{}{}
	}
	for _, key := range changes.Keys {
		if _, ok := set.keys[key]; ok {
			check[key] = struct{}{}
		}
	}
	if len(check) == 0 {
		k.publish(set.keys, through)
		return nil
	}

	// Check each one in a snapshot at or past through, and publish a copy.
	tx, err := k.c.engine.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	keys := maps.Clone(set.keys)
	for key := range check {
		found, err := tx.LookupGraphQuads(ctx, k.edges(key), 1)
		if err != nil {
			return err
		}
		if len(found) != 0 {
			keys[key] = struct{}{}
		} else {
			delete(keys, key)
		}
	}
	k.publish(keys, through)
	return nil
}

// build lists every edge of the predicate in one snapshot and publishes its
// keys at the snapshot's revision. The cache keeps the changes after it, so
// the next pass patches from there.
func (k *KeyIndex) build(ctx context.Context) error {
	// Open one snapshot and note its revision.
	start := time.Now()
	tx, err := k.c.engine.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	seqno, err := tx.GetSeqno(ctx)
	if err != nil {
		return err
	}

	// List the keys every edge of the predicate holds on the index's side.
	edges, err := tx.LookupGraphQuads(ctx, k.filter, 0)
	if err != nil {
		return err
	}
	keys := make(map[string]struct{}, len(edges))
	for _, edge := range edges {
		key, err := k.edgeKey(edge)
		if err != nil {
			continue
		}
		keys[key] = struct{}{}
	}

	// Publish them and log the size, which says how many readers it spares.
	k.publish(keys, seqno)
	logrus.WithFields(logrus.Fields{
		"predicate": k.filter.GetPredicate(),
		"edges":     len(edges),
		"keys":      len(keys),
		"revision":  seqno,
		"took":      time.Since(start).Round(time.Millisecond),
	}).Info("key index built")

	// Patch from the build's revision to the cache's.
	k.pending.Store(true)
	return nil
}

// publish stores keys labeled at, unless a set at a newer revision is
// published, and wakes the readers waiting on the set it replaces.
func (k *KeyIndex) publish(keys map[string]struct{}, at uint64) {
	old := k.set.Load()
	if old != nil && old.at >= at {
		return
	}
	k.set.Store(&keySet{at: at, keys: keys, next: make(chan struct{})})
	if old == nil {
		close(k.first)
	} else {
		close(old.next)
	}
}
