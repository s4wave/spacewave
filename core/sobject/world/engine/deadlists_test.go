package sobject_world_engine

import (
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
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
	resolve := func() {
		t.Helper()
		for range 2 {
			if _, err := l.resolve(); err != nil {
				t.Fatal(err)
			}
			for _, e := range l.entries {
				if e.net != nil && e.net.encoded != nil {
					<-e.net.encoded
				}
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
	carried := func(want ...string) []*block.PutBatchEntry {
		// Render each root a save sets as + and releases as -.
		t.Helper()
		puts, roots := l.carry()
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
		return puts
	}

	// A record waiting for its naming publication is carried only once it
	// resolves.
	waiting := &pendingNet{steps: block.ReplacementSteps{{From: ref("a"), To: ref("b")}}}
	l.name(ref("a"), ref("b"), waiting)
	resolve()
	carried()
	waiting.seal(nil)
	name("b", "c", true)
	if puts := carried("+"+block.DeadlistRootName(1), "+"+block.DeadlistRootName(2)); len(puts) != 2 {
		t.Fatalf("save carries %d blocks, want 2", len(puts))
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
	if puts := carried("+"+block.DeadlistDueRootName(1), "-"+block.DeadlistRootName(1)); len(puts) != 0 {
		t.Fatalf("handoff carries %d blocks", len(puts))
	}
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
	l = restoreDeadlists(&ReplayCursor{Deadlists: l.encode(), NextDeadlist: l.next, AdoptedDeadlist: l.adopted})
	l.advance(0, false)
	if puts := carried("+"+block.DeadlistDueRootName(2), "-"+block.DeadlistRootName(2), "+"+block.DeadlistDueRootName(7), "-"+block.DeadlistRootName(7)); len(puts) != 0 {
		t.Fatalf("restored handoff carries %d blocks", len(puts))
	}
}
