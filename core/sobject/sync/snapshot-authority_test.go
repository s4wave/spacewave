package sobject_sync

import (
	"strings"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// TestSnapshotExchangeRequiresHeldAuthority drives the real packet exchange
// with valid readable candidates that differ only at the authority boundary.
func TestSnapshotExchangeRequiresHeldAuthority(t *testing.T) {
	// Establish authority before accepting anything from the remote stream.
	const soID = "snapshot-held-authority"
	owner := mustKeyPair(t)
	reader := mustKeyPair(t)
	local := mustKeyPair(t)
	localID, err := peer.IDFromPrivateKey(local)

	// Abort if peer iDFromPrivateKey fails.
	if err != nil {
		t.Fatal(err)
	}
	initial := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
			participantCfg(mustPeerIDStr(t, owner), sobject.SOParticipantRole_SOParticipantRole_OWNER),
			participantCfg(mustPeerIDStr(t, reader), sobject.SOParticipantRole_SOParticipantRole_READER),
			participantCfg(localID.String(), sobject.SOParticipantRole_SOParticipantRole_WRITER),
		}},
		Root: &sobject.SORoot{InnerSeqno: 1},
	}
	trustSnapshotConfig(t, soID, initial, owner)
	signSnapshotRoot(t, soID, initial, owner)
	initial.RootGrants = append(initial.RootGrants, buildGrant(t, soID, owner, local.GetPublic()))

	// Each rejection preserves every held byte, including configuration and queue state.
	for _, test := range []struct {
		// name identifies the attempted authority violation.
		name string
		// mutate changes an otherwise valid candidate and, when needed, its held checkpoint.
		mutate func(held, candidate *sobject.SOState)
		// wantError names the decisive boundary rather than an unrelated earlier failure.
		wantError string
	}{
		{name: "authorized sequence jump"},
		{name: "same head self promotion", mutate: func(_, candidate *sobject.SOState) {
			candidate.Config.Participants[1].Role = sobject.SOParticipantRole_SOParticipantRole_OWNER
			signSnapshotRoot(t, soID, candidate, reader)
		}, wantError: "configuration authority"},
		{name: "chainless different head", mutate: func(_, candidate *sobject.SOState) {
			candidate.Config.ConfigChainHash[0] ^= 1
			candidate.Config.ConfigChainSeqno++
		}, wantError: "requested history"},
		{name: "empty held checkpoint", mutate: func(held, candidate *sobject.SOState) {
			held.Config.ConfigChainHash = nil
			candidate.Config = held.Config.CloneVT()
		}, wantError: "configuration authority"},
		{name: "reader signed root", mutate: func(_, candidate *sobject.SOState) {
			signSnapshotRoot(t, soID, candidate, reader)
		}, wantError: "root authority"},
		{name: "tampered root", mutate: func(_, candidate *sobject.SOState) {
			candidate.Root.Inner[0] ^= 1
		}, wantError: "root authority"},
		{name: "unsigned root", mutate: func(_, candidate *sobject.SOState) {
			candidate.Root.ValidatorSignatures = nil
		}, wantError: "consensus"},
		{name: "duplicate signer", mutate: func(_, candidate *sobject.SOState) {
			candidate.Root.ValidatorSignatures = append(candidate.Root.ValidatorSignatures, candidate.Root.ValidatorSignatures[0].CloneVT())
		}, wantError: "root authority"},
	} {
		t.Run(test.name, func(t *testing.T) {

			// cloneVT held via initial.
			held := initial.CloneVT()
			candidate := held.CloneVT()
			candidate.Root.InnerSeqno = 5
			signSnapshotRoot(t, soID, candidate, owner)
			if test.mutate != nil {
				test.mutate(held, candidate)
			}
			before := held.CloneVT()
			host, ctr := newMemHost(soID, held)
			t.Cleanup(host.ClearContext)

			// newSOSync syncer.
			syncer := NewSOSync(gateLogger(), nil, soID, localID, local, host, nil)
			data, err := candidate.MarshalVT()
			if err != nil {
				t.Fatal(err)
			}
			snapshot := &SOSyncSnapshot{SoState: data, RootSeqno: 5}
			if len(held.GetConfig().GetConfigChainHash()) == 0 {
				err = host.ImportPeerSnapshot(t.Context(), candidate, nil, localID, nil)
			} else {
				err = runSnapshotExchange(t, syncer, t.Context(), &SOSyncMessage{
					Body: &SOSyncMessage_Snapshot{Snapshot: snapshot},
				})
			}
			if test.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
				if !ctr.GetValue().EqualVT(candidate) {
					t.Fatal("authorized sequence jump did not converge")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want %q", err, test.wantError)
			}
			if !ctr.GetValue().EqualVT(before) {
				t.Fatal("rejected snapshot changed held state")
			}
		})
	}
}

// TestPeerSnapshotSameContentKeepsHeldProof accepts independently signed
// identical state while rejecting changes to either signed content component.
func TestPeerSnapshotSameContentKeepsHeldProof(t *testing.T) {

	// Perform the action.
	const soID = "same-content-independent-validators"
	first, second := mustKeyPair(t), mustKeyPair(t)
	localID, err := peer.IDFromPrivateKey(first)
	if err != nil {
		t.Fatal(err)
	}
	initial := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
			participantCfg(localID.String(), sobject.SOParticipantRole_SOParticipantRole_OWNER),
			participantCfg(mustPeerIDStr(t, second), sobject.SOParticipantRole_SOParticipantRole_OWNER),
		}},
		Root: &sobject.SORoot{InnerSeqno: 1},
	}
	trustSnapshotConfig(t, soID, initial, first)
	signSnapshotRoot(t, soID, initial, first)

	// cloneVT candidate via initial.
	candidate := initial.CloneVT()
	signSnapshotRoot(t, soID, candidate, second)
	if candidate.GetRoot().EqualVT(initial.GetRoot()) {
		t.Fatal("fixture needs independent signatures")
	}
	host, state := newMemHost(soID, initial)
	t.Cleanup(host.ClearContext)
	if err := host.ImportPeerSnapshot(t.Context(), candidate, nil, localID, nil); err != nil {
		t.Fatal(err)
	}
	if !state.GetValue().EqualVT(initial) {
		t.Fatal("same-content import replaced held proof")
	}
	for _, field := range []string{"inner", "nonces"} {
		changed := candidate.CloneVT()
		if field == "inner" {
			changed.Root.Inner = append(changed.Root.Inner, 1)
		} else {
			changed.Root.AccountNonces = []*sobject.SOAccountNonce{{PeerId: localID.String(), Nonce: 1}}
		}
		if err := host.ImportPeerSnapshot(t.Context(), changed, nil, localID, nil); err == nil || !strings.Contains(err.Error(), "conflicts") {
			t.Fatalf("changed %s accepted: %v", field, err)
		}
		if !state.GetValue().EqualVT(initial) {
			t.Fatal("conflicting import modified held state")
		}
	}
}
