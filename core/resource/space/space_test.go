package resource_space

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// TestSharingWatchStateCoalescesNearSimultaneousChanges asserts that two
// source updates that land while the loop is mid-emission coalesce into a
// single follow-on emission, not one per source. This is the contract that
// distinguishes the unified broadcast pattern from the previous dual-channel
// pattern: the loop reads every source snapshot under the same HoldLock
// that obtains the wait channel, so updates that race with the loop's send
// fold into the next read instead of producing one extra emission per
// source.
func TestSharingWatchStateCoalescesNearSimultaneousChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	state := &sharingWatchState{
		soState: &sobject.SOState{
			Config: &sobject.SharedObjectConfig{
				Participants: []*sobject.SOParticipantConfig{
					{PeerId: "peer-1", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
				},
			},
		},
	}

	var (
		emissionsMu sync.Mutex
		emissions   []*s4wave_space.SpaceSharingState
	)
	released := make(chan struct{})
	emitted := make(chan struct{}, 8)

	send := func(s *s4wave_space.SpaceSharingState) error {
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
		loopErr <- state.runWatchLoop(ctx, "peer-1", send)
	}()

	<-emitted

	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state.soState = &sobject.SOState{
			Config: &sobject.SharedObjectConfig{
				Participants: []*sobject.SOParticipantConfig{
					{PeerId: "peer-1", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
					{PeerId: "peer-2", Role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
				},
			},
		}
		broadcast()
	})
	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state.mailboxEntries = []*s4wave_provider_spacewave.MailboxEntryInfo{
			{Id: 1, PeerId: "peer-3", Status: "pending"},
		}
		broadcast()
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
	if got := len(emissions[1].GetParticipants()); got != 2 {
		t.Fatalf("coalesced emission missing soState update: got %d participants, want 2", got)
	}
	if got := len(emissions[1].GetMailboxEntries()); got != 1 {
		t.Fatalf("coalesced emission missing mailbox update: got %d entries, want 1", got)
	}
	if !emissions[1].GetCanManage() {
		t.Fatal("coalesced emission lost owner role classification")
	}
}

// TestSharingWatchStateEqualityGateSuppressesDuplicates asserts the EqualVT
// suppression at the emit boundary catches the case where a source
// broadcasts an update whose computed snapshot is byte-identical to the
// previous emission. Two no-op writes (same value as the current snapshot)
// must not produce any extra emission; the loop reads, builds the same
// proto, and skips the send. A subsequent real change must still emit so
// the test is not just observing a stuck loop.
func TestSharingWatchStateEqualityGateSuppressesDuplicates(t *testing.T) {
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

	state := &sharingWatchState{soState: makeInitialSO()}

	var (
		emissionsMu sync.Mutex
		emissions   int
	)
	released := make(chan struct{})
	emitted := make(chan struct{}, 8)

	send := func(s *s4wave_space.SpaceSharingState) error {
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
		loopErr <- state.runWatchLoop(ctx, "peer-1", send)
	}()

	<-emitted

	for range 2 {
		state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			state.soState = makeInitialSO()
			broadcast()
		})
	}

	close(released)

	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state.soState = &sobject.SOState{
			Config: &sobject.SharedObjectConfig{
				Participants: []*sobject.SOParticipantConfig{
					{PeerId: "peer-1", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
					{PeerId: "peer-2", Role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
				},
			},
		}
		broadcast()
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

func TestBuildSpaceParticipantInfoUsesPresentationLabels(t *testing.T) {
	info := buildSpaceParticipantInfo(
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
		&sharingParticipantPresentationState{
			selfAccountID: "acct-self",
			selfEntityID:  "casey",
			accountLabels: map[string]string{
				"acct-other": "alice",
			},
		},
	)

	if len(info) != 2 {
		t.Fatalf("expected 2 participant rows, got %d", len(info))
	}
	if info[0].GetAccountId() != "acct-other" || info[0].GetEntityId() != "alice" {
		t.Fatalf("unexpected other participant row: %+v", info[0])
	}
	if info[1].GetAccountId() != "acct-self" || info[1].GetEntityId() != "casey" {
		t.Fatalf("unexpected self participant row: %+v", info[1])
	}
	if !info[1].GetIsSelf() {
		t.Fatalf("expected self row, got %+v", info[1])
	}
	if !slices.Equal(info[1].GetPeerIds(), []string{"peer-self-1", "peer-self-2"}) {
		t.Fatalf("unexpected grouped peer ids: %v", info[1].GetPeerIds())
	}
	if info[1].GetRole() != sobject.SOParticipantRole_SOParticipantRole_OWNER {
		t.Fatalf("unexpected self role: %v", info[1].GetRole())
	}
}

func TestBuildSpaceParticipantInfoFallsBackToAccountAndPeer(t *testing.T) {
	info := buildSpaceParticipantInfo(
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
		&sharingParticipantPresentationState{},
	)

	if len(info) != 2 {
		t.Fatalf("expected 2 participant rows, got %d", len(info))
	}
	if info[0].GetAccountId() != "acct-cloud" {
		t.Fatalf("unexpected cloud participant row: %+v", info[0])
	}
	if info[0].GetEntityId() != "" {
		t.Fatalf("expected no attested label fallback, got %+v", info[0])
	}
	if info[1].GetAccountId() != "" || !slices.Equal(info[1].GetPeerIds(), []string{"peer-local"}) {
		t.Fatalf("unexpected local participant row: %+v", info[1])
	}
}

// TestSpaceSharingCoalescingEndToEnd drives the sharing watch through its real
// production change source: a ccontainer.CContainer[*sobject.SOState] feeding
// the bridgeSOState goroutine. Three rapid mutations land while the loop is
// gated mid-emission. The contract is that the bridge folds those wakeups into
// the single bcast and the loop reads the latest snapshot exactly once when
// the gate releases, producing one follow-on emission for the three rapid
// mutations rather than one per mutation. This is the end-to-end version of
// the iter 1 in-broadcast coalescing test, exercising the bridge plumbing
// rather than poking state.bcast directly.
func TestSpaceSharingCoalescingEndToEnd(t *testing.T) {
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

	state := &sharingWatchState{soState: initialSO}
	ctr := ccontainer.NewCContainer[*sobject.SOState](initialSO)

	var (
		emissionsMu sync.Mutex
		emissions   []*s4wave_space.SpaceSharingState
	)
	released := make(chan struct{})
	emitted := make(chan struct{}, 16)

	send := func(s *s4wave_space.SpaceSharingState) error {
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
	go state.bridgeSOState(bridgeCtx, ctr)

	loopErr := make(chan error, 1)
	go func() {
		loopErr <- state.runWatchLoop(ctx, "peer-1", send)
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
	if got := len(last.GetParticipants()); got != 2 {
		t.Fatalf("coalesced follow-on missing latest mutation: got %d participants, want 2", got)
	}
	if got := last.GetParticipants()[1].GetPeerId(); got != "peer-4" {
		t.Fatalf("coalesced follow-on did not reflect latest mutation: got %s, want peer-4", got)
	}
	if !last.GetCanManage() {
		t.Fatal("coalesced follow-on lost owner role classification")
	}
}
