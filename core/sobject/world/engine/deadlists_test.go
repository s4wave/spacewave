package sobject_world_engine

import (
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/net/hash"
)

// TestDeadlistLedger walks records through a failed save, adoption past a
// break, a reader pin and a restart, and checks that every save names only
// blocks it carries or a durable save stored, and that no orphan becomes due.
func TestDeadlistLedger(t *testing.T) {
	// ref builds a block ref from name.
	ref := func(name string) *block.BlockRef {
		r, err := block.BuildBlockRef([]byte(name), nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	// resolve takes every record whose puts resolved, waiting for the
	// encodings it starts.
	var l deadlistLedger
	var writer writerLock
	store := block_mock.NewMockStore(hash.RecommendedHashType)
	unlock, err := writer.Lock(t.Context(), "ledger-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { unlock() }()
	workerCtx, stopWorker := context.WithCancel(t.Context())
	preparer := newDeadlistPreparer(workerCtx, &writer, &l, store, nil)
	defer stopWorker()
	resolve := func() {
		t.Helper()
		for {
			if _, err := l.resolve(); err != nil {
				t.Fatal(err)
			}
			waiting := false
			for _, e := range l.entries {
				if e.net != nil && e.net.encoded != nil {
					waiting = true
				}
			}
			if !waiting {
				return
			}
			wake := l.encoded
			unlock()
			select {
			case <-wake:
			case <-t.Context().Done():
				t.Fatal(t.Context().Err())
			}
			unlock, err = writer.Lock(t.Context(), "ledger-test")
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	// name names the World to after from, with a resolved interval when ok,
	// and resolves the ledger.
	name := func(from, to string, ok bool) {
		// Parse from, which is empty for a forfeited predecessor.
		t.Helper()
		var f *block.BlockRef
		if from != "" {
			f = ref(from)
		}

		// Seal an empty interval when ok, or forfeit it.
		var net *pendingNet
		if ok {
			net = &pendingNet{steps: block.ReplacementSteps{{From: ref("from " + to), To: ref(to)}}}
			net.seal(nil)
		}
		l.name(f, ref(to), net)
		resolve()
	}

	// carried checks the roots a save carries and returns its blocks.
	carried := func(want ...string) {
		// Render each root a save sets as + and releases as -.
		t.Helper()
		roots := l.carry()
		var got []string
		for _, r := range roots {
			if r.Ref == nil {
				got = append(got, "-"+r.Name)
				continue
			}
			got = append(got, "+"+r.Name)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("save carries %v, want %v", got, want)
		}
	}

	// A record waiting for its naming publication is carried only once it
	// resolves.
	waiting := &pendingNet{steps: block.ReplacementSteps{{From: ref("a"), To: ref("b")}}}
	l.name(ref("a"), ref("b"), waiting)
	resolve()
	carried()
	waiting.seal(nil)
	name("b", "c", true)
	carried("+"+block.DeadlistRootName(1), "+"+block.DeadlistRootName(2))
	for _, entry := range l.entries {
		if exists, err := store.GetBlockExists(t.Context(), entry.record); err != nil || !exists {
			t.Fatalf("prepared record missing before naming: exists=%v error=%v", exists, err)
		}
	}

	// A failed save carries everything again; a durable one stores it.
	l.saved()
	l.unsave()
	carried("+"+block.DeadlistRootName(1), "+"+block.DeadlistRootName(2))
	l.confirm(l.saved())
	carried()

	// Adopting c with a pin from before record 2 makes only record 1 due, and
	// its handoff needs no blocks: the store holds them.
	l.adopt(ref("c"))
	if !l.advance(1, true) {
		t.Fatal("advance moved nothing")
	}
	carried("+"+block.DeadlistDueRootName(1), "-"+block.DeadlistRootName(1))
	l.confirm(l.saved())

	// Store a forfeited interval, a chain from it and a fork off the chain.
	name("c", "d", false)
	name("", "e", true)
	name("e", "f", true)
	name("x", "g", true)
	l.confirm(l.saved())

	// Adopting past a break orphans the stored records before it, which are
	// released and never due.
	name("y", "h", true)
	l.adopt(ref("h"))
	carried("-"+block.DeadlistRootName(4), "-"+block.DeadlistRootName(5), "-"+block.DeadlistRootName(6), "+"+block.DeadlistRootName(7))
	l.confirm(l.saved())

	// A restart restores the pending records as stored; with no pin open,
	// each becomes due without carrying a block.
	unlock()
	stopWorker()
	preparer.close()
	unlock, err = writer.Lock(t.Context(), "ledger-test")
	if err != nil {
		t.Fatal(err)
	}
	l = restoreDeadlists(&ReplayCursor{Deadlists: l.encode(), NextDeadlist: l.next, AdoptedDeadlist: l.adopted})
	l.advance(0, false)
	carried("+"+block.DeadlistDueRootName(2), "-"+block.DeadlistRootName(2), "+"+block.DeadlistDueRootName(7), "-"+block.DeadlistRootName(7))
}
