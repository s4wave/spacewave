package sobject

import (
	"bytes"
	"context"
	"slices"
	"testing"
)

// appendOps is a processor that appends each operation's data to the state,
// and refuses an operation whose data is "refuse".
func appendOps(_ context.Context, _ SharedObjectStateSnapshot, cur []byte, ops []*SOOperationInner) (*[]byte, []*SOOperationResult, error) {
	op := ops[0]
	if string(op.GetOpData()) == "refuse" {
		return nil, []*SOOperationResult{
			BuildSOOperationResult(op.GetPeerId(), op.GetNonce(), false, &SOOperationRejectionErrorDetails{ErrorMsg: "refused"}),
		}, nil
	}
	next := append(slices.Clone(cur), op.GetOpData()...)
	return &next, nil, nil
}

// foldTest folds snap through appendOps.
func foldTest(t *testing.T, snap SharedObjectStateSnapshot) *FoldResult {
	t.Helper()
	res, err := Fold(t.Context(), snap, appendOps)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestFoldOutcomes checks that the fold applies decodable operations from
// writers and reports why it skipped the others.
func TestFoldOutcomes(t *testing.T) {
	// Hold an owner, a writer and a reader.
	peers := createMockPeers(t, 3)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers,
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_WRITER,
		SOParticipantRole_SOParticipantRole_READER,
	)

	// The owner and writer write, the writer's processor refuses one, and the
	// reader writes, once encrypted and once in plain text.
	applied := []*SOOperation{
		writeTestOp(t, state, keys[0], "a"),
		writeTestOp(t, state, keys[1], "b"),
	}
	refused := writeTestOp(t, state, keys[1], "refuse")
	reader := writeTestOp(t, state, keys[2], "c")
	plain, err := BuildSOOperation(mockSharedObjectID, keys[0], []byte("plain"), linkAt(state, keys[0], 2), NewSOOperationLocalID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddOperation(mockSharedObjectID, plain); err != nil {
		t.Fatal(err)
	}

	// Each operation has its outcome and only the applied ones change the state.
	res := foldTest(t, testHandle(t, state, keys[2]))
	if len(res.Outcomes) != 5 {
		t.Fatalf("placed %d operations; want 5", len(res.Outcomes))
	}
	want := map[string]string{
		string(refused.Hash()): "refused",
		string(reader.Hash()):  "its author could not write to the shared object",
		string(plain.Hash()):   "its data could not be decoded",
	}
	for _, op := range applied {
		want[string(op.Hash())] = ""
	}
	for _, outcome := range res.Outcomes {
		if got := outcome.Reason; got != want[string(outcome.Hash)] {
			t.Fatalf("operation %x: reason %q; want %q", outcome.Hash, got, want[string(outcome.Hash)])
		}
	}
	if len(res.StateData) != 2 || !bytes.ContainsRune(res.StateData, 'a') || !bytes.ContainsRune(res.StateData, 'b') {
		t.Fatalf("folded state %q; want a and b", res.StateData)
	}
}

// TestFoldOfflineMembersConverge checks that two members who write while
// offline from each other each see their own write at once and reach the same
// state after exchanging operations.
func TestFoldOfflineMembersConverge(t *testing.T) {
	// Both members start from the same genesis.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	genesis, _ := newTestSOState(t, peers)
	states := []*SOState{genesis.CloneVT(), genesis.CloneVT()}

	// Each writes twice offline and sees its own writes.
	var written [2][]*SOOperation
	for i, state := range states {
		for _, data := range []string{"x", "y"} {
			written[i] = append(written[i], writeTestOp(t, state, keys[i], string(rune('a'+i))+data))
		}
		res := foldTest(t, testHandle(t, state, keys[i]))
		if len(res.Outcomes) != 2 || res.Outcomes[0].Reason != "" || res.Outcomes[1].Reason != "" {
			t.Fatalf("member %d does not see its own writes: %+v", i, res.Outcomes)
		}
	}

	// Each receives the other's operations, in the order the other wrote them.
	for i, state := range states {
		for _, op := range written[1-i] {
			if _, err := state.AddOperation(mockSharedObjectID, op); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Both hold the same set and fold it to the same state and outcomes.
	if !states[0].EqualVT(states[1]) {
		t.Fatal("members hold different states after the exchange")
	}
	res := [2]*FoldResult{}
	for i, state := range states {
		res[i] = foldTest(t, testHandle(t, state, keys[i]))
	}
	if !bytes.Equal(res[0].StateData, res[1].StateData) || len(res[0].StateData) != 8 {
		t.Fatalf("members folded %q and %q", res[0].StateData, res[1].StateData)
	}
	if !slices.EqualFunc(res[0].Outcomes, res[1].Outcomes, func(a, b FoldOutcome) bool {
		return bytes.Equal(a.Hash, b.Hash) && a.Reason == b.Reason
	}) {
		t.Fatal("members placed the operations differently")
	}
}

// TestFoldRejectsRemovedMember checks that replay skips every operation of a
// removed member, even one signed under a config that named it.
func TestFoldRejectsRemovedMember(t *testing.T) {
	// The writer writes under the genesis config.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	state, genesis := newTestSOState(t, peers)
	removed := writeTestOp(t, state, keys[1], "removed")
	kept := writeTestOp(t, state, keys[0], "kept")

	// The owner removes the writer.
	next := state.GetConfig().CloneVT()
	next.Participants = next.Participants[:1]
	removal, err := BuildSOConfigChange(mockSharedObjectID, state.GetConfig(), next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, keys[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	state.Config, err = VerifyConfigChange(mockSharedObjectID, state.GetConfig(), removal)
	if err != nil {
		t.Fatal(err)
	}
	if err := pruneRemovedParticipants(mockSharedObjectID, state, map[string]struct{}{peers[1].GetPeerID().String(): {}}, keys[0]); err != nil {
		t.Fatal(err)
	}

	// Replay applies the owner's operation and skips the removed writer's.
	res := foldTest(t, testHandle(t, state, keys[0], genesis))
	if got := res.Outcome(peers[1].GetPeerID().String(), mustInner(t, removed).GetLocalId()); got == nil || got.Reason != ReasonRemoved {
		t.Fatalf("removed member's operation outcome %+v; want %q", got, ReasonRemoved)
	}
	if got := res.Outcome(peers[0].GetPeerID().String(), mustInner(t, kept).GetLocalId()); got == nil || got.Reason != "" {
		t.Fatalf("owner's operation outcome %+v; want applied", got)
	}
	if string(res.StateData) != "kept" {
		t.Fatalf("folded state %q; want kept", res.StateData)
	}
}

// mustInner returns the verified body of op.
func mustInner(t *testing.T, op *SOOperation) *SOOperationInner {
	t.Helper()
	inner, err := op.UnmarshalInner()
	if err != nil {
		t.Fatal(err)
	}
	return inner
}
