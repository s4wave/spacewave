package sobject

import (
	"bytes"
	"context"
	"crypto/sha256"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// BuildSOLeaveRequest signs consent to remove each supplied identity at one held configuration.
func BuildSOLeaveRequest(sharedObjectID string, configHash []byte, keys ...crypto.PrivKey) (*SOLeaveRequest, error) {
	// Every signature binds its own removal to this object and admission generation.
	request := &SOLeaveRequest{SharedObjectId: sharedObjectID, ConfigHash: bytes.Clone(configHash)}
	data, err := request.MarshalVT()
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		signature, err := peer.NewSignature("sobject leave", key, hash.RecommendedHashType, data, true)
		if err != nil {
			return nil, err
		}
		request.Signatures = append(request.Signatures, signature)
	}

	// Apply the same bounds and duplicate checks as the receiving owner.
	if _, err := request.Verify(); err != nil {
		return nil, err
	}
	return request, nil
}

// Verify authenticates departing identities without conferring authority over another participant.
func (r *SOLeaveRequest) Verify() ([]string, error) {
	// Bound proof work before parsing keys or verifying signatures.
	if r.GetSharedObjectId() == "" || len(r.GetSharedObjectId()) > 256 {
		return nil, errors.New("leave requires a bounded shared object ID")
	}
	if len(r.GetConfigHash()) != 32 || len(r.GetSignatures()) == 0 || len(r.GetSignatures()) > MaxParticipants {
		return nil, errors.New("leave requires a configuration hash and bounded identity proofs")
	}
	unsigned := r.CloneVT()
	unsigned.Signatures = nil
	data, err := unsigned.MarshalVT()
	if err != nil {
		return nil, err
	}

	// Each independently verified key authorizes only its own removal.
	var peers []string
	for _, signature := range r.GetSignatures() {
		key, err := signature.ParsePubKey()
		if err != nil {
			return nil, err
		}
		if key == nil {
			return nil, peer.ErrEmptyPeerID
		}
		id, err := peer.IDFromPublicKey(key)
		if err != nil {
			return nil, err
		}
		valid, err := signature.VerifyWithPublic("sobject leave", key, data)
		if err != nil {
			return nil, err
		}
		if !valid || slices.Contains(peers, id.String()) {
			return nil, errors.New("invalid or duplicate leave proof")
		}
		peers = append(peers, id.String())
	}
	return peers, nil
}

// LeaveSOParticipants commits a proven voluntary removal under the owner's existing signing authority.
// Retries return only the original removal proof; an old request cannot remove a rejoined participant.
func LeaveSOParticipants(ctx context.Context, host *SOHost, owner crypto.PrivKey, request *SOLeaveRequest) (*SOLeaveResponse, error) {
	// Consent is bound to the selected host before its state can be read or changed.
	peers, err := request.Verify()
	if err != nil {
		return nil, err
	}
	if request.GetSharedObjectId() != host.GetSharedObjectID() {
		return nil, errors.New("leave request addresses a different shared object")
	}
	data, err := request.MarshalVT()
	if err != nil {
		return nil, err
	}
	requestHash := sha256.Sum256(data)

	for {
		state, err := host.GetHostState(ctx)
		if err != nil {
			return nil, err
		}
		current := state.GetConfig()
		var changes []*SOConfigChange

		// A completed request remains retryable without revealing subsequent membership changes.
		if !bytes.Equal(current.GetConfigChainHash(), request.GetConfigHash()) {
			changes, err = host.ReadConfigHistory(ctx, request.GetConfigHash(), current.GetConfigChainHash())
			if err != nil {
				return nil, err
			}
			for i, change := range changes {
				if bytes.Equal(change.GetRevocationInfo().GetLeaveRequestHash(), requestHash[:]) {
					return &SOLeaveResponse{Changes: changes[:i+1]}, nil
				}
			}
			signed, err := host.ReadConfigEntry(ctx, request.GetConfigHash())
			if err != nil {
				return nil, err
			}
			if !leaveProofsRemainCurrent(peers, signed.GetConfig(), changes) {
				return nil, errors.New("leave configuration changed before removal")
			}
		}

		// Preserve every other participant and remove the proven grants in the same commit.
		next := current.CloneVT()
		next.Participants = slices.DeleteFunc(next.Participants, func(p *SOParticipantConfig) bool { return slices.Contains(peers, p.GetPeerId()) })
		if len(next.Participants) == len(current.GetParticipants()) {
			return nil, errors.New("leave proofs name no current participant")
		}
		if len(next.Participants) != 0 && slices.ContainsFunc(current.GetParticipants(), func(p *SOParticipantConfig) bool {
			return IsOwner(p.GetRole()) && slices.Contains(peers, p.GetPeerId())
		}) {
			return nil, errors.New("remaining root grants require ownership transfer before owner departure")
		}
		change, err := BuildSOConfigChange(current, next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, owner, &SORevocationInfo{LeaveRequestHash: requestHash[:]})
		if err != nil {
			return nil, err
		}
		err = host.ApplyConfigChange(ctx, change, func(state *SOState) error {
			state.RootGrants = slices.DeleteFunc(state.RootGrants, func(grant *SOGrant) bool { return slices.Contains(peers, grant.GetPeerId()) })
			return nil
		})
		if err == nil {
			return &SOLeaveResponse{Changes: append(changes, change)}, nil
		}

		// A concurrent owner change invalidates only this optimistic attempt. Re-read
		// retained history and apply the same consent if its identities stayed admitted.
		latest, latestErr := host.GetHostState(ctx)
		if latestErr != nil || bytes.Equal(latest.GetConfig().GetConfigChainHash(), current.GetConfigChainHash()) {
			return nil, err
		}
	}
}

// leaveProofsRemainCurrent permits rebasing consent only when every signer was
// admitted at the signed head and remained admitted through each later transition.
// A signer absent from any of those configurations must consent again.
func leaveProofsRemainCurrent(peers []string, signed *SharedObjectConfig, changes []*SOConfigChange) bool {
	if len(changes) == 0 {
		return false
	}
	configs := []*SharedObjectConfig{signed}
	for _, change := range changes {
		configs = append(configs, change.GetConfig())
	}
	for _, config := range configs {
		for _, peerID := range peers {
			if !slices.ContainsFunc(config.GetParticipants(), func(p *SOParticipantConfig) bool {
				return p.GetPeerId() == peerID
			}) {
				return false
			}
		}
	}
	return true
}
