package sharingstate

import (
	"bytes"
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
)

// ParticipantPresentation contains account labels used by sharing rows.
type ParticipantPresentation struct {
	SelfAccountID string
	SelfEntityID  string
	AccountLabels map[string]string
}

// MailboxEntry contains mailbox metadata needed by sharing state projection.
type MailboxEntry struct {
	ID        int64
	InviteID  string
	PeerID    string
	Status    string
	CreatedAt int64
	AccountID string
	EntityID  string
}

// ParticipantInfo is the app-facing participant presentation row.
type ParticipantInfo struct {
	AccountID string
	EntityID  string
	PeerIDs   []string
	Role      sobject.SOParticipantRole
	IsSelf    bool
}

// SharingState contains the sharing snapshot for a space.
type SharingState struct {
	// Participants is the canonical SharedObject audience.
	Participants []*sobject.SOParticipantConfig
	// Invites contains the owner's current invitation records.
	Invites []*sobject.SOInvite
	// MailboxEntries contains pending invitation delivery metadata.
	MailboxEntries []*MailboxEntry
	// JoinRequests contains the redemptions held for owner approval.
	JoinRequests []*sobject.SOJoinRequest
	// ViewerRole is the mounted participant's effective role.
	ViewerRole sobject.SOParticipantRole
	// CanManage indicates that the mounted participant may manage sharing.
	CanManage bool
	// ParticipantInfo groups participant presentation without conferring authority.
	ParticipantInfo []*ParticipantInfo
	// ConfigChainHash identifies the latest verified configuration entry.
	ConfigChainHash []byte
	// ConfigChainSeqno is the owner's configuration sequence number.
	ConfigChainSeqno uint64
	// ViewerPeerID is the authenticated participant of the mounted SharedObject.
	ViewerPeerID string
	// DeparturePending reports a departure the successor has not committed.
	DeparturePending bool
	// SequencerPeerID is the appointed sequencer, empty under Merge.
	SequencerPeerID string
	// Control is who controls the shared object.
	Control sobject.SOControl
	// ViewerWeight is the viewer's voting weight under group control.
	ViewerWeight uint64
	// TotalWeight is the voting weight of every voter under group control.
	TotalWeight uint64
	// Agreements are the changes voters agree to that the group has not
	// decided, by most voting weight.
	Agreements []*sobject.ControlAgreement
}

// State carries every input snapshot the sharing watch reads per emission.
type State struct {
	soState                 *sobject.SOState
	departing               []string
	mailboxEntries          []*MailboxEntry
	joinRequests            []*sobject.SOJoinRequest
	participantPresentation *ParticipantPresentation
	err                     error
	bcast                   broadcast.Broadcast
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

// SetMailboxEntries updates mailbox entries and wakes watchers.
func (s *State) SetMailboxEntries(entries []*MailboxEntry) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
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

// ViewerRole returns the current viewer's effective participant role.
func ViewerRole(state *sobject.SOState, peerID string) sobject.SOParticipantRole {
	if state == nil || peerID == "" {
		return sobject.SOParticipantRole_SOParticipantRole_UNKNOWN
	}

	role := sobject.SOParticipantRole_SOParticipantRole_UNKNOWN
	for _, participant := range state.GetConfig().GetParticipants() {
		if participant.GetPeerId() != peerID {
			continue
		}
		if participant.GetRole() > role {
			role = participant.GetRole()
		}
	}
	return role
}

// BuildParticipantInfo groups participant rows by account or peer identity.
func BuildParticipantInfo(
	soState *sobject.SOState,
	selfPeerID string,
	presentation *ParticipantPresentation,
) []*ParticipantInfo {
	if soState == nil || soState.GetConfig() == nil {
		return nil
	}

	participants := soState.GetConfig().GetParticipants()
	if len(participants) == 0 {
		return nil
	}

	rows := make(map[string]*ParticipantInfo, len(participants))
	keys := make([]string, 0, len(participants))
	for _, participant := range participants {
		peerID := participant.GetPeerId()
		if peerID == "" {
			continue
		}

		accountID := participant.GetEntityId()
		key := accountID
		if key == "" {
			key = "peer:" + peerID
		}

		row := rows[key]
		if row == nil {
			row = &ParticipantInfo{
				AccountID: accountID,
				Role:      participant.GetRole(),
			}
			if accountID != "" && presentation != nil {
				if label := presentation.AccountLabels[accountID]; label != "" {
					row.EntityID = label
				}
				if accountID == presentation.SelfAccountID && presentation.SelfEntityID != "" {
					row.EntityID = presentation.SelfEntityID
				}
			}
			rows[key] = row
			keys = append(keys, key)
		}

		if participant.GetRole() > row.Role {
			row.Role = participant.GetRole()
		}
		row.PeerIDs = append(row.PeerIDs, peerID)
		if peerID == selfPeerID {
			row.IsSelf = true
		}
	}

	if len(keys) == 0 {
		return nil
	}

	slices.SortStableFunc(keys, func(a, b string) int {
		return strings.Compare(participantSortLabel(rows[a]), participantSortLabel(rows[b]))
	})

	out := make([]*ParticipantInfo, 0, len(keys))
	for _, key := range keys {
		out = append(out, rows[key])
	}
	return out
}

func participantSortLabel(info *ParticipantInfo) string {
	if info == nil {
		return ""
	}
	if info.EntityID != "" {
		return info.EntityID
	}
	if info.AccountID != "" {
		return info.AccountID
	}
	if len(info.PeerIDs) != 0 {
		return info.PeerIDs[0]
	}
	return ""
}

// Equal reports whether two sharing snapshots contain the same projected values.
func (s *SharingState) Equal(that *SharingState) bool {
	if s == nil || that == nil {
		return s == that
	}
	return s.ViewerRole == that.ViewerRole &&
		s.CanManage == that.CanManage &&
		s.ViewerPeerID == that.ViewerPeerID &&
		s.DeparturePending == that.DeparturePending &&
		s.SequencerPeerID == that.SequencerPeerID &&
		s.Control == that.Control &&
		s.ViewerWeight == that.ViewerWeight &&
		s.TotalWeight == that.TotalWeight &&
		slices.EqualFunc(s.Agreements, that.Agreements, equalAgreement) &&
		bytes.Equal(s.ConfigChainHash, that.ConfigChainHash) &&
		s.ConfigChainSeqno == that.ConfigChainSeqno &&
		slices.EqualFunc(s.Participants, that.Participants, func(a, b *sobject.SOParticipantConfig) bool {
			return a.EqualVT(b)
		}) &&
		slices.EqualFunc(s.Invites, that.Invites, func(a, b *sobject.SOInvite) bool {
			return a.EqualVT(b)
		}) &&
		slices.EqualFunc(s.MailboxEntries, that.MailboxEntries, equalMailboxEntry) &&
		slices.EqualFunc(s.JoinRequests, that.JoinRequests, func(a, b *sobject.SOJoinRequest) bool {
			return a.EqualVT(b)
		}) &&
		slices.EqualFunc(s.ParticipantInfo, that.ParticipantInfo, equalParticipantInfo)
}

// equalAgreement reports whether two agreements name one change with the same
// voters.
func equalAgreement(a, b *sobject.ControlAgreement) bool {
	return bytes.Equal(a.Hash, b.Hash) && a.Weight == b.Weight && slices.Equal(a.Voters, b.Voters)
}

func equalMailboxEntry(a *MailboxEntry, b *MailboxEntry) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ID == b.ID &&
		a.InviteID == b.InviteID &&
		a.PeerID == b.PeerID &&
		a.Status == b.Status &&
		a.CreatedAt == b.CreatedAt &&
		a.AccountID == b.AccountID &&
		a.EntityID == b.EntityID
}

func equalParticipantInfo(a *ParticipantInfo, b *ParticipantInfo) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.AccountID == b.AccountID &&
		a.EntityID == b.EntityID &&
		a.Role == b.Role &&
		a.IsSelf == b.IsSelf &&
		slices.Equal(a.PeerIDs, b.PeerIDs)
}
