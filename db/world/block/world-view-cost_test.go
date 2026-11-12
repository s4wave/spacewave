//go:build !js

package world_block_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/world"
)

// viewCostEngine counts the transactions one view opens and records how the
// view waits for changes.
type viewCostEngine struct {
	world.Engine

	// txs counts the transactions the view opened.
	txs atomic.Int64

	// bcast guards waitFor, tested and keyWaits and fires when one rises.
	bcast broadcast.Broadcast
	// waitFor is the highest seqno the view has waited for.
	waitFor uint64
	// tested counts the ChangeSets the feed tested against the view's filter.
	tested uint64
	// keyWaits counts the object key waits the view has started.
	keyWaits int
}

// NewTransaction counts the transaction and opens it on the Engine.
func (e *viewCostEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	e.txs.Add(1)
	return e.Engine.NewTransaction(ctx, write)
}

// WaitSeqno records value and waits on the Engine.
func (e *viewCostEngine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	e.raise(&e.waitFor, value)
	return e.Engine.WaitSeqno(ctx, value)
}

// WatchChanges records the seqno the view waits past and each ChangeSet the
// feed tests against its filter, and watches the Engine.
func (e *viewCostEngine) WatchChanges(
	ctx context.Context,
	afterSeqno uint64,
	touches world.ChangeFilter,
	cb func(seqno uint64, changes *world.ChangeSet) error,
) error {
	e.raise(&e.waitFor, afterSeqno+1)
	return world.WatchTouchingChanges(ctx, e.Engine, afterSeqno, func(changes *world.ChangeSet) bool {
		accepted := touches.Accepts(changes)
		e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			e.tested++
			broadcast()
		})
		return accepted
	}, cb)
}

// raise raises the field at v to value under bcast.
func (e *viewCostEngine) raise(v *uint64, value uint64) {
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if value > *v {
			*v = value
			broadcast()
		}
	})
}

// WaitObjectRev counts the key wait and waits on the Engine.
func (e *viewCostEngine) WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		e.keyWaits++
		broadcast()
	})
	return e.Engine.WaitObjectRev(ctx, key, rev, ignoreNotFound)
}

// waitIdle waits until the view waits for a commit after seqno, or, for a
// view that waits on object keys, until it has started keyWaits key waits.
// When tested is set, a view whose filter the feed tested a change against is
// also idle, as a view that keeps its change watch never waits again.
func (e *viewCostEngine) waitIdle(
	ctx context.Context,
	t *testing.T,
	name string,
	seqno uint64,
	keyWaits int,
	tested bool,
) {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		var idle bool
		var waitCh <-chan struct{}
		e.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			idle = e.waitFor > seqno ||
				(keyWaits != 0 && e.keyWaits >= keyWaits) ||
				(tested && e.tested != 0)
			waitCh = getWaitCh()
		})
		if idle {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-timeout:
			t.Fatalf("%s did not wait for a change after seqno %d", name, seqno)
		case <-waitCh:
		}
	}
}

// viewCostCase is one kind of World watch and the transactions it may open
// on a commit to an object it did not read.
type viewCostCase struct {
	name string
	// want is the transaction count on the unrelated commit.
	want int64
	// keyWaits is the object key waits the view starts per read, or zero for
	// a view that waits on the seqno.
	keyWaits int
	// run runs the view on eng until ctx ends.
	run func(ctx context.Context, eng world.Engine) error
}

// TestWorldViewCostOnUnrelatedCommit opens one view per kind of World watch
// on the seeded World, commits a change to an object only one of them read,
// and checks the transactions each view opens in response.
func TestWorldViewCostOnUnrelatedCommit(t *testing.T) {
	// Open an Engine on a small seeded World; the transaction counts do not
	// depend on its size.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	tbed, rootRef, err := seedWorld(ctx, 2*worldSeedObjectsPerTx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tbed.Release()
	eng, cleanup := openSeededEngine(ctx, t, tbed, rootRef)
	defer cleanup()

	// Each view reads its own seed object, except the one that reads the
	// changed object.
	changedKey := worldSeedObjectKey(1)
	cases := []viewCostCase{{
		name: "reread-per-commit",
		want: 1,
		run: func(ctx context.Context, eng world.Engine) error {
			return rereadViewPerCommit(ctx, eng, worldSeedObjectKey(2))
		},
	}, {
		name: "object-rev-wait",
		want: 1,
		run: func(ctx context.Context, eng world.Engine) error {
			return waitObjectRevView(ctx, eng, worldSeedObjectKey(3))
		},
	}, {
		name: "tracked-object",
		run: func(ctx context.Context, eng world.Engine) error {
			return runTrackedView(ctx, eng, func(ws world.WorldState) error {
				_, _, err := world.GetObjectRev(ctx, ws, worldSeedObjectKey(4))
				return err
			})
		},
	}, {
		name: "tracked-iterate",
		run: func(ctx context.Context, eng world.Engine) error {
			return runTrackedView(ctx, eng, func(ws world.WorldState) error {
				iter := ws.IterateObjects(ctx, worldSeedTypeKey(0), false)
				defer iter.Close()
				for iter.Next() {
				}
				return iter.Err()
			})
		},
	}, {
		name: "tracked-graph",
		run: func(ctx context.Context, eng world.Engine) error {
			return runTrackedView(ctx, eng, func(ws world.WorldState) error {
				filter := world.NewGraphQuadWithKeys(worldSeedObjectKey(5), worldSeedParentPred, "", "")
				_, err := ws.LookupGraphQuads(ctx, filter, 0)
				return err
			})
		},
	}, {
		name: "tracked-changed-object",
		want: 1,
		run: func(ctx context.Context, eng world.Engine) error {
			return runTrackedView(ctx, eng, func(ws world.WorldState) error {
				_, _, err := world.GetObjectRev(ctx, ws, changedKey)
				return err
			})
		},
	}, {
		name:     "key-object",
		keyWaits: 1,
		run: func(ctx context.Context, eng world.Engine) error {
			return waitObjectChangeView(ctx, eng, worldSeedObjectKey(6))
		},
	}, {
		name:     "key-missing-object",
		keyWaits: 1,
		run: func(ctx context.Context, eng world.Engine) error {
			return waitObjectChangeView(ctx, eng, "view-cost/missing")
		},
	}, {
		name:     "key-objects",
		keyWaits: 2,
		run: func(ctx context.Context, eng world.Engine) error {
			return waitObjectChangeView(ctx, eng, worldSeedObjectKey(7), worldSeedObjectKey(8))
		},
	}, {
		name:     "key-changed-object",
		want:     1,
		keyWaits: 1,
		run: func(ctx context.Context, eng world.Engine) error {
			return waitObjectChangeView(ctx, eng, changedKey)
		},
	}}

	// Start every view and wait for it to settle at the current seqno.
	seqno, err := eng.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	engs := make([]*viewCostEngine, len(cases))
	errs := make(chan error, len(cases))
	for i, c := range cases {
		engs[i] = &viewCostEngine{Engine: eng}
		go func() {
			if err := c.run(ctx, engs[i]); ctx.Err() == nil {
				errs <- err
			}
		}()
	}
	for i, c := range cases {
		engs[i].waitIdle(ctx, t, c.name, seqno, c.keyWaits, false)
	}

	// Commit a change to the changed object.
	before := make([]int64, len(cases))
	for i := range engs {
		before[i] = engs[i].txs.Load()
	}
	if err := seedWorldTx(ctx, eng, func(ws world.WorldState) error {
		// Bump the changed object's revision.
		return bumpObjectRev(ctx, ws, changedKey)
	}); err != nil {
		t.Fatal(err.Error())
	}

	// A key view on an unchanged object never wakes, so only the view on the
	// changed object starts another key wait. Its wakeup shares the commit's
	// key comparison with every other key view. A tracked view on an
	// unchanged object receives the commit's changes and keeps waiting; a
	// view that reruns waits again.
	for i, c := range cases {
		keyWaits := c.keyWaits
		if c.want != 0 {
			keyWaits += c.keyWaits
		}
		engs[i].waitIdle(ctx, t, c.name, seqno+1, keyWaits, c.want == 0)
	}

	// Compare the transactions each view opened for the commit.
	select {
	case err := <-errs:
		t.Fatalf("view ended: %v", err)
	default:
	}
	for i, c := range cases {
		got := engs[i].txs.Load() - before[i]
		t.Logf("%s: %d transactions", c.name, got)
		if got != c.want {
			t.Errorf("%s: %d transactions on the commit, want %d", c.name, got, c.want)
		}
	}
}

// rereadViewPerCommit rereads the object at key after every commit, as a
// watch that waits on the World seqno does.
func rereadViewPerCommit(ctx context.Context, eng world.Engine, key string) error {
	for {
		// Read the object and the seqno it was read at.
		tx, err := eng.NewTransaction(ctx, false)
		if err != nil {
			return err
		}
		seqno, err := tx.GetSeqno(ctx)
		if err == nil {
			_, _, err = world.GetObjectRev(ctx, tx, key)
		}
		tx.Discard()
		if err != nil {
			return err
		}

		// Wait for the next commit.
		if _, err := eng.WaitSeqno(ctx, seqno+1); err != nil {
			return err
		}
	}
}

// waitObjectRevView waits for each next revision of the object at key, as a
// watch over one object's revision does.
func waitObjectRevView(ctx context.Context, eng world.Engine, key string) error {
	// Read the current revision.
	rev, _, err := world.GetObjectRev(ctx, world.NewEngineWorldState(eng, false), key)
	if err != nil {
		return err
	}

	// Wait for each later revision.
	for {
		rev, err = world.WaitObjectRevBySeqno(ctx, eng, key, rev+1, false)
		if err != nil {
			return err
		}
	}
}

// runTrackedView runs read through a ReadSet and runs it again each time a
// read changes, as the World state watch does.
func runTrackedView(ctx context.Context, eng world.Engine, read func(ws world.WorldState) error) error {
	for {
		if err := runTrackedViewOnce(ctx, eng, read); err != nil {
			return err
		}
	}
}

// runTrackedViewOnce runs read once and waits for one of its reads to change.
func runTrackedViewOnce(ctx context.Context, eng world.Engine, read func(ws world.WorldState) error) error {
	// Open the read transaction the view reads.
	tx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	seqno, err := tx.GetSeqno(ctx)
	if err != nil {
		return err
	}

	// Read through the tracked state and wait for a change.
	reads := world.NewReadSet()
	if err := read(reads.Track(tx)); err != nil {
		return err
	}
	_, err = reads.WaitChange(ctx, eng, seqno)
	return err
}

// waitObjectChangeView reads the roots of the objects at keys and waits for
// one of them to change, as a watch over a few objects does.
func waitObjectChangeView(ctx context.Context, eng world.Engine, keys ...string) error {
	ws := world.NewEngineWorldState(eng, false)
	for {
		seen, err := world.GetObjectRootRefsBatch(ctx, ws, keys)
		if err != nil {
			return err
		}
		if err := world.WaitObjectChange(ctx, ws, seen...); err != nil {
			return err
		}
	}
}
