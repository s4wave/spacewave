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
// departed owner's grants and checkpoint, so the successor commits the departure with
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
	if slices.Contains(peers, successor) {
		return nil, errors.New("ownership successor must be a remaining participant")
	}
	return applySOOwnerPromotion(ctx, host, owner, current, successor, request)
}

// PromoteSOOwner raises a participant to OWNER without a departure. A host
// whose session identity received a transfer promotes its storage identity.
func PromoteSOOwner(ctx context.Context, host *SOHost, owner crypto.PrivKey, peerID string) error {
	// An existing owner needs no promotion.
	state, err := host.GetHostState(ctx)
	if err != nil {
		return err
	}
	if isOwnerPeer(state.GetConfig(), peerID) {
		return nil
	}

	// Raise the participant without carrying a departure.
	_, err = applySOOwnerPromotion(ctx, host, owner, state.GetConfig(), peerID, nil)
	return err
}

// applySOOwnerPromotion signs one TRANSFER_OWNERSHIP of current that raises
// successor and carries request, which is nil without a departure.
func applySOOwnerPromotion(
	ctx context.Context,
	host *SOHost,
	owner crypto.PrivKey,
	current *SharedObjectConfig,
	successor string,
	request *SOLeaveRequest,
) (*SOConfigChange, error) {
	// Raise the successor in a copy of the current configuration.
	next := current.CloneVT()
	index := slices.IndexFunc(next.GetParticipants(), func(p *SOParticipantConfig) bool { return p.GetPeerId() == successor })
	if index == -1 {
		return nil, errors.New("ownership successor must be a remaining participant")
	}
	next.Participants[index].Role = SOParticipantRole_SOParticipantRole_OWNER

	// Sign the promotion together with the consent it carries.
	change := newSOConfigChange(host.GetSharedObjectID(), current, next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_TRANSFER_OWNERSHIP)
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
// transfer at the configuration head, signed by a remaining owner. The same
// change raises each peer in promote to OWNER, so the successor's account can
// sign as host as soon as the departure settles. It reports false without
// changes when the head carries no departure or owner is departing, so a host
// may call it after each configuration change it accepts.
func CompleteSOOwnershipTransfer(ctx context.Context, host *SOHost, owner crypto.PrivKey, promote ...string) (bool, error) {
	// Only an owner at the head can commit the departure it carries.
	state, err := host.GetHostState(ctx)
	if err != nil {
		return false, err
	}
	ownerID, err := peer.IDFromPrivateKey(owner)
	if err != nil {
		return false, err
	}
	head := state.GetConfig().GetConfigChainHash()
	if len(head) == 0 || !isOwnerPeer(state.GetConfig(), ownerID.String()) {
		return false, nil
	}

	// Read the departure the head transition carries.
	entry, err := host.ReadConfigEntry(ctx, head)
	if err != nil {
		return false, err
	}
	peers, err := SODepartingPeers(entry)
	if err != nil || len(peers) == 0 {
		return false, err
	}

	// A departing owner cannot re-sign the proofs that outlive it.
	if slices.Contains(peers, ownerID.String()) {
		return false, nil
	}
	if _, err := leaveSOParticipants(ctx, host, owner, entry.GetLeaveRequest(), promote); err != nil {
		return false, err
	}
	return true, nil
}

// ReadSOOwnershipSuccessor returns the owner that the latest retained departure
// transfer handed the object to, or empty when retained history has none. That
// is the first owner in the transfer's configuration that was not departing,
// which is the promoted successor unless another owner already remained.
func ReadSOOwnershipSuccessor(ctx context.Context, host *SOHost) (string, error) {
	// Walk back from the head to the latest departure transfer.
	state, err := host.GetHostState(ctx)
	if err != nil {
		return "", err
	}
	head := state.GetConfig().GetConfigChainHash()
	var entry *SOConfigChange
	for len(head) != 0 && !isSODepartureTransfer(entry) {
		entry, err = host.ReadConfigEntry(ctx, head)
		if errors.Is(err, ErrConfigHistoryUnavailable) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		head = entry.GetPreviousHash()
	}
	if !isSODepartureTransfer(entry) {
		return "", nil
	}

	// The successor is the first owner the departure leaves behind.
	peers, err := entry.GetLeaveRequest().Verify()
	if err != nil {
		return "", err
	}
	for _, p := range entry.GetConfig().GetParticipants() {
		if IsOwner(p.GetRole()) && !slices.Contains(peers, p.GetPeerId()) {
			return p.GetPeerId(), nil
		}
	}
	return "", nil
}

// SODepartingPeers returns the peers whose verified leave consent the change
// carries when it is an ownership transfer, or nil for any other change. Each
// has departed in its own view before the successor commits its removal.
func SODepartingPeers(entry *SOConfigChange) ([]string, error) {
	if !isSODepartureTransfer(entry) {
		return nil, nil
	}
	return entry.GetLeaveRequest().Verify()
}

// ReadSODepartingPeers returns SODepartingPeers for the retained change that
// produced head, or nil when head is empty or its change is not retained.
func ReadSODepartingPeers(ctx context.Context, host *SOHost, head []byte) ([]string, error) {
	// Read the retained change that produced head.
	if len(head) == 0 {
		return nil, nil
	}
	entry, err := host.ReadConfigEntry(ctx, head)
	if errors.Is(err, ErrConfigHistoryUnavailable) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Return the leave consent it carries.
	return SODepartingPeers(entry)
}

// isSODepartureTransfer reports whether entry is a transfer that carries a departure.
func isSODepartureTransfer(entry *SOConfigChange) bool {
	return entry.GetChangeType() == SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_TRANSFER_OWNERSHIP && entry.GetLeaveRequest() != nil
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
