package world_block

import (
	"maps"
	"os"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// SetRootHoldsEnv names the environment variable that turns off recording the
// object roots a write sets when set to "off". A recording transaction then
// names no set roots in its operations, and replay holds only the roots of
// payload operations and of direct object writes, as before set roots
// existed. It is the fallback while set roots are new.
const SetRootHoldsEnv = "SPACEWAVE_WORLD_SET_ROOT_HOLDS"

// setRootHoldsOff is read once at startup from SetRootHoldsEnv.
var setRootHoldsOff = os.Getenv(SetRootHoldsEnv) == "off"

// noteSetRoot records ref as the root the object at key holds now, replacing
// any root noted for key since the last take. Only the last root per key is
// kept here: with the changelog off, an earlier root the same operation
// replaced is never reached from the committed World, and no later operation
// can name it. With it on, noteLoggedRoot keeps the earlier roots the
// changelog links. A root in another bucket lives in another store, which this
// World's holds do not reach, and an empty root needs no hold, so either drops
// the key.
//
// Every write of an object root into the object tree calls it, so a recorder
// that takes the roots around an operation sees the final root of each object
// the operation set, however deeply the operation nested its writes.
func (t *WorldState) noteSetRoot(key string, ref *bucket.ObjectRef) {
	if setRootHoldsOff {
		return
	}
	if ref.GetBucketId() != "" || ref.GetRootRef().GetEmpty() {
		delete(t.setRoots, key)
		return
	}
	if t.setRoots == nil {
		t.setRoots = make(map[string]*block.BlockRef)
	}
	t.setRoots[key] = ref.GetRootRef().Clone()
}

// noteLoggedRoot records ref when a changelog entry links an Object record
// holding it on the entry's new side. The committed World reaches every such
// root, including one a later write in the same operation replaced or whose
// object it deleted, so each is held as the final roots are. Replay need not
// rebuild it: the operation may only name a block staged before it ran.
func (t *WorldState) noteLoggedRoot(ref *bucket.ObjectRef) {
	if setRootHoldsOff || ref.GetBucketId() != "" || ref.GetRootRef().GetEmpty() {
		return
	}
	t.loggedRoots = append(t.loggedRoots, ref.GetRootRef().Clone())
}

// moveSetRoot carries the root noted for oldKey to newKey when a rename moves
// the record, which keeps its root.
func (t *WorldState) moveSetRoot(oldKey, newKey string) {
	root, ok := t.setRoots[oldKey]
	if !ok {
		return
	}
	delete(t.setRoots, oldKey)
	t.setRoots[newKey] = root
}

// forgetSetRoot drops the root noted for key when its record leaves the
// object tree: nothing reads a deleted object's root, so nothing needs it held.
func (t *WorldState) forgetSetRoot(key string) {
	delete(t.setRoots, key)
}

// TakeSetRoots returns the final root of each object whose root was set since
// the last take, Commit or Discard, ordered by object key, then each other
// root a changelog entry links, in write order, each once, and forgets them.
// The caller serializes it with the state's writes.
func (t *WorldState) TakeSetRoots() []*block.BlockRef {
	// An operation that set no root names none.
	if len(t.setRoots) == 0 && len(t.loggedRoots) == 0 {
		return nil
	}

	// Gather the final roots in key order, then the logged roots they do not
	// already name, so the recorded entry is deterministic.
	roots := make([]*block.BlockRef, 0, len(t.setRoots)+len(t.loggedRoots))
	seen := make(map[string]struct{}, cap(roots))
	add := func(ref *block.BlockRef) {
		id := ref.MarshalString()
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			roots = append(roots, ref)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(t.setRoots)) {
		add(t.setRoots[key])
	}
	for _, ref := range t.loggedRoots {
		add(ref)
	}
	t.setRoots, t.loggedRoots = nil, nil
	return roots
}

// TakeSetRoots returns the same-bucket object roots the transaction set since
// the last call, its Commit or its Discard, and forgets them: the final root
// of each object, and each root a changelog entry links.
// A recording transaction takes them around each operation it applies, so the
// recorded operation names the roots it set; see
// world_block_tx.Tx.PayloadRefs.
func (t *Tx) TakeSetRoots() []*block.BlockRef {
	lkr := t.rmtx.Locker()
	lkr.Lock()
	defer lkr.Unlock()
	return t.state.TakeSetRoots()
}

// TakeSetRoots returns the roots the write transaction set since the last
// call, see Tx.TakeSetRoots, or nil for a read snapshot.
func (e *EngineTx) TakeSetRoots() []*block.BlockRef {
	if e.writeTx == nil {
		return nil
	}
	return e.writeTx.TakeSetRoots()
}
