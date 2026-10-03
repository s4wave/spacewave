package sobject

import (
	"context"
	"slices"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
)

// rosterFunc implements RosterHost with a function.
type rosterFunc func(ctx context.Context, dropped []string) (bool, error)

// SetRosterDropped calls f.
func (f rosterFunc) SetRosterDropped(ctx context.Context, dropped []string) (bool, error) {
	return f(ctx, dropped)
}

// TestRosterDropAndReturn checks that dropping an offline device lets the
// owner trim, that the device's late edit lands above the checkpoint and
// applies while a removed member's late edit does not, and that the
// checkpointer restores the device once it builds on the owner's latest edit.
func TestRosterDropAndReturn(t *testing.T) {
	// Owner A edits under writers B, C and D. C and D go offline holding a1
	// and each signs a late edit there.
	peers := createMockPeers(t, 4)
	keys := mustPrivKeys(t, peers)
	state, genesis := newTestSOState(t, peers)
	ids := []string{peers[0].GetPeerID().String(), peers[1].GetPeerID().String(), peers[2].GetPeerID().String(), peers[3].GetPeerID().String()}
	a1 := writeTestOp(t, state, keys[0], "a1")
	lateC := writeTestOp(t, state.CloneVT(), keys[2], "late c")
	lateD := writeTestOp(t, state.CloneVT(), keys[3], "late d")

	// The owner removes D and drops C from the roster, once.
	host, current := newTestSOHost(t.Context(), state)
	if _, err := RemoveSOParticipants(t.Context(), host, ids[3:], keys[0], nil); err != nil {
		t.Fatal(err)
	}
	for i, want := range []bool{true, false} {
		changed, err := SetSORoster(t.Context(), host, ids[2:3], keys[0])
		if err != nil {
			t.Fatal(err)
		}
		if changed != want {
			t.Fatalf("drop %d changed the roster: %v", i, changed)
		}
	}
	state = *current
	if roster := state.GetConfig().TrimRoster(); !slices.Equal(roster, ids[:2]) {
		t.Fatalf("roster %v; want A, B", roster)
	}

	// A and B reach a stable point without C, and A trims below it.
	writeTestOp(t, state, keys[0], "a2")
	writeTestOp(t, state, keys[1], "")
	set := mustOperationSet(t, state)
	checkpoint, err := state.BuildStableCheckpoint(mockSharedObjectID, keys[0], set.StablePoint(state.GetConfig().TrimRoster()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.AdoptCheckpoint(mockSharedObjectID, checkpoint); err != nil {
		t.Fatal(err)
	}
	if mustOperationSet(t, state).Get(a1.Hash()) != nil {
		t.Fatal("the checkpoint did not trim a1")
	}
	writeTestOp(t, state, keys[0], "a3")

	// Both late edits name the trimmed a1 and still land above the
	// checkpoint. C's applies; D's does not.
	for _, op := range []*SOOperation{lateC, lateD} {
		if _, err := state.AddOperation(mockSharedObjectID, op); err != nil {
			t.Fatal(err)
		}
	}
	set = mustOperationSet(t, state)
	order := set.Order()
	snap := testHandle(t, state, keys[0], genesis)
	for _, tc := range []struct {
		op     *SOOperation
		reason string
	}{{lateC, ""}, {lateD, ReasonRemoved}} {
		if !slices.ContainsFunc(order, func(h []byte) bool { return string(h) == string(tc.op.Hash()) }) {
			t.Fatal("a late edit was not placed above the checkpoint")
		}
		_, _, reason, err := PrepareReplayOp(t.Context(), snap, mustInner(t, tc.op))
		if err != nil {
			t.Fatal(err)
		}
		if reason != tc.reason {
			t.Fatalf("late edit rejected for %q; want %q", reason, tc.reason)
		}
	}

	// C has not built on a3, so it stays dropped until it acknowledges.
	if set.BuiltOnLatest(ids[2], ids[0]) {
		t.Fatal("C caught up before building on a3")
	}
	writeTestOp(t, state, keys[2], "")

	// The checkpointer restores C.
	so := &ackTestObject{t: t, priv: keys[0], state: state, ctr: ccontainer.NewCContainer[SharedObjectStateSnapshot](nil)}
	so.publish()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	restored := make(chan []string, 1)
	err = RestoreRoster(ctx, so, rosterFunc(func(_ context.Context, dropped []string) (bool, error) {
		restored <- dropped
		cancel()
		return true, nil
	}))
	if err != context.Canceled {
		t.Fatalf("restorer ended with %v", err)
	}
	if dropped := <-restored; len(dropped) != 0 {
		t.Fatalf("restorer kept %v dropped", dropped)
	}
}
