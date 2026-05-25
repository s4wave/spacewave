package sharingstate

import (
	"context"
	"slices"
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

	state := NewState(initialSO, nil, nil)
	ctr := ccontainer.NewCContainer[*sobject.SOState](initialSO)

	var (
		emissionsMu sync.Mutex
		emissions   []*SharingState
	)
	released := make(chan struct{})
	emitted := make(chan struct{}, 16)

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

	bridgeCtx, cancelBridge := context.WithCancel(ctx)
	defer cancelBridge()
	go state.BridgeSOState(bridgeCtx, ctr)

	loopErr := make(chan error, 1)
	go func() {
		loopErr <- state.RunWatchLoop(ctx, "peer-1", send)
	}()

	<-emitted

	for i := 2; i <= 4; i++ {
		next := &sobject.SOState{
			Config: &sobject.SharedObjectConfig{
				Participants: append(slices.Clone(initialParticipants),
					&sobject.SOParticipantConfig{
						PeerId: "peer-" + string(rune('0'+i)),
						Role:   sobject.SOParticipantRole_SOParticipantRole_WRITER,
					},
				),
			},
		}
		ctr.SetValue(next)
	}

	close(released)
	<-emitted

	drainTimer := time.NewTimer(50 * time.Millisecond)
	defer drainTimer.Stop()
	var extras int
drain:
	for {
		select {
		case <-emitted:
			extras++
			if !drainTimer.Stop() {
				select {
				case <-drainTimer.C:
				default:
				}
			}
			drainTimer.Reset(50 * time.Millisecond)
		case <-drainTimer.C:
			break drain
		}
	}

	cancel()
	<-loopErr

	if extras != 0 {
		t.Fatalf("expected 0 extra emissions after the coalesced follow-on, got %d", extras)
	}

	emissionsMu.Lock()
	defer emissionsMu.Unlock()

	if got := len(emissions); got != 2 {
		t.Fatalf("expected 2 emissions (initial + coalesced follow-on), got %d", got)
	}
	last := emissions[1]
	if got := len(last.Participants); got != 2 {
		t.Fatalf("coalesced follow-on missing latest mutation: got %d participants, want 2", got)
	}
	if got := last.Participants[1].GetPeerId(); got != "peer-4" {
		t.Fatalf("coalesced follow-on did not reflect latest mutation: got %s, want peer-4", got)
	}
	if !last.CanManage {
		t.Fatal("coalesced follow-on lost owner role classification")
	}
}
