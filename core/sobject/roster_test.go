package sobject

import (
	"context"
	"slices"
	"strings"
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
	err = MaintainRoster(ctx, so, rosterFunc(func(_ context.Context, dropped []string) (bool, error) {
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

// TestNextRosterDropped checks that the checkpointer drops the members that
// lag behind it only once the operations outgrow RosterDropBytes, never drops
// itself or a member that built on its latest edit, and keeps a dropped device
// until it builds on that edit.
func TestNextRosterDropped(t *testing.T) {
	// Owner A and writers B and C. B answers A's first edit; C is offline.
	peers := createMockPeers(t, 3)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	ids := []string{peers[0].GetPeerID().String(), peers[1].GetPeerID().String(), peers[2].GetPeerID().String()}

	// Nothing drops below the budget.
	writeTestOp(t, state, keys[0], "a1")
	writeTestOp(t, state, keys[1], "")
	cfg := state.GetConfig()
	set := mustOperationSet(t, state)
	if got := nextRosterDropped(cfg, set, ids[0]); len(got) != 0 {
		t.Fatalf("dropped %v below the budget", got)
	}

	// Edits past the budget drop C, which has built on nothing, and keep B.
	edit := strings.Repeat("x", MaxInnerDataSize/2)
	for set.Size() < RosterDropBytes {
		writeTestOp(t, state, keys[0], edit)
		set = mustOperationSet(t, state)
	}
	writeTestOp(t, state, keys[1], "")
	set = mustOperationSet(t, state)
	got := nextRosterDropped(cfg, set, ids[0])
	if !slices.Equal(got, ids[2:]) {
		t.Fatalf("dropped %v past the budget; want C", got)
	}

	// Without C, all but B's last acknowledgment is stable, so A can trim it.
	trimming := cfg.CloneVT()
	trimming.RosterDroppedPeerIds = got
	if stable := set.StablePoint(trimming.TrimRoster()); len(stable) != len(set.Order())-1 {
		t.Fatalf("stable point holds %d of %d operations", len(stable), len(set.Order()))
	}

	// B leaves the budget's edits unanswered, so it is dropped with C.
	writeTestOp(t, state, keys[0], edit)
	set = mustOperationSet(t, state)
	if got := nextRosterDropped(cfg, set, ids[0]); !slices.Equal(got, slices.Sorted(slices.Values(ids[1:]))) {
		t.Fatalf("dropped %v; want B and C", got)
	}

	// A dropped C stays dropped until it builds on A's latest edit.
	dropped := cfg.CloneVT()
	dropped.RosterDroppedPeerIds = ids[2:]
	if got := nextRosterDropped(dropped, set, ids[0]); !slices.Contains(got, ids[2]) {
		t.Fatalf("restored C before it built on A's latest edit: %v", got)
	}
	writeTestOp(t, state, keys[2], "")
	set = mustOperationSet(t, state)
	if got := nextRosterDropped(dropped, set, ids[0]); slices.Contains(got, ids[2]) {
		t.Fatalf("kept C dropped after it built on A's latest edit: %v", got)
	}
}

// TestMaintainRosterDropsOfflineDevice checks that the checkpointer, watching
// a state that outgrew RosterDropBytes, drops the device that never built on
// its edits.
func TestMaintainRosterDropsOfflineDevice(t *testing.T) {
	// Owner A writes past the budget while writer B is offline.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	edit := strings.Repeat("x", MaxInnerDataSize/2)
	for mustOperationSet(t, state).Size() < RosterDropBytes {
		writeTestOp(t, state, keys[0], edit)
	}
	so := &ackTestObject{t: t, priv: keys[0], state: state, ctr: ccontainer.NewCContainer[SharedObjectStateSnapshot](nil)}
	so.publish()

	// The checkpointer drops B.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	dropped := make(chan []string, 1)
	err := MaintainRoster(ctx, so, rosterFunc(func(_ context.Context, ids []string) (bool, error) {
		dropped <- ids
		cancel()
		return true, nil
	}))
	if err != context.Canceled {
		t.Fatalf("maintainer ended with %v", err)
	}
	if got := <-dropped; !slices.Equal(got, []string{peers[1].GetPeerID().String()}) {
		t.Fatalf("maintainer dropped %v; want B", got)
	}
}
