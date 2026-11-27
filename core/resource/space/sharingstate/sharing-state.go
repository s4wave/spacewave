package sharingstate

import (
	"bytes"
	"slices"

	"github.com/s4wave/spacewave/core/sobject"
)

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

// equalAgreement reports whether two agreements identify one change with the
// same voters.
func equalAgreement(a, b *sobject.ControlAgreement) bool {
	return bytes.Equal(a.Hash, b.Hash) && a.Weight == b.Weight && slices.Equal(a.Voters, b.Voters)
}
