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
	}
	trustSnapshotConfig(t, soID, initial, owner)
	initial.KeyEpochs = []*sobject.SOKeyEpoch{{Grants: []*sobject.SOGrant{buildGrant(t, soID, owner, local.GetPublic())}}}

	// Each rejection preserves every held byte, including configuration and operations.
	for _, test := range []struct {
		// name identifies the attempted authority violation.
		name string
		// mutate changes an otherwise valid candidate and, when needed, its held checkpoint.
		mutate func(held, candidate *sobject.SOState)
		// wantError names the decisive boundary rather than an unrelated earlier failure.
		wantError string
	}{
		{name: "authorized height jump"},
		{name: "same head self promotion", mutate: func(_, candidate *sobject.SOState) {
			candidate.Config.Participants[1].Role = sobject.SOParticipantRole_SOParticipantRole_OWNER
			advanceSnapshotCheckpoint(t, soID, candidate, reader)
		}, wantError: "configuration authority"},
		{name: "chainless different head", mutate: func(_, candidate *sobject.SOState) {
			candidate.Config.ConfigChainHash[0] ^= 1
			candidate.Config.ConfigChainSeqno++
		}, wantError: "requested history"},
		{name: "empty held checkpoint", mutate: func(held, candidate *sobject.SOState) {
			held.Config.ConfigChainHash = nil
			candidate.Config = held.Config.CloneVT()
		}, wantError: "configuration authority"},
		{name: "reader signed checkpoint", mutate: func(_, candidate *sobject.SOState) {
			advanceSnapshotCheckpoint(t, soID, candidate, reader)
		}, wantError: "checkpoint"},
		{name: "tampered checkpoint", mutate: func(_, candidate *sobject.SOState) {
			candidate.Checkpoint.Inner[0] ^= 1
		}, wantError: "checkpoint"},
		{name: "unsigned checkpoint", mutate: func(_, candidate *sobject.SOState) {
			candidate.Checkpoint.Signatures = nil
		}, wantError: "checkpoint"},
		{name: "divergent checkpoint", mutate: func(held, _ *sobject.SOState) {
			writeSyncOp(t, soID, held, owner, "covered only by the held checkpoint")
			advanceSnapshotCheckpoint(t, soID, held, owner)
		}, wantError: "does not follow"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// The owner advances the candidate two heights past the held checkpoint.
			held := initial.CloneVT()
			candidate := held.CloneVT()
			advanceSnapshotCheckpoint(t, soID, candidate, owner)
			advanceSnapshotCheckpoint(t, soID, candidate, owner)
			if test.mutate != nil {
				test.mutate(held, candidate)
			}
			before := held.CloneVT()
			host, ctr := newMemHost(soID, held)
			t.Cleanup(host.ClearContext)

			// Exchange the candidate, or import it directly when no chain is held.
			syncer := NewSOSync(gateLogger(), nil, soID, localID, local, host, nil)
			if len(held.GetConfig().GetConfigChainHash()) == 0 {
				err = host.ImportPeerSnapshot(t.Context(), candidate, nil, localID, nil)
			} else {
				err = runSnapshotExchange(t, syncer, t.Context(), snapshotMessage(t, candidate))
			}
			if test.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
				if !ctr.GetValue().EqualVT(candidate) {
					t.Fatal("authorized height jump did not converge")
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

// TestPeerSnapshotSameContentKeepsHeldProof accepts an independently signed
// copy of the held checkpoint while rejecting a different one at its height.
func TestPeerSnapshotSameContentKeepsHeldProof(t *testing.T) {
	// Two owners; the first signed the held genesis checkpoint.
	const soID = "same-content-independent-owners"
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
	}
	trustSnapshotConfig(t, soID, initial, first)

	// The second owner signs the same checkpoint body.
	inner, err := initial.GetCheckpointInner()
	if err != nil {
		t.Fatal(err)
	}
	candidate := initial.CloneVT()
	candidate.Checkpoint, err = sobject.BuildSOCheckpoint(second, inner)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.GetCheckpoint().EqualVT(initial.GetCheckpoint()) {
		t.Fatal("fixture needs independent signatures")
	}

	// Importing it keeps the held proof.
	host, state := newMemHost(soID, initial)
	t.Cleanup(host.ClearContext)
	if err := host.ImportPeerSnapshot(t.Context(), candidate, nil, localID, nil); err != nil {
		t.Fatal(err)
	}
	if !state.GetValue().EqualVT(initial) {
		t.Fatal("same-content import replaced held proof")
	}

	// A different body at the held height conflicts.
	changed := inner.CloneVT()
	changed.StateData = append(changed.StateData, 1)
	candidate.Checkpoint, err = sobject.BuildSOCheckpoint(second, changed)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.ImportPeerSnapshot(t.Context(), candidate, nil, localID, nil); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("conflicting checkpoint accepted: %v", err)
	}
	if !state.GetValue().EqualVT(initial) {
		t.Fatal("conflicting import modified held state")
	}
}
