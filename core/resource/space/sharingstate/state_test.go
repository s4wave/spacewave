package sharingstate

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
)

func TestWatchStateCoalescesNearSimultaneousChanges(t *testing.T) {
	// Bound the watch by the test.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start from an owner alone in the configuration.
	state := NewState(
		&sobject.SOState{
			Config: &sobject.SharedObjectConfig{
				Participants: []*sobject.SOParticipantConfig{
					{PeerId: "peer-1", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
				},
			},
		},
		nil,
		nil,
		nil,
	)

	// Record each emission.
	var (
		emissionsMu sync.Mutex
		emissions   []*SharingState
	)
	released := make(chan struct{})
	emitted := make(chan struct{}, 8)

	// Hold the first send until both sources have changed.
	send := func(s *SharingState) error {
		emissionsMu.Lock()
		idx := len(emissions)
		emissions = append(emissions, s)
		emissionsMu.Unlock()
		emitted <- struct{}{}
		if idx == 0 {
			<-released
		}
		return nil
	}

	// Run the watch loop.
	loopErr := make(chan error, 1)
	go func() {
		loopErr <- state.RunWatchLoop(ctx, "peer-1", send)
	}()

	// Wait for the initial emission.
	<-emitted

	// Change both sources while the first send is held.
	state.SetSOState(&sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{
				{PeerId: "peer-1", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
				{PeerId: "peer-2", Role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
			},
		},
	}, nil)
	state.SetMailboxEntries([]*MailboxEntry{
		{ID: 1, PeerID: "peer-3", Status: "pending"},
	})

	// Release the held send and wait for the coalesced emission.
	close(released)
	<-emitted

	// Stop the loop.
	cancel()
	<-loopErr

	// Inspect the recorded emissions.
	emissionsMu.Lock()
	defer emissionsMu.Unlock()

	// Both changes arrive in one emission.
	if got := len(emissions); got != 2 {
		t.Fatalf("expected 2 emissions (initial + coalesced), got %d", got)
	}
	if got := len(emissions[1].Participants); got != 2 {
		t.Fatalf("coalesced emission missing soState update: got %d participants, want 2", got)
	}
	if got := len(emissions[1].MailboxEntries); got != 1 {
		t.Fatalf("coalesced emission missing mailbox update: got %d entries, want 1", got)
	}
	if !emissions[1].CanManage {
		t.Fatal("coalesced emission lost owner role classification")
	}
}

func TestWatchStateEqualityGateSuppressesDuplicates(t *testing.T) {
	// Bound the watch by the test.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Build equal snapshots as distinct values.
	makeInitialSO := func() *sobject.SOState {
		return &sobject.SOState{
			Config: &sobject.SharedObjectConfig{
				Participants: []*sobject.SOParticipantConfig{
					{PeerId: "peer-1", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
				},
			},
		}
	}

	// Start from an owner alone in the configuration.
	state := NewState(makeInitialSO(), nil, nil, nil)

	// Count emissions.
	var (
		emissionsMu sync.Mutex
		emissions   int
	)
	released := make(chan struct{})
	emitted := make(chan struct{}, 8)

	// Hold the first send while duplicates are written.
	send := func(s *SharingState) error {
		emissionsMu.Lock()
		idx := emissions
		emissions++
		emissionsMu.Unlock()
		emitted <- struct{}{}
		if idx == 0 {
			<-released
		}
		return nil
	}

	// Run the watch loop.
	loopErr := make(chan error, 1)
	go func() {
		loopErr <- state.RunWatchLoop(ctx, "peer-1", send)
	}()

	// Wait for the initial emission.
	<-emitted

	// Write snapshots equal to the initial one.
	for range 2 {
		state.SetSOState(makeInitialSO(), nil)
	}

	// Release the held send.
	close(released)

	// Write a real change.
	state.SetSOState(&sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{
				{PeerId: "peer-1", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
				{PeerId: "peer-2", Role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
			},
		},
	}, nil)

	// Wait for its emission.
	<-emitted

	// Stop the loop.
	cancel()
	<-loopErr

	// Inspect the emission count.
	emissionsMu.Lock()
	defer emissionsMu.Unlock()

	// The duplicates emitted nothing.
	if emissions != 2 {
		t.Fatalf("expected 2 emissions (initial + one real change after duplicate writes), got %d", emissions)
	}
}

// TestWatchStateConfigRevisionEmitsAndDeduplicates verifies that config-chain
// metadata is part of the sharing projection and that an exact repeated
// snapshot remains suppressed after the metadata change has been emitted.
func TestWatchStateConfigRevisionEmitsAndDeduplicates(t *testing.T) {
	// Bound the watch by the test.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Build snapshots that differ only in configuration revision.
	participants := []*sobject.SOParticipantConfig{{
		PeerId: "peer-1",
		Role:   sobject.SOParticipantRole_SOParticipantRole_OWNER,
	}}
	makeState := func(hash string, seqno uint64) *sobject.SOState {
		return &sobject.SOState{Config: &sobject.SharedObjectConfig{
			Participants:     participants,
			ConfigChainHash:  []byte(hash),
			ConfigChainSeqno: seqno,
		}}
	}

	// Run the watch loop, holding the first send.
	state := NewState(makeState("config-1", 1), nil, nil, nil)
	emitted := make(chan *SharingState, 4)
	releaseFirst := make(chan struct{})
	loopErr := make(chan error, 1)
	emissions := 0
	go func() {
		loopErr <- state.RunWatchLoop(ctx, "peer-1", func(next *SharingState) error {
			// Fold the revision and its exact duplicate while the initial
			// response is in flight, matching the watch loop's coalescing path.
			emissions++
			emitted <- next
			if next.ConfigChainSeqno == 1 {
				<-releaseFirst
			}
			return nil
		})
	}()

	// The initial emission carries the first revision.
	first := <-emitted
	if first.ConfigChainSeqno != 1 {
		t.Fatalf("initial config seqno = %d, want 1", first.ConfigChainSeqno)
	}
	if first.ViewerPeerID != "peer-1" {
		t.Fatalf("initial viewer peer id = %q, want peer-1", first.ViewerPeerID)
	}
	state.SetSOState(makeState("config-2", 2), nil)
	state.SetSOState(makeState("config-2", 2), nil)
	close(releaseFirst)
	second := <-emitted
	if second.ConfigChainSeqno != 2 || string(second.ConfigChainHash) != "config-2" {
		t.Fatalf("revision snapshot = (%q, %d), want (config-2, 2)", second.ConfigChainHash, second.ConfigChainSeqno)
	}

	// Keep the loop alive long enough to observe the duplicate wakeup; a
	// third emission would violate suppression of equal source snapshots.
	state.SetSOState(makeState("config-2", 2), nil)
	select {
	case <-emitted:
		t.Fatal("exact duplicate config revision emitted a third snapshot")
	case <-time.After(100 * time.Millisecond):
	}

	// Stop the loop and count emissions.
	cancel()
	if err := <-loopErr; err != context.Canceled {
		t.Fatalf("watch loop error = %v, want context canceled", err)
	}
	if emissions != 2 {
		t.Fatalf("emissions = %d, want initial plus one revision", emissions)
	}
}

// TestWatchStateOmitsDepartingPeers checks that a viewer whose leave the head
// ownership transfer carries sees itself departed while the configuration
// still lists it as owner.
func TestWatchStateOmitsDepartingPeers(t *testing.T) {
	// Bound the watch by the test.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// The viewer is an owner whose leave the head transfer carries.
	state := NewState(&sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{
				{PeerId: "peer-1", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
				{PeerId: "peer-2", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
			},
		},
	}, []string{"peer-1"}, nil, nil)

	// Read the first emission.
	var got *SharingState
	err := state.RunWatchLoop(ctx, "peer-1", func(s *SharingState) error {
		got = s
		return context.Canceled
	})
	if err != context.Canceled {
		t.Fatalf("RunWatchLoop: %v", err)
	}
	if got.ViewerRole != sobject.SOParticipantRole_SOParticipantRole_UNKNOWN || got.CanManage || !got.DeparturePending {
		t.Fatalf("departing viewer role = %v, can manage = %v, pending = %v", got.ViewerRole, got.CanManage, got.DeparturePending)
	}
	if len(got.Participants) != 1 || got.Participants[0].GetPeerId() != "peer-2" {
		t.Fatalf("participants = %v, want only peer-2", got.Participants)
	}
	if len(got.ParticipantInfo) != 1 || got.ParticipantInfo[0].IsSelf {
		t.Fatalf("participant info = %+v, want only peer-2", got.ParticipantInfo)
	}
}

func TestBuildParticipantInfoUsesPresentationLabels(t *testing.T) {
	info := BuildParticipantInfo(
		&sobject.SOState{
			Config: &sobject.SharedObjectConfig{
				Participants: []*sobject.SOParticipantConfig{
					{
						PeerId:   "peer-self-1",
						EntityId: "acct-self",
						Role:     sobject.SOParticipantRole_SOParticipantRole_OWNER,
					},
					{
						PeerId:   "peer-self-2",
						EntityId: "acct-self",
						Role:     sobject.SOParticipantRole_SOParticipantRole_WRITER,
					},
					{
						PeerId:   "peer-other",
						EntityId: "acct-other",
						Role:     sobject.SOParticipantRole_SOParticipantRole_WRITER,
					},
				},
			},
		},
		"peer-self-1",
		&ParticipantPresentation{
			SelfAccountID: "acct-self",
			SelfEntityID:  "casey",
			AccountLabels: map[string]string{
				"acct-other": "alice",
			},
		},
	)

	if len(info) != 2 {
		t.Fatalf("expected 2 participant rows, got %d", len(info))
	}
	if info[0].AccountID != "acct-other" || info[0].EntityID != "alice" {
		t.Fatalf("unexpected other participant row: %+v", info[0])
	}
	if info[1].AccountID != "acct-self" || info[1].EntityID != "casey" {
		t.Fatalf("unexpected self participant row: %+v", info[1])
	}
	if !info[1].IsSelf {
		t.Fatalf("expected self row, got %+v", info[1])
	}
	if !slices.Equal(info[1].PeerIDs, []string{"peer-self-1", "peer-self-2"}) {
		t.Fatalf("unexpected grouped peer ids: %v", info[1].PeerIDs)
	}
	if info[1].Role != sobject.SOParticipantRole_SOParticipantRole_OWNER {
		t.Fatalf("unexpected self role: %v", info[1].Role)
	}
}

func TestBuildParticipantInfoFallsBackToAccountAndPeer(t *testing.T) {
	info := BuildParticipantInfo(
		&sobject.SOState{
			Config: &sobject.SharedObjectConfig{
				Participants: []*sobject.SOParticipantConfig{
					{
						PeerId:   "peer-cloud",
						EntityId: "acct-cloud",
						Role:     sobject.SOParticipantRole_SOParticipantRole_WRITER,
					},
					{
						PeerId: "peer-local",
						Role:   sobject.SOParticipantRole_SOParticipantRole_READER,
					},
				},
			},
		},
		"",
		&ParticipantPresentation{},
	)

	if len(info) != 2 {
		t.Fatalf("expected 2 participant rows, got %d", len(info))
	}
	if info[0].AccountID != "acct-cloud" {
		t.Fatalf("unexpected cloud participant row: %+v", info[0])
	}
	if info[0].EntityID != "" {
		t.Fatalf("expected no attested label fallback, got %+v", info[0])
	}
	if info[1].AccountID != "" || !slices.Equal(info[1].PeerIDs, []string{"peer-local"}) {
		t.Fatalf("unexpected local participant row: %+v", info[1])
	}
}

type gatedWatchable struct {
	ccontainer.Watchable[*sobject.SOState]
	observed chan<- struct{}
	release  <-chan struct{}
}

func (w *gatedWatchable) WaitValueChange(
	ctx context.Context,
	old *sobject.SOState,
	errCh <-chan error,
) (*sobject.SOState, error) {
	next, err := w.Watchable.WaitValueChange(ctx, old, errCh)
	if err != nil {
		return next, err
	}
	select {
	case w.observed <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-w.release:
		return next, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// awaitSignal receives from ch, failing the test by name if it never arrives.
//
// Each caller's signal is produced by this test's own causation and lands in
// microseconds, so the deadline is not a schedule assertion: it exists so an
// owner that stops converging reports which signal was missed instead of
// hanging until the package timeout dumps every goroutine.
func awaitSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestCoalescingEndToEnd(t *testing.T) {
	// Bound the watch by the test.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start from an owner alone in the configuration.
	initialParticipants := []*sobject.SOParticipantConfig{
		{PeerId: "peer-1", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
	}
	initialSO := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: initialParticipants,
		},
	}

	// Key participant sets for comparison.
	participantKey := func(participants []*sobject.SOParticipantConfig) string {
		keys := make([]string, 0, len(participants))
		for _, participant := range participants {
			keys = append(keys,
				participant.GetPeerId()+":"+strconv.Itoa(int(participant.GetRole()))+
					":"+participant.GetEntityId(),
			)
		}
		return strings.Join(keys, ",")
	}

	// Fold the initial state and gate the bridge.
	state := NewState(initialSO, nil, nil, nil)
	ctr := ccontainer.NewCContainer[*sobject.SOState](initialSO)
	bridgeObserved := make(chan struct{}, 1)
	bridgeRelease := make(chan struct{})

	// Record each emission.
	var (
		emissionsMu sync.Mutex
		emissions   []*SharingState
	)
	released := make(chan struct{})
	emitted := make(chan struct{}, 1)
	finalPeer := "peer-4"
	finalEmitted := make(chan struct{})

	// Hold the first send and signal the final participant set.
	send := func(s *SharingState) error {
		emissionsMu.Lock()
		idx := len(emissions)
		emissions = append(emissions, s)
		emissionsMu.Unlock()
		if idx == 0 {
			emitted <- struct{}{}
			<-released
		}
		if len(s.Participants) == 2 &&
			s.Participants[1].GetPeerId() == finalPeer {
			finalEmitted <- struct{}{}
		}
		return nil
	}

	// Bridge the gated container into the state.
	bridgeCtx, cancelBridge := context.WithCancel(ctx)
	defer cancelBridge()
	go state.BridgeSOState(bridgeCtx, &gatedWatchable{
		Watchable: ctr,
		observed:  bridgeObserved,
		release:   bridgeRelease,
	}, func(context.Context, *sobject.SOState) ([]string, error) {
		return nil, nil
	})

	// Run the watch loop.
	loopErr := make(chan error, 1)
	go func() {
		loopErr <- state.RunWatchLoop(ctx, "peer-1", send)
	}()
	awaitSignal(t, emitted, "the first emission to block the send")

	// Write three participant sets while the bridge is gated.
	written := map[string]struct{}{
		participantKey(initialParticipants): {},
	}
	for i := 2; i <= 4; i++ {
		next := &sobject.SOState{
			Config: &sobject.SharedObjectConfig{
				Participants: append(slices.Clone(initialParticipants),
					&sobject.SOParticipantConfig{
						PeerId: "peer-" + strconv.Itoa(i),
						Role:   sobject.SOParticipantRole_SOParticipantRole_WRITER,
					},
				),
			},
		}
		ctr.SetValue(next)
		written[participantKey(next.GetConfig().GetParticipants())] = struct{}{}
		if i == 2 {
			awaitSignal(t, bridgeObserved, "the bridge to observe the first write")
		}
	}

	// Release the bridge and the held send.
	close(bridgeRelease)
	close(released)
	awaitSignal(t, finalEmitted, "the loop to converge on the final participant set")

	// Stop the loop and the bridge.
	cancel()
	cancelBridge()
	if err := <-loopErr; err != context.Canceled {
		t.Fatalf("watch loop returned %v, want context canceled", err)
	}

	// The last emission is the final set, and every emission was written.
	emissionsMu.Lock()
	defer emissionsMu.Unlock()
	last := emissions[len(emissions)-1]
	finalParticipants := []*sobject.SOParticipantConfig{
		initialParticipants[0],
		{PeerId: finalPeer, Role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
	}
	if got, want := participantKey(last.Participants), participantKey(finalParticipants); got != want {
		t.Fatalf("last emission has participant set %q, want %q", got, want)
	}
	if !last.CanManage {
		t.Fatal("final emission lost owner role classification")
	}
	for _, emission := range emissions {
		if _, ok := written[participantKey(emission.Participants)]; !ok {
			t.Fatalf("emission carried unwritten participant set %q", participantKey(emission.Participants))
		}
	}
}
