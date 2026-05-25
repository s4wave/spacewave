package sharingstate

import (
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
	Participants    []*sobject.SOParticipantConfig
	Invites         []*sobject.SOInvite
	MailboxEntries  []*MailboxEntry
	ViewerRole      sobject.SOParticipantRole
	CanManage       bool
	ParticipantInfo []*ParticipantInfo
}

// State carries every input snapshot the sharing watch reads per emission.
type State struct {
	soState                 *sobject.SOState
	mailboxEntries          []*MailboxEntry
	participantPresentation *ParticipantPresentation
	err                     error
	bcast                   broadcast.Broadcast
}

// NewState returns a sharing watch state owner.
func NewState(
	soState *sobject.SOState,
	mailboxEntries []*MailboxEntry,
	presentation *ParticipantPresentation,
) *State {
	return &State{
		soState:                 soState,
		mailboxEntries:          mailboxEntries,
		participantPresentation: presentation,
	}
}

// SetSOState updates the shared object state snapshot and wakes watchers.
func (s *State) SetSOState(next *sobject.SOState) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.soState = next
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

// BridgeSOState forwards SO state container updates into the local broadcast.
func (s *State) BridgeSOState(
	ctx context.Context,
	soStateCtr ccontainer.Watchable[*sobject.SOState],
) {
	current := s.soState
	for {
		next, err := soStateCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			if ctx.Err() == nil {
				s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
					if s.err == nil {
						s.err = err
					}
					broadcast()
				})
			}
			return
		}
		for {
			latest := soStateCtr.GetValue()
			if latest == next {
				break
			}
			next = latest
		}
		current = next
		s.SetSOState(next)
	}
}

// RunWatchLoop emits a fresh SpaceSharingState whenever any folded source changes.
func (s *State) RunWatchLoop(
	ctx context.Context,
	peerID string,
	send func(*SharingState) error,
) error {
	var prevResp *SharingState
	for {
		var (
			resp      *SharingState
			bridgeErr error
			waitCh    <-chan struct{}
		)
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			bridgeErr = s.err
			viewerRole := ViewerRole(s.soState, peerID)
			resp = &SharingState{
				Participants:   s.soState.GetConfig().GetParticipants(),
				Invites:        s.soState.GetInvites(),
				MailboxEntries: s.mailboxEntries,
				ViewerRole:     viewerRole,
				CanManage:      sobject.IsOwner(viewerRole),
				ParticipantInfo: BuildParticipantInfo(
					s.soState,
					peerID,
					s.participantPresentation,
				),
			}
			waitCh = getWaitCh()
		})
		if bridgeErr != nil {
			return bridgeErr
		}
		if prevResp == nil || !resp.Equal(prevResp) {
			if err := send(resp); err != nil {
				return err
			}
			prevResp = resp
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
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
		slices.EqualFunc(s.Participants, that.Participants, func(a, b *sobject.SOParticipantConfig) bool {
			return a.EqualVT(b)
		}) &&
		slices.EqualFunc(s.Invites, that.Invites, func(a, b *sobject.SOInvite) bool {
			return a.EqualVT(b)
		}) &&
		slices.EqualFunc(s.MailboxEntries, that.MailboxEntries, equalMailboxEntry) &&
		slices.EqualFunc(s.ParticipantInfo, that.ParticipantInfo, equalParticipantInfo)
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
