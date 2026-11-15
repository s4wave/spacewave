package sobject

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// testSkip returns a skip of op, held in state, with reason.
func testSkip(t *testing.T, state *SOState, op *SOOperation, reason string) *SOSkippedOperation {
	t.Helper()
	set, err := state.OperationSet(mockSharedObjectID)
	if err != nil {
		t.Fatal(err)
	}
	inner := mustInner(t, op)
	h := set.Find(inner.GetPeerId(), inner.GetLocalId())
	if h == nil {
		t.Fatal("operation not held")
	}
	return &SOSkippedOperation{
		Position: &SOOperationPosition{PeerId: inner.GetPeerId(), Nonce: inner.GetNonce(), OpHash: h},
		Reason:   reason,
	}
}

// applyTestSkip has priv, an owner, sign a change that adds skip to state's
// config, and applies it to state.
func applyTestSkip(t *testing.T, state *SOState, priv crypto.PrivKey, skip *SOSkippedOperation) {
	t.Helper()
	next := state.GetConfig().CloneVT()
	next.SkippedOperations = append(next.SkippedOperations, skip)
	sortSkippedOperations(next.SkippedOperations)
	change, err := BuildSOConfigChange(mockSharedObjectID, state.GetConfig(), next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SKIP_OPERATION, priv, nil)
	if err != nil {
		t.Fatal(err)
	}
	state.Config, err = VerifyConfigChange(mockSharedObjectID, state.GetConfig(), change)
	if err != nil {
		t.Fatal(err)
	}
}

// testPosition returns a position of peerID at nonce naming the hash of seed.
func testPosition(peerID string, nonce uint64, seed byte) *SOOperationPosition {
	h := sha256.Sum256([]byte{seed})
	return &SOOperationPosition{PeerId: peerID, Nonce: nonce, OpHash: h[:]}
}

// TestSkippedOperationsValidate checks the caps on a config's skips: at most
// MaxSkippedOperations entries, each a real position with a reason of at most
// MaxSkipReasonSize bytes, strictly sorted by op hash. Every replica rejects a
// config over a cap, whoever wrote it.
func TestSkippedOperationsValidate(t *testing.T) {
	peers := createMockPeers(t, 1)
	peerID := peers[0].GetPeerID().String()
	state, _ := newTestSOState(t, peers)
	skips := func(n int, reason string) []*SOSkippedOperation {
		out := make([]*SOSkippedOperation, n)
		for i := range out {
			out[i] = &SOSkippedOperation{Position: testPosition(peerID, uint64(i+1), byte(i)), Reason: reason} //nolint:gosec // a small test count.
		}
		sortSkippedOperations(out)
		return out
	}
	for _, tc := range []struct {
		name  string
		skips []*SOSkippedOperation
		ok    bool
	}{
		{"at the cap", skips(MaxSkippedOperations, strings.Repeat("r", MaxSkipReasonSize)), true},
		{"over the cap", skips(MaxSkippedOperations+1, ""), false},
		{"long reason", skips(1, strings.Repeat("r", MaxSkipReasonSize+1)), false},
		{"unsorted", []*SOSkippedOperation{skips(2, "")[1], skips(2, "")[0]}, false},
		{"duplicate", []*SOSkippedOperation{skips(1, "")[0], skips(1, "")[0]}, false},
		{"no operation", []*SOSkippedOperation{{Position: &SOOperationPosition{PeerId: peerID, Nonce: 0, OpHash: testPosition(peerID, 1, 0).GetOpHash()}}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := state.GetConfig().CloneVT()
			cfg.SkippedOperations = tc.skips
			if err := cfg.Validate(); (err == nil) != tc.ok {
				t.Fatalf("Validate returned %v; want ok %v", err, tc.ok)
			}
		})
	}
}

// TestSkipsOperationMatchesHashAuthorNonce checks that a skip names one
// operation by its hash, author and nonce together: an operation that shares
// the hash but not the author or nonce, or the author and nonce but not the
// hash, is not skipped.
func TestSkipsOperationMatchesHashAuthorNonce(t *testing.T) {
	peers := createMockPeers(t, 2)
	a, b := peers[0].GetPeerID().String(), peers[1].GetPeerID().String()
	pos := testPosition(a, 3, 1)
	cfg := &SharedObjectConfig{SkippedOperations: []*SOSkippedOperation{{Position: pos}}}
	other := testPosition(a, 3, 2).GetOpHash()
	for _, tc := range []struct {
		name string
		h    []byte
		peer string
		seq  uint64
		want bool
	}{
		{"same position", pos.GetOpHash(), a, 3, true},
		{"other nonce", pos.GetOpHash(), a, 4, false},
		{"other author", pos.GetOpHash(), b, 3, false},
		{"other hash", other, a, 3, false},
	} {
		if got := cfg.SkipsOperation(tc.h, &SOOperationInner{PeerId: tc.peer, Nonce: tc.seq}); got != tc.want {
			t.Fatalf("%s: SkipsOperation is %v; want %v", tc.name, got, tc.want)
		}
	}
}

// TestFoldRefusesSkippedOperation checks that replay refuses the operation an
// owner skipped with ReasonSkipped, whatever it would have done, and applies
// the operations around it.
func TestFoldRefusesSkippedOperation(t *testing.T) {
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	state, genesis := newTestSOState(t, peers)
	before := writeTestOp(t, state, keys[1], "before")
	stalled := writeTestOp(t, state, keys[1], "stalled")
	after := writeTestOp(t, state, keys[0], "after")
	applyTestSkip(t, state, keys[0], testSkip(t, state, stalled, "its handler is gone"))

	res := foldTest(t, testHandle(t, state, keys[0], genesis))
	for _, tc := range []struct {
		op     *SOOperation
		reason string
	}{{before, ""}, {stalled, ReasonSkipped}, {after, ""}} {
		inner := mustInner(t, tc.op)
		got := res.Outcome(inner.GetPeerId(), inner.GetLocalId())
		if got == nil || got.Reason != tc.reason {
			t.Fatalf("operation outcome %+v; want reason %q", got, tc.reason)
		}
	}
	if got := string(res.StateData); strings.Contains(got, "stalled") || !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("state %q; want before and after without stalled", got)
	}
}

// TestBodyDriverCutsSkippedAppliedOperation checks that a driver whose body
// already applied an operation an owner then skips applies again from that
// operation, refusing it, and reaches the state of a driver that starts after
// the skip. It fails when the driver keeps the applied position.
func TestBodyDriverCutsSkippedAppliedOperation(t *testing.T) {
	// The writer writes two operations and the owner one.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	state, genesis := newTestSOState(t, peers)
	w1 := writeTestOp(t, state, keys[1], "w1")
	writeTestOp(t, state, keys[1], "w2")
	writeTestOp(t, state, keys[0], "a")

	// The warm driver applies all three.
	so := &testDriverSharedObject{peerID: peers[0].GetPeerID()}
	le := logrus.New().WithField("test", t.Name())
	warm := &testBody{}
	dw := NewBodyDriver(le, so, warm, nil)
	if err := dw.Step(t.Context(), testHandle(t, state, keys[0], genesis)); err != nil {
		t.Fatal(err)
	}
	if got := warm.stateAt(len(warm.states)); !strings.Contains(got, "w1") {
		t.Fatalf("state %q before the skip; want w1 applied", got)
	}

	// The owner skips w1. Both drivers step and reach the same state.
	applyTestSkip(t, state, keys[0], testSkip(t, state, w1, ""))
	cold := &testBody{}
	dc := NewBodyDriver(le, so, cold, nil)
	for _, d := range []*BodyDriver{dw, dc} {
		if err := d.Step(t.Context(), testHandle(t, state, keys[0], genesis)); err != nil {
			t.Fatal(err)
		}
	}
	got, want := warm.stateAt(len(warm.states)), cold.stateAt(len(cold.states))
	if got != want || strings.Contains(got, "w1") || !strings.Contains(got, "w2") || !strings.Contains(got, "a") {
		t.Fatalf("after the skip the warm state is %q and the cold one %q; want them equal, with w2 and a and without w1", got, want)
	}
}

// TestSkippedOperationsPruneAndMerge checks that a config change drops only
// the skips the checkpoint covers, and that a merge joins two owners'
// concurrent skips by op hash and drops one a branch pruned.
func TestSkippedOperationsPruneAndMerge(t *testing.T) {
	peers := createMockPeers(t, 2)
	a, b := peers[0].GetPeerID().String(), peers[1].GetPeerID().String()
	covered := &SOSkippedOperation{Position: testPosition(a, 2, 1)}
	above := &SOSkippedOperation{Position: testPosition(a, 3, 2)}
	other := &SOSkippedOperation{Position: testPosition(b, 1, 3)}

	// The checkpoint covers a through nonce 2.
	set := NewSOOperationSet(mockSharedObjectID, &SOCheckpointInner{Authors: []*SOOperationPosition{testPosition(a, 2, 9)}})
	next := &SharedObjectConfig{SkippedOperations: []*SOSkippedOperation{covered, above, other}}
	sortSkippedOperations(next.SkippedOperations)
	pruneSkippedOperations(set, next)
	if len(next.GetSkippedOperations()) != 2 || next.SkipsOperation(covered.GetPosition().GetOpHash(), &SOOperationInner{PeerId: a, Nonce: 2}) {
		t.Fatalf("pruned to %v; want the two skips above the checkpoint", next.GetSkippedOperations())
	}

	// Two owners skip concurrently: the merge holds both.
	parent := &SharedObjectConfig{SkippedOperations: []*SOSkippedOperation{covered}}
	mine := &SharedObjectConfig{SkippedOperations: []*SOSkippedOperation{covered, above}}
	theirs := &SharedObjectConfig{SkippedOperations: []*SOSkippedOperation{covered, other}}
	sortSkippedOperations(mine.SkippedOperations)
	sortSkippedOperations(theirs.SkippedOperations)
	merged := applyConfigDelta(mine, parent, theirs)
	if err := validateSkippedOperations(merged.GetSkippedOperations()); err != nil || len(merged.GetSkippedOperations()) != 3 {
		t.Fatalf("merged skips %v (%v); want all three, sorted", merged.GetSkippedOperations(), err)
	}

	// A branch that pruned the covered skip drops it from the merge.
	pruned := &SharedObjectConfig{SkippedOperations: []*SOSkippedOperation{other}}
	merged = applyConfigDelta(mine, parent, pruned)
	if len(merged.GetSkippedOperations()) != 2 || merged.SkipsOperation(covered.GetPosition().GetOpHash(), &SOOperationInner{PeerId: a, Nonce: 2}) {
		t.Fatalf("merged skips %v; want above and other", merged.GetSkippedOperations())
	}
}

// TestSkipSOOperationIsFinal checks the owner's skip through SkipSOOperation:
// a skip of a position no held operation matches fails, a writer cannot skip,
// skipping again changes nothing, and a later skip keeps the earlier one, so
// no change the package makes withdraws a skip above the checkpoint.
func TestSkipSOOperationIsFinal(t *testing.T) {
	// The writer writes two operations.
	ctx := t.Context()
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	first := writeTestOp(t, state, keys[1], "first")
	second := writeTestOp(t, state, keys[1], "second")
	g := newGroupHost(ctx, state)
	skipFirst := testSkip(t, state, first, "stuck")
	skipSecond := testSkip(t, state, second, "")

	// A mistyped position and a writer's skip fail.
	wrong := skipFirst.GetPosition().CloneVT()
	wrong.Nonce++
	if err := SkipSOOperation(ctx, g.host, wrong, "", keys[0]); err == nil {
		t.Fatal("skipped a position no held operation matches")
	}
	if err := SkipSOOperation(ctx, g.host, skipFirst.GetPosition(), "", keys[1]); err == nil {
		t.Fatal("a writer skipped an operation")
	}

	// The owner skips the first operation, then again, which changes nothing.
	if err := SkipSOOperation(ctx, g.host, skipFirst.GetPosition(), skipFirst.GetReason(), keys[0]); err != nil {
		t.Fatal(err)
	}
	seqno := g.ctr.GetValue().GetConfig().GetConfigChainSeqno()
	if err := SkipSOOperation(ctx, g.host, skipFirst.GetPosition(), "again", keys[0]); err != nil {
		t.Fatal(err)
	}
	if got := g.ctr.GetValue().GetConfig().GetConfigChainSeqno(); got != seqno {
		t.Fatalf("skipping again moved the config to seqno %d from %d", got, seqno)
	}

	// A later skip keeps the first, with its reason.
	if err := SkipSOOperation(ctx, g.host, skipSecond.GetPosition(), "", keys[0]); err != nil {
		t.Fatal(err)
	}
	cfg := g.ctr.GetValue().GetConfig()
	if len(cfg.GetSkippedOperations()) != 2 {
		t.Fatalf("config skips %v; want both", cfg.GetSkippedOperations())
	}
	for _, skip := range cfg.GetSkippedOperations() {
		if skip.GetPosition().EqualVT(skipFirst.GetPosition()) && skip.GetReason() != skipFirst.GetReason() {
			t.Fatalf("first skip reason %q; want %q", skip.GetReason(), skipFirst.GetReason())
		}
	}
}

// TestStaleCheckpointKeepsSkip checks that a checkpoint an owner signs under a
// config older than a skip cannot cover the skipped operation with its effect.
// Owner B, still under the config before owner A's skip, checkpoints a state
// that applied the operation; a replica holding the skip must refuse that
// checkpoint or hold a state without the operation.
func TestStaleCheckpointKeepsSkip(t *testing.T) {
	// Two owners and a writer; the writer writes the operation A skips.
	peers := createMockPeers(t, 3)
	keys := mustPrivKeys(t, peers)
	owner := SOParticipantRole_SOParticipantRole_OWNER
	state, _ := newTestSOState(t, peers, owner, owner, SOParticipantRole_SOParticipantRole_WRITER)
	skipped := writeTestOp(t, state, keys[2], "skipped")

	// B keeps the config before the skip; A's replica takes the skip.
	stale := state.CloneVT()
	applyTestSkip(t, state, keys[0], testSkip(t, state, skipped, ""))

	// B checkpoints a state that applied the operation.
	xfrm, err := testHandle(t, stale, keys[1]).GetTransformer(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	enc, err := xfrm.EncodeBlock([]byte("skipped"))
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := stale.BuildNextCheckpoint(mockSharedObjectID, keys[1], enc)
	if err != nil {
		t.Fatal(err)
	}

	// The replica refuses it, or holds a state without the operation.
	if err := state.AdoptCheckpoint(mockSharedObjectID, checkpoint); err != nil {
		return
	}
	inner, err := state.GetCheckpoint().UnmarshalInner()
	if err != nil {
		t.Fatal(err)
	}
	data, err := xfrm.DecodeBlock(inner.GetStateData())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "skipped") {
		t.Fatalf("adopted a checkpoint signed under the config before the skip, with state %q holding the skipped operation", data)
	}
}
