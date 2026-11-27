package sharingstate

import (
	"bytes"
	"context"
	"slices"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
)

// State carries every input snapshot the sharing watch reads per emission.
type State struct {
	// bcast guards the fields below and fires when one changes.
	bcast broadcast.Broadcast
	// soState is the SharedObject state.
	soState *sobject.SOState
	// departing are the peers that have left in their own view.
	departing []string
	// mailboxEntries are the pending invitation deliveries.
	mailboxEntries []*MailboxEntry
	// joinRequests are the redemptions held for owner approval.
	joinRequests []*sobject.SOJoinRequest
	// participantPresentation labels the participant rows.
	participantPresentation *ParticipantPresentation
	// err is the first bridge failure. The watch returns it.
	err error
}

// DepartingFunc returns the peers that have left in their own view at state,
// though the configuration still lists them.
type DepartingFunc func(ctx context.Context, state *sobject.SOState) ([]string, error)

// NewState constructs a State from the first sharing snapshot.
// The sharing state omits the departing peers.
func NewState(
	soState *sobject.SOState,
	departing []string,
	mailboxEntries []*MailboxEntry,
	presentation *ParticipantPresentation,
) *State {
	return &State{
		soState:                 soState,
		departing:               departing,
		mailboxEntries:          mailboxEntries,
		participantPresentation: presentation,
	}
}

// SetSOState updates the shared object state snapshot and its departing
// peers, and wakes watchers.
func (s *State) SetSOState(next *sobject.SOState, departing []string) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.soState = next
		s.departing = departing
		broadcast()
	})
}

// SetMailboxEntries updates mailbox entries and wakes watchers when they
// differ from the current entries.
func (s *State) SetMailboxEntries(entries []*MailboxEntry) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if slices.EqualFunc(s.mailboxEntries, entries, equalMailboxEntry) {
			return
		}
		s.mailboxEntries = entries
		broadcast()
	})
}

// SetJoinRequests updates the pending join requests and wakes watchers.
func (s *State) SetJoinRequests(requests []*sobject.SOJoinRequest) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.joinRequests = requests
		broadcast()
	})
}

// BridgeJoinRequests forwards the pending join requests into the local
// broadcast until ctx ends.
func (s *State) BridgeJoinRequests(
	ctx context.Context,
	ctr ccontainer.Watchable[*sobject.SOJoinRequestList],
) {
	current := ctr.GetValue()
	s.SetJoinRequests(current.GetRequests())
	for {
		next, err := ctr.WaitValueChange(ctx, current, nil)
		if err != nil {
			s.fail(ctx, err)
			return
		}
		current = next
		s.SetJoinRequests(current.GetRequests())
	}
}

// BridgeSOState forwards SO state container updates into the local broadcast.
// It reads the departing peers again only when the configuration head changes.
func (s *State) BridgeSOState(
	ctx context.Context,
	soStateCtr ccontainer.Watchable[*sobject.SOState],
	readDeparting DepartingFunc,
) {
	current, departing := s.soState, s.departing
	for {
		// Wait for the latest state after the current one.
		next, err := soStateCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			s.fail(ctx, err)
			return
		}
		for {
			latest := soStateCtr.GetValue()
			if latest == next {
				break
			}
			next = latest
		}

		// Read the departing peers of a new configuration head.
		head := next.GetConfig().GetConfigChainHash()
		if !bytes.Equal(head, current.GetConfig().GetConfigChainHash()) {
			departing, err = readDeparting(ctx, next)
			if err != nil {
				s.fail(ctx, err)
				return
			}
		}
		current = next
		s.SetSOState(next, departing)
	}
}

// fail ends the watch with err unless ctx was canceled.
func (s *State) fail(ctx context.Context, err error) {
	if ctx.Err() != nil {
		return
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if s.err == nil {
			s.err = err
		}
		broadcast()
	})
}

// RunWatchLoop emits a fresh SpaceSharingState whenever any folded source changes.
func (s *State) RunWatchLoop(
	ctx context.Context,
	peerID string,
	send func(*SharingState) error,
) error {
	var prevResp *SharingState
	for {
		// Project every input and the wait channel under one lock.
		var (
			resp      *SharingState
			bridgeErr error
			waitCh    <-chan struct{}
		)
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			// Read the control state from the full config, departing peers included.
			bridgeErr = s.err
			cfg := s.soState.GetConfig()
			agreements, err := s.soState.ControlAgreements()
			if bridgeErr == nil {
				bridgeErr = err
			}

			// Departing peers are absent from the projected audience.
			soState := withoutPeers(s.soState, s.departing)
			viewerRole := ViewerRole(soState, peerID)
			canManage := sobject.IsOwner(viewerRole)

			// Only owners see join requests.
			var joinRequests []*sobject.SOJoinRequest
			if canManage {
				joinRequests = s.joinRequests
			}

			// Project the sharing state and take the next wait channel.
			resp = &SharingState{
				Participants:   soState.GetConfig().GetParticipants(),
				Invites:        soState.GetInvites(),
				MailboxEntries: s.mailboxEntries,
				JoinRequests:   joinRequests,
				ViewerPeerID:   peerID,
				ViewerRole:     viewerRole,
				CanManage:      canManage,
				ParticipantInfo: BuildParticipantInfo(
					soState,
					peerID,
					s.participantPresentation,
				),
				ConfigChainHash:  soState.GetConfig().GetConfigChainHash(),
				ConfigChainSeqno: soState.GetConfig().GetConfigChainSeqno(),
				DeparturePending: len(s.departing) != 0,
				SequencerPeerID:  soState.GetConfig().GetSequencer().GetPeerId(),
				Control:          cfg.GetControl(),
				ViewerWeight:     cfg.VotingWeight(peerID),
				TotalWeight:      cfg.TotalVotingWeight(),
				Agreements:       agreements,
			}
			waitCh = getWaitCh()
		})

		// Send the projection when it changed.
		if bridgeErr != nil {
			return bridgeErr
		}
		if prevResp == nil || !resp.Equal(prevResp) {
			if err := send(resp); err != nil {
				return err
			}
			prevResp = resp
		}

		// Wait for the next source change.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

// withoutPeers returns state with the peers omitted from its participants.
func withoutPeers(state *sobject.SOState, peers []string) *sobject.SOState {
	if len(peers) == 0 || state.GetConfig() == nil {
		return state
	}
	view := state.CloneVT()
	view.Config.Participants = slices.DeleteFunc(view.Config.Participants, func(p *sobject.SOParticipantConfig) bool {
		return slices.Contains(peers, p.GetPeerId())
	})
	return view
}
