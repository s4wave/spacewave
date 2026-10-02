package sobject

import (
	"bytes"
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// TransferSOOwnership promotes a successor to OWNER in one change that carries
// the departing owner's leave consent. Only a remaining owner can re-sign the
// departed owner's grants and root, so the successor commits the departure with
// CompleteSOOwnershipTransfer. An empty successor selects the remaining
// participant with the highest role, then the earliest in configuration order.
func TransferSOOwnership(ctx context.Context, host *SOHost, owner crypto.PrivKey, successor string, request *SOLeaveRequest) (*SOConfigChange, error) {
	// The signing owner consents to its own departure from this object.
	peers, err := request.Verify()
	if err != nil {
		return nil, err
	}
	if request.GetSharedObjectId() != host.GetSharedObjectID() {
		return nil, errors.New("leave request addresses a different shared object")
	}
	ownerID, err := peer.IDFromPrivateKey(owner)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(peers, ownerID.String()) {
		return nil, errors.New("ownership transfer requires the signing owner's leave consent")
	}

	// Promote from the configuration the consent was signed at.
	state, err := host.GetHostState(ctx)
	if err != nil {
		return nil, err
	}
	current := state.GetConfig()
	if !bytes.Equal(current.GetConfigChainHash(), request.GetConfigHash()) {
		return nil, errors.New("leave consent does not match the current configuration")
	}

	// Raise the named or default successor, which must remain after the departure.
	if successor == "" {
		successor = selectSOSuccessor(current, peers)
	}
	next := current.CloneVT()
	index := slices.IndexFunc(next.GetParticipants(), func(p *SOParticipantConfig) bool { return p.GetPeerId() == successor })
	if index == -1 || slices.Contains(peers, successor) {
		return nil, errors.New("ownership successor must be a remaining participant")
	}
	next.Participants[index].Role = SOParticipantRole_SOParticipantRole_OWNER

	// Sign the promotion together with the consent it carries.
	change := newSOConfigChange(current, next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_TRANSFER_OWNERSHIP)
	change.LeaveRequest = request.CloneVT()
	if err := signSOConfigChange(change, owner); err != nil {
		return nil, err
	}
	if err := host.ApplyConfigChange(ctx, change, nil); err != nil {
		return nil, err
	}
	return change, nil
}

// CompleteSOOwnershipTransfer commits the departure carried by an ownership
// transfer at the configuration head, signed by a remaining owner. It reports
// false without changes when the head is not a transfer or owner is departing,
// so a host may call it after each configuration change it accepts.
func CompleteSOOwnershipTransfer(ctx context.Context, host *SOHost, owner crypto.PrivKey) (bool, error) {
	// Only the transition at the head carries a pending departure.
	state, err := host.GetHostState(ctx)
	if err != nil {
		return false, err
	}
	head := state.GetConfig().GetConfigChainHash()
	if len(head) == 0 {
		return false, nil
	}
	entry, err := host.ReadConfigEntry(ctx, head)
	if err != nil {
		return false, err
	}
	if entry.GetChangeType() != SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_TRANSFER_OWNERSHIP {
		return false, nil
	}

	// A departing owner cannot re-sign the proofs that outlive it.
	peers, err := entry.GetLeaveRequest().Verify()
	if err != nil {
		return false, err
	}
	ownerID, err := peer.IDFromPrivateKey(owner)
	if err != nil {
		return false, err
	}
	if slices.Contains(peers, ownerID.String()) || !isOwnerPeer(state.GetConfig(), ownerID.String()) {
		return false, nil
	}
	if _, err := LeaveSOParticipants(ctx, host, owner, entry.GetLeaveRequest()); err != nil {
		return false, err
	}
	return true, nil
}

// selectSOSuccessor returns the remaining participant with the highest role,
// the earliest in configuration order among equals, or empty if none remains.
func selectSOSuccessor(config *SharedObjectConfig, departing []string) string {
	var successor *SOParticipantConfig
	for _, p := range config.GetParticipants() {
		if !slices.Contains(departing, p.GetPeerId()) && p.GetRole() > successor.GetRole() {
			successor = p
		}
	}
	return successor.GetPeerId()
}
