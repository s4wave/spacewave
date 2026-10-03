package sobject_world_engine

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// rejectedEditsTestSharedObject records the rejected edits the engine reports.
type rejectedEditsTestSharedObject struct {
	*testSharedObject
	reports [][]*sobject.SORejectedEdit
}

// SetRejectedEdits records the reported edits.
func (s *rejectedEditsTestSharedObject) SetRejectedEdits(edits []*sobject.SORejectedEdit) {
	s.reports = append(s.reports, edits)
}

// TestReportRejectedEditsNamesConflict applies a member's creation, then
// delivers a concurrent creation of the same object that sorts first. The
// member's own edit must be reported once as rejected, losing to the other
// author, and never reported while it still applies. A remounted engine
// resuming the saved replay reports it again.
func TestReportRejectedEditsNamesConflict(t *testing.T) {
	// Two members create the same object concurrently.
	privA, pidA := newReplayTestKey(t)
	privB, pidB := newReplayTestKey(t)
	space := newReplayTestSpace(t, pidA, pidB)
	opA := space.sign("A shared", privA, "object-shared", &sobject.SOOperationLink{Nonce: 1})
	opB := space.sign("B shared", privB, "object-shared", &sobject.SOOperationLink{Nonce: 1})

	// The member whose creation sorts later loses the conflict.
	loser, winner, loserPeer, winnerPeer := opA, opB, pidA, pidB
	if bytes.Compare(opA.Hash(), opB.Hash()) < 0 {
		loser, winner, loserPeer, winnerPeer = opB, opA, pidB, pidA
	}

	// The losing member reports nothing while its creation applies.
	member := space.member(loserPeer, false)
	member.replayer.so.(*testSharedObject).localStore = store_kvtx_inmem.NewStore()
	so := &rejectedEditsTestSharedObject{testSharedObject: member.replayer.so.(*testSharedObject)}
	engine := &soEngine{so: so}
	var outcomes []replayOutcome
	deliver := func(op *sobject.SOOperation) {
		// Add the operation to the set.
		t.Helper()
		if _, err := member.set.Add(op); err != nil {
			t.Fatal(err.Error())
		}

		// Replay the set and report its rejected edits.
		var err error
		_, outcomes, err = member.replayer.replay(context.Background(), member.snap, member.set, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		engine.reportRejectedEdits(member.set, outcomes)
	}
	deliver(loser)
	if len(so.reports) != 0 {
		t.Fatalf("reported %v before the conflict", so.reports)
	}

	// The winning creation arrives and rejects it, naming the winner.
	deliver(winner)
	if len(so.reports) != 1 || len(so.reports[0]) != 1 {
		t.Fatalf("reports %v; want one rejected edit", so.reports)
	}
	edit := so.reports[0][0]
	if !bytes.Equal(edit.GetOpHash(), loser.Hash()) || edit.GetReason() == "" {
		t.Fatalf("rejected edit %v; want %x with a reason", edit, loser.Hash())
	}
	if want := []string{winnerPeer.String()}; !slices.Equal(edit.GetLostToPeerIds(), want) {
		t.Fatalf("lost to %q; want %q", edit.GetLostToPeerIds(), want)
	}

	// Replaying the same set again reports nothing new.
	engine.reportRejectedEdits(member.set, outcomes)
	if len(so.reports) != 1 {
		t.Fatalf("reported %d times; want once", len(so.reports))
	}

	// Save the replay and resume it in a new replayer.
	ctx := context.Background()
	if err := member.replayer.save(ctx); err != nil {
		t.Fatal(err.Error())
	}
	member.replayer = &replayer{c: space.c, so: so.testSharedObject}
	if err := member.replayer.load(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// A new engine on the resumed replay reports the same edit.
	remounted := &rejectedEditsTestSharedObject{testSharedObject: so.testSharedObject}
	engine = &soEngine{so: remounted}
	_, outcomes, err := member.replayer.replay(ctx, member.snap, member.set, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	engine.reportRejectedEdits(member.set, outcomes)
	if len(remounted.reports) != 1 || !slices.EqualFunc(remounted.reports[0], so.reports[0], (*sobject.SORejectedEdit).EqualVT) {
		t.Fatalf("remounted engine reported %v; want %v", remounted.reports, so.reports[0])
	}
}
