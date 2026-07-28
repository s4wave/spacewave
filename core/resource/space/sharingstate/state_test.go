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
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

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
	)

	var (
		emissionsMu sync.Mutex
		emissions   []*SharingState
	)
	released := make(chan struct{})
	emitted := make(chan struct{}, 8)

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

	loopErr := make(chan error, 1)
	go func() {
		loopErr <- state.RunWatchLoop(ctx, "peer-1", send)
	}()

	<-emitted

	state.SetSOState(&sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{
				{PeerId: "peer-1", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
				{PeerId: "peer-2", Role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
			},
		},
	})
	state.SetMailboxEntries([]*MailboxEntry{
		{ID: 1, PeerID: "peer-3", Status: "pending"},
	})

	close(released)
	<-emitted

	cancel()
	<-loopErr

	emissionsMu.Lock()
	defer emissionsMu.Unlock()

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
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	makeInitialSO := func() *sobject.SOState {
		return &sobject.SOState{
			Config: &sobject.SharedObjectConfig{
				Participants: []*sobject.SOParticipantConfig{
					{PeerId: "peer-1", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
				},
			},
		}
	}

	state := NewState(makeInitialSO(), nil, nil)

	var (
		emissionsMu sync.Mutex
		emissions   int
	)
	released := make(chan struct{})
	emitted := make(chan struct{}, 8)

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

	loopErr := make(chan error, 1)
	go func() {
		loopErr <- state.RunWatchLoop(ctx, "peer-1", send)
	}()

	<-emitted

	for range 2 {
		state.SetSOState(makeInitialSO())
	}

	close(released)

	state.SetSOState(&sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{
				{PeerId: "peer-1", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
				{PeerId: "peer-2", Role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
			},
		},
	})

	<-emitted

	cancel()
	<-loopErr

	emissionsMu.Lock()
	defer emissionsMu.Unlock()

	if emissions != 2 {
		t.Fatalf("expected 2 emissions (initial + one real change after duplicate writes), got %d", emissions)
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
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	initialParticipants := []*sobject.SOParticipantConfig{
		{PeerId: "peer-1", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
	}
	initialSO := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: initialParticipants,
		},
	}

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

	state := NewState(initialSO, nil, nil)
	ctr := ccontainer.NewCContainer[*sobject.SOState](initialSO)
	bridgeObserved := make(chan struct{}, 1)
	bridgeRelease := make(chan struct{})

	var (
		emissionsMu sync.Mutex
		emissions   []*SharingState
	)
	released := make(chan struct{})
	emitted := make(chan struct{}, 1)
	finalPeer := "peer-4"
	finalEmitted := make(chan struct{})

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

	bridgeCtx, cancelBridge := context.WithCancel(ctx)
	defer cancelBridge()
	go state.BridgeSOState(bridgeCtx, &gatedWatchable{
		Watchable: ctr,
		observed:  bridgeObserved,
		release:   bridgeRelease,
	})

	loopErr := make(chan error, 1)
	go func() {
		loopErr <- state.RunWatchLoop(ctx, "peer-1", send)
	}()
	awaitSignal(t, emitted, "the first emission to block the send")

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

	close(bridgeRelease)
	close(released)
	awaitSignal(t, finalEmitted, "the loop to converge on the final participant set")

	cancel()
	cancelBridge()
	if err := <-loopErr; err != context.Canceled {
		t.Fatalf("watch loop returned %v, want context canceled", err)
	}

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
