package sobject_sync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"net"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// TestAuthenticatedCatchupPinsPagesAndContinues proves real paired streams catch
// up across multiple history pages and then propagate configuration-only changes.
func TestAuthenticatedCatchupPinsPagesAndContinues(t *testing.T) {
	// An owner and a reader start from the same authenticated state.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	const soID = "paged-authenticated-catchup"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	initial := authenticationState(t, soID, owner, reader)
	local := newAuthenticationPeer(t, soID, owner, initial)
	remote := newAuthenticationPeer(t, soID, reader, initial)

	// Author a suffix larger than one page while the receiver remains offline.
	for range maxHistoryPageEntries + 2 {
		current, err := local.soHost.GetHostState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		change, err := sobject.BuildSOConfigChange(soID, current.Config, current.Config, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, owner, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := local.soHost.ApplyConfigChange(ctx, change, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := local.soHost.UpdateSOState(ctx, func(state *sobject.SOState) error {
		advanceSnapshotCheckpoint(t, soID, state, owner)
		writeSyncOp(t, soID, state, owner, "visible write after offline work")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	target, err := local.soHost.GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	states, release, err := remote.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Both endpoints authenticate and negotiate simultaneously over an unbuffered stream.
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	observed := &authenticationStream{Conn: left, messages: make(chan *SOSyncMessage, 64)}
	localDone, remoteDone := make(chan error, 1), make(chan error, 1)

	// The receiver accepts the complete target state.
	go func() { localDone <- local.runStream(ctx, gateLogger(), observed, "transport-a", "transport-b") }()
	go func() { remoteDone <- remote.runStream(ctx, gateLogger(), right, "transport-b", "transport-a") }()
	accepted, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		return state.EqualVT(target), nil
	}, remoteDone)
	if err != nil {
		t.Fatal(err)
	}
	if !accepted.GetCheckpoint().EqualVT(target.GetCheckpoint()) || len(accepted.GetOps()) != 1 {
		t.Fatal("accepted configuration did not bring its checkpoint and write")
	}

	// Every page stays within both limits, and the receiver can serve the retained suffix.
	var pageCount int
	for len(observed.messages) != 0 {
		message := <-observed.messages
		if page := message.GetHistoryPage(); page != nil {
			pageCount++
			if len(page.GetChanges()) > maxHistoryPageEntries || message.SizeVT() > maxMessageSize {
				t.Fatal("sender exceeded a history page budget")
			}
		}
	}
	if pageCount != 2 {
		t.Fatalf("history pages = %d, want 2", pageCount)
	}
	retained, err := remote.soHost.ReadConfigHistory(ctx, initial.Config.ConfigChainHash, target.Config.ConfigChainHash)
	if err != nil || len(retained) != maxHistoryPageEntries+2 {
		t.Fatalf("receiver retained %d changes: %v", len(retained), err)
	}

	// The same open stream must notice a configuration change without a newer checkpoint.
	change, err := sobject.BuildSOConfigChange(soID, target.Config, target.Config, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := local.soHost.ApplyConfigChange(ctx, change, nil); err != nil {
		t.Fatal(err)
	}
	accepted, err = states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		return state.GetConfig().GetConfigChainSeqno() == target.Config.ConfigChainSeqno+1, nil
	}, remoteDone)
	if err != nil {
		t.Fatal(err)
	}
	if !accepted.GetCheckpoint().EqualVT(target.GetCheckpoint()) {
		t.Fatal("configuration-only catch-up changed the checkpoint")
	}

	// Cancellation closes transport and joins all workers in both directions.
	cancel()
	for _, done := range []<-chan error{localDone, remoteDone} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("catch-up workers survived cancellation")
		}
	}
}

// TestAuthenticatedReaderCannotPromoteSnapshot submits the escalation over the
// actual authenticated protocol and checks that authority and content stay held.
func TestAuthenticatedReaderCannotPromoteSnapshot(t *testing.T) {
	// The owner and a reader start from the same authenticated state.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	const soID = "authenticated-reader-forgery"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	initial := authenticationState(t, soID, owner, reader)
	local := newAuthenticationPeer(t, soID, owner, initial)
	remote := newAuthenticationPeer(t, soID, reader, initial)

	// The reader authenticates by hand against the owner's stream.
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	done := make(chan error, 1)
	go func() { done <- local.runStream(ctx, gateLogger(), left, "transport-a", "transport-b") }()
	if _, err := remote.authenticate(ctx, stream_packet.NewSession(right, 64*1024), "transport-b", "transport-a"); err != nil {
		t.Fatal(err)
	}

	// Acknowledge the owner's head so the stream reaches catch-up.
	session := stream_packet.NewSession(right, maxMessageSize)
	localHead := &SOSyncMessage{}
	if err := session.RecvMsg(localHead); err != nil {
		t.Fatal(err)
	}
	if err := session.SendMsg(syncAcknowledgment(localHead.GetHead().GetRevision())); err != nil {
		t.Fatal(err)
	}

	// The reader signs a checkpoint under its self-promoted, same-head configuration.
	forged := initial.CloneVT()
	forged.Config.Participants[1].Role = sobject.SOParticipantRole_SOParticipantRole_OWNER
	advanceSnapshotCheckpoint(t, soID, forged, reader)
	data, err := forged.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if err := session.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Head{Head: &SOSyncHead{
		Revision: 1, StateHash: digest[:], ConfigHash: forged.Config.ConfigChainHash, ConfigSeqno: forged.Config.ConfigChainSeqno,
	}}}); err != nil {
		t.Fatal(err)
	}

	// Answer the history request with the forged snapshot.
	request := &SOSyncMessage{}
	if err := session.RecvMsg(request); err != nil {
		t.Fatal(err)
	}
	if err := session.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Snapshot{Snapshot: &SOSyncSnapshot{
		Revision: 1, BaseHash: request.GetHistoryRequest().GetBaseHash(), SoState: data,
	}}}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "configuration authority") {
			t.Fatalf("forged reader result = %v", err)
		}
	case <-ctx.Done():
		t.Fatal("forged reader did not terminate the stream")
	}
	got, err := local.soHost.GetHostState(ctx)
	if err != nil || !got.EqualVT(initial) {
		t.Fatalf("reader forgery changed held state: %v", err)
	}
}

// TestCatchupMissingHistoryRequiresRecovery preserves divergent local state.
func TestCatchupMissingHistoryRequiresRecovery(t *testing.T) {
	// The owner and a reader start from the same authenticated state.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	const soID = "missing-history-recovery"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	initial := authenticationState(t, soID, owner, reader)

	// The owner advances its config by one change.
	candidate := initial.CloneVT()
	change, err := sobject.BuildSOConfigChange(soID, initial.Config, initial.Config, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Config, err = sobject.VerifyConfigChange(soID, initial.Config, change)
	if err != nil {
		t.Fatal(err)
	}

	// The newer provider has its checkpoint but no retained path back to the receiver.
	local := newAuthenticationPeer(t, soID, owner, candidate)
	remote := newAuthenticationPeer(t, soID, reader, initial)
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	localDone, remoteDone := make(chan error, 1), make(chan error, 1)

	// Both sides stop with recovery, and the receiver keeps its state.
	go func() { localDone <- local.runStream(ctx, gateLogger(), left, "transport-a", "transport-b") }()
	go func() { remoteDone <- remote.runStream(ctx, gateLogger(), right, "transport-b", "transport-a") }()
	for _, done := range []<-chan error{localDone, remoteDone} {
		select {
		case err := <-done:
			if !errors.Is(err, sobject.ErrConfigHistoryUnavailable) {
				t.Fatalf("missing history result = %v", err)
			}
		case <-ctx.Done():
			t.Fatal("missing history did not return recovery")
		}
	}
	got, err := remote.soHost.GetHostState(ctx)
	if err != nil || !got.EqualVT(initial) {
		t.Fatalf("missing history changed receiver state: %v", err)
	}
}

// TestUnverifiableCatchupRequiresRecovery preserves local data and reports
// recovery for an empty checkpoint or an unrequested snapshot.
func TestUnverifiableCatchupRequiresRecovery(t *testing.T) {
	for _, mode := range []string{"empty checkpoint", "unrequested snapshot"} {
		t.Run(mode, func(t *testing.T) {
			// Build the shared state, without a checkpoint in the first mode.
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			const soID = "unverifiable-catchup-recovery"
			owner, reader := mustKeyPair(t), mustKeyPair(t)
			initial := authenticationState(t, soID, owner, reader)
			if mode == "empty checkpoint" {
				initial.Config.ConfigChainHash = nil
				initial.Config.ConfigChainSeqno = 0
			}

			// The owner reports each recovery decision for its peer.
			recovery := make(chan bool, 1)
			local := newObservedPeer(t, soID, owner, initial, recoveryRecorder{recovery: recovery})
			remote := newAuthenticationPeer(t, soID, reader, initial)
			left, right := net.Pipe()
			t.Cleanup(func() { left.Close(); right.Close() })
			done := make(chan error, 1)

			// An empty checkpoint fails admission at authentication.
			go func() { done <- local.runStream(ctx, gateLogger(), left, "transport-a", "transport-b") }()
			_, authErr := remote.authenticate(ctx, stream_packet.NewSession(right, 64*1024), "transport-b", "transport-a")
			if mode == "empty checkpoint" && !errors.Is(authErr, sobject.ErrConfigHistoryUnavailable) {
				t.Fatalf("empty checkpoint admission = %v", authErr)
			}

			// An authenticated reader answers with a snapshot nobody requested.
			if mode != "empty checkpoint" && authErr != nil {
				t.Fatal(authErr)
			}
			if mode == "unrequested snapshot" {
				session := stream_packet.NewSession(right, maxMessageSize)
				if err := session.RecvMsg(&SOSyncMessage{}); err != nil {
					t.Fatal(err)
				}
				data, err := initial.MarshalVT()
				if err != nil {
					t.Fatal(err)
				}
				if err := session.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Snapshot{Snapshot: &SOSyncSnapshot{SoState: data}}}); err != nil {
					t.Fatal(err)
				}
			}

			// The owner stops with recovery and keeps its state.
			select {
			case err := <-done:
				if !errors.Is(err, sobject.ErrConfigHistoryUnavailable) {
					t.Fatalf("catchup result = %v", err)
				}
			case <-ctx.Done():
				t.Fatal("catchup did not terminate")
			}
			select {
			case needed := <-recovery:
				if !needed {
					t.Fatal("catchup cleared recovery")
				}
			default:
				t.Fatal("catchup did not report recovery")
			}
			got, err := local.soHost.GetHostState(ctx)
			if err != nil || !got.EqualVT(initial) {
				t.Fatalf("catchup changed local state: %v", err)
			}
		})
	}
}

// TestConcurrentOwnerBranchesMerge proves two owners that each extended the
// config while apart converge over a real stream: the owner whose head loses
// merges its branch onto the other head, and both keep every participant. The
// large branches yield a merge record above 1 MiB, which syncs in one page.
func TestConcurrentOwnerBranchesMerge(t *testing.T) {
	t.Run("small", func(t *testing.T) { testConcurrentOwnerBranchesMerge(t, 1, 0) })
	t.Run("large", func(t *testing.T) { testConcurrentOwnerBranchesMerge(t, 40, 2048) })
}

// testConcurrentOwnerBranchesMerge runs the merge with a losing branch of
// count records and a winning branch of count+1. Each added reader carries an
// entity ID of padding bytes, so the records grow.
func testConcurrentOwnerBranchesMerge(t *testing.T, count, padding int) {
	// Two owners start from one config.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	const soID = "concurrent-owner-merge"
	ownerA, ownerB := mustKeyPair(t), mustKeyPair(t)
	initial := &sobject.SOState{Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
		participantCfg(mustPeerIDStr(t, ownerA), sobject.SOParticipantRole_SOParticipantRole_OWNER),
		participantCfg(mustPeerIDStr(t, ownerB), sobject.SOParticipantRole_SOParticipantRole_OWNER),
	}}}

	// Both hold the same genesis and its checkpoint.
	genesis, err := sobject.BuildSOConfigChange(soID, initial.Config, initial.Config, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, ownerA, nil)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Config, err = sobject.VerifyConfigChange(soID, initial.Config, genesis); err != nil {
		t.Fatal(err)
	}
	if initial.Checkpoint, err = sobject.BuildGenesisSOCheckpoint(ownerA, soID, initial.Config.GetConfigChainHash(), nil); err != nil {
		t.Fatal(err)
	}

	// Each owner runs a host retaining the genesis.
	newPeer := func(key crypto.PrivKey) *SOSync {
		// Build the host and the sync under the owner's identity.
		host, _ := newMemHostRetaining(soID, initial.CloneVT(), []*sobject.SOConfigChange{genesis})
		t.Cleanup(host.ClearContext)
		id, err := peer.IDFromPrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		return NewSOSync(gateLogger(), nil, soID, id, key, host, nopPeerObserver{})
	}
	a, b := newPeer(ownerA), newPeer(ownerB)

	// While apart, A adds count readers and B adds one more, so B's head wins.
	addReader := func(s *SOSync, signer crypto.PrivKey) string {
		// Add a new reader to the held config.
		t.Helper()
		current, err := s.soHost.GetHostState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		reader := mustPeerIDStr(t, mustKeyPair(t))
		next := current.GetConfig().CloneVT()
		participant := participantCfg(reader, sobject.SOParticipantRole_SOParticipantRole_READER)
		if padding != 0 {
			participant.EntityId = reader + strings.Repeat("e", padding)
		}
		next.Participants = append(next.Participants, participant)

		// Sign the change and apply it locally.
		change, err := sobject.BuildSOConfigChange(soID, current.GetConfig(), next, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, signer, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.soHost.ApplyConfigChange(ctx, change, nil); err != nil {
			t.Fatal(err)
		}
		return reader
	}
	var readers []string
	for range count {
		readers = append(readers, addReader(a, ownerA))
	}
	for range count + 1 {
		readers = append(readers, addReader(b, ownerB))
	}

	// Sync until both hold the same head above both branches.
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	done := make(chan error, 2)
	go func() { done <- a.runStream(ctx, gateLogger(), left, "transport-a", "transport-b") }()
	go func() { done <- b.runStream(ctx, gateLogger(), right, "transport-b", "transport-a") }()
	var heads [2]*sobject.SharedObjectConfig
	for i, s := range []*SOSync{a, b} {
		// Wait for the merge to reach this owner.
		states, release, err := s.soHost.GetSOStateCtr(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		state, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
			return state.GetConfig().GetConfigChainSeqno() == uint64(count+2), nil
		}, done)
		release()
		if err != nil {
			t.Fatal(err)
		}
		heads[i] = state.GetConfig()
	}

	// Both hold the merge record's config, with every reader from both branches.
	if !bytes.Equal(heads[0].GetConfigChainHash(), heads[1].GetConfigChainHash()) {
		t.Fatal("owners hold different heads after merging")
	}
	for _, reader := range readers {
		if !slices.ContainsFunc(heads[0].GetParticipants(), func(p *sobject.SOParticipantConfig) bool { return p.GetPeerId() == reader }) {
			t.Fatalf("merged config lacks reader %s", reader)
		}
	}
	merge, err := a.soHost.ReadConfigEntry(ctx, heads[0].GetConfigChainHash())
	if err != nil {
		t.Fatal(err)
	}
	if merge.GetChangeType() != sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_MERGE || len(merge.GetMerge().GetBranch()) != count {
		t.Fatalf("head is %s absorbing %d records, want %d", merge.GetChangeType(), len(merge.GetMerge().GetBranch()), count)
	}
	if padding != 0 && merge.SizeVT() <= 1024*1024 {
		t.Fatalf("merge record is %d bytes, want over 1 MiB", merge.SizeVT())
	}
}

// TestCatchupSendsOnlyMissingRecords catches a peer up from one op behind at
// two state sizes over real paired streams, and checks that the snapshot
// carries the missing op and not the state: its bytes stay flat between 32
// and 320 held ops. A peer whose ops a newer checkpoint covers converges to
// the same head, and a forged restore neither invents nor swaps a record.
func TestCatchupSendsOnlyMissingRecords(t *testing.T) {
	// catchUp seeds both peers with n ops, writes behind more on the sender
	// after any checkpoint, syncs the receiver, and returns the snapshot and
	// the target's full encoding size.
	payload := strings.Repeat("p", 200)
	catchUp := func(n, behind int, checkpoint bool) (*SOSyncSnapshot, int) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		const soID = "held-record-catchup"
		owner, reader := mustKeyPair(t), mustKeyPair(t)
		initial := authenticationState(t, soID, owner, reader)
		for i := range n {
			writeSyncOp(t, soID, initial, owner, payload+strconv.Itoa(i))
		}
		local := newAuthenticationPeer(t, soID, owner, initial)
		remote := newAuthenticationPeer(t, soID, reader, initial)
		if err := local.soHost.UpdateSOState(ctx, func(state *sobject.SOState) error {
			if checkpoint {
				advanceSnapshotCheckpoint(t, soID, state, owner)
			}
			for i := range behind {
				writeSyncOp(t, soID, state, owner, payload+"behind"+strconv.Itoa(i))
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		target, err := local.soHost.GetHostState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		states, release, err := remote.soHost.GetSOStateCtr(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer release()

		// Sync until the receiver holds the target, then find the snapshot
		// the sender wrote.
		left, right := net.Pipe()
		defer left.Close()
		defer right.Close()
		observed := &authenticationStream{Conn: left, messages: make(chan *SOSyncMessage, 64)}
		remoteDone := make(chan error, 1)
		go func() { _ = local.runStream(ctx, gateLogger(), observed, "transport-a", "transport-b") }()
		go func() { remoteDone <- remote.runStream(ctx, gateLogger(), right, "transport-b", "transport-a") }()
		if _, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
			return state.EqualVT(target), nil
		}, remoteDone); err != nil {
			t.Fatal(err)
		}
		var snapshot *SOSyncSnapshot
		for len(observed.messages) != 0 {
			if sent := (<-observed.messages).GetSnapshot(); sent != nil {
				snapshot = sent
			}
		}
		if snapshot == nil {
			t.Fatal("the sender wrote no snapshot")
		}
		full := target.Clone()
		full.Invites = nil
		return snapshot, full.SizeVT()
	}

	// One op behind sends that op, a run on each side of it and the small
	// fields, at both sizes.
	small, smallFull := catchUp(32, 1, false)
	large, largeFull := catchUp(320, 1, false)
	t.Logf("one op behind: snapshot %d of %d bytes at 32 ops, %d of %d at 320", small.SizeVT(), smallFull, large.SizeVT(), largeFull)
	sent := &sobject.SOState{}
	if err := sent.UnmarshalVT(large.GetSoState()); err != nil {
		t.Fatal(err)
	}
	if len(sent.GetOps()) != 1 || sent.GetConfig() != nil || sent.GetCheckpoint() != nil {
		t.Fatalf("the snapshot carries %d ops, config %v, checkpoint %v; want the missing op alone", len(sent.GetOps()), sent.GetConfig() != nil, sent.GetCheckpoint() != nil)
	}
	if large.SizeVT() > small.SizeVT()+64 {
		t.Fatalf("snapshot grew from %d to %d bytes with the held state", small.SizeVT(), large.SizeVT())
	}

	// A receiver whose ops the new checkpoint covers receives the
	// checkpoint and the new op, and converges.
	covered, _ := catchUp(32, 1, true)
	if covered.GetRestore().GetCheckpoint() || len(covered.GetRestore().GetOps()) != 0 {
		t.Fatal("the sender restored a record the receiver lacks")
	}

	// A forged restore fails: a run past the held list, one overlapping the
	// last, an empty one, or the right count from the wrong held op.
	held := []*sobject.SOOperation{{Inner: []byte("a")}, {Inner: []byte("b")}}
	delivered := []*sobject.SOOperation{{Inner: []byte("c")}}
	for _, runs := range [][]*SOSyncHeldRun{
		{{Index: 0, Offer: 1, Count: 2}},
		{{Index: 0, Offer: 0, Count: 1}, {Index: 0, Offer: 1, Count: 1}},
		{{Index: 0, Offer: 0, Count: 0}},
		{{Index: 3, Offer: 0, Count: 1}},
	} {
		if _, err := restoreHeld(delivered, runs, held); !errors.Is(err, errHeldRecord) {
			t.Fatalf("forged runs %v: want errHeldRecord, got %v", runs, err)
		}
	}
	records := []*sobject.SOOperation{held[0], delivered[0], held[1]}
	offer, err := recordHashes(held)
	if err != nil {
		t.Fatal(err)
	}
	kept, runs, err := omitHeld(records, offer)
	if err != nil || len(kept) != 1 || len(runs) != 2 {
		t.Fatalf("omitHeld kept %d in %d runs: %v", len(kept), len(runs), err)
	}
	runs[1].Offer = 0
	swapped, err := restoreHeld(kept, runs, held)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(swapped, records) {
		t.Fatal("a swapped held op rebuilt the advertised list")
	}
}

// recoveryRecorder sends each recovery decision to recovery.
type recoveryRecorder struct {
	nopPeerObserver
	recovery chan<- bool
}

// PeerRecovery records the decision.
func (r recoveryRecorder) PeerRecovery(_ peer.ID, required bool) {
	r.recovery <- required
}
