package sobject

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// RemoveSOParticipants removes participant configs and grants in one signed
// configuration change. It returns the peer IDs that were present and removed.
// Under group control signerPriv, a voter, agrees to the removal and it
// returns those peers with ErrAwaitingGroup; the voters take the key from them
// once the group decides. A concurrent local change of the config does not
// fail the removal.
func RemoveSOParticipants(
	ctx context.Context,
	host *SOHost,
	targetPeerIDs []string,
	signerPriv crypto.PrivKey,
	revInfo *SORevocationInfo,
) ([]string, error) {
	return RemoveSOParticipantsThrough(ctx, host, targetPeerIDs, nil, signerPriv, revInfo)
}

// RemoveSOParticipantsThrough removes participants as RemoveSOParticipants
// does, admitting the operations of each peer that admitThrough names only up
// to the nonce it gives instead of up to the peer's latest held operation.
//
// It exists to recover a shared object whose author signed operations whose
// payloads no member holds any longer, such as a device whose storage was
// lost: replay stops at the first such operation forever, and every later
// operation waits behind it. Every member's replay rejects the cut operations
// as ReasonRemoved before it reads their payloads, so the object moves on
// without them.
//
// Invariants:
//   - Only operations of the named peers above their admitThrough nonce stop
//     applying; every other author's operations, and the named peers'
//     operations at or below the nonce, replay as before.
//   - A nonce above the peer's latest held operation is refused: the cut
//     would admit operations the members have not seen.
//   - A nonce below what the checkpoint covers is refused: the checkpoint's
//     World already holds those operations, and no removal can take them out.
//   - The pin names the exact operation at the nonce; an equivocated nonce is
//     refused, since it does not name one operation.
func RemoveSOParticipantsThrough(
	ctx context.Context,
	host *SOHost,
	targetPeerIDs []string,
	admitThrough map[string]uint64,
	signerPriv crypto.PrivKey,
	revInfo *SORevocationInfo,
) ([]string, error) {
	return retryConfigChange(ctx, host, func() ([]string, error) {
		return removeSOParticipants(ctx, host, targetPeerIDs, admitThrough, signerPriv, revInfo)
	})
}

// removeSOParticipants makes one attempt at RemoveSOParticipants from the
// config the host holds now.
func removeSOParticipants(
	ctx context.Context,
	host *SOHost,
	targetPeerIDs []string,
	admitThrough map[string]uint64,
	signerPriv crypto.PrivKey,
	revInfo *SORevocationInfo,
) ([]string, error) {
	// Collect the requested peers.
	targets := make(map[string]struct{}, len(targetPeerIDs))
	for _, peerID := range targetPeerIDs {
		if peerID != "" {
			targets[peerID] = struct{}{}
		}
	}
	if len(targets) == 0 {
		return nil, nil
	}

	// Read the current config.
	state, err := host.GetHostState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get current SO state")
	}
	currentCfg := state.GetConfig()
	if currentCfg == nil {
		return nil, nil
	}

	// Remove only peers that currently participate.
	var removed []string
	for _, participant := range currentCfg.GetParticipants() {
		if _, ok := targets[participant.GetPeerId()]; ok {
			removed = append(removed, participant.GetPeerId())
		}
	}
	present := make(map[string]struct{}, len(removed))
	for _, peerID := range removed {
		present[peerID] = struct{}{}
	}
	for peerID := range admitThrough {
		if _, ok := present[peerID]; !ok {
			return nil, errors.Errorf("cannot cut %s: it is not a participant", peerID)
		}
	}
	if len(removed) == 0 {
		return nil, nil
	}

	// Sign the removal, dropping the removed peers from the roster's drops, and
	// prune their grants with it.
	nextCfg := currentCfg.CloneVT()
	nextCfg.Participants = slices.DeleteFunc(nextCfg.Participants, func(participant *SOParticipantConfig) bool {
		_, ok := targets[participant.GetPeerId()]
		return ok
	})
	nextCfg.RosterDroppedPeerIds = slices.DeleteFunc(nextCfg.RosterDroppedPeerIds, func(peerID string) bool {
		_, ok := targets[peerID]
		return ok
	})
	if err := pinRemovedAuthors(host.GetSharedObjectID(), state, nextCfg, removed, admitThrough); err != nil {
		return nil, err
	}
	err = ChangeSOConfig(ctx, host, state, nextCfg, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, signerPriv, revInfo, func(state *SOState) error {
		return pruneRemovedParticipants(host.GetSharedObjectID(), state, targets, signerPriv)
	})
	if err != nil && !errors.Is(err, ErrAwaitingGroup) {
		return nil, err
	}
	return removed, err
}

// RemoveSOParticipant removes one participant config and grant.
func RemoveSOParticipant(
	ctx context.Context,
	host *SOHost,
	targetPeerID string,
	signerPriv crypto.PrivKey,
	revInfo *SORevocationInfo,
) (bool, error) {
	removed, err := RemoveSOParticipants(ctx, host, []string{targetPeerID}, signerPriv, revInfo)
	return len(removed) != 0, err
}

// pinRemovedAuthors records in next, the config that removes peers, the last
// operation state holds from each removed peer that its checkpoint does not
// cover, or the operation at the nonce admitThrough gives for the peer. It
// drops pins the checkpoint now covers and pins of peers next admits again,
// and keeps the list sorted by peer ID.
func pinRemovedAuthors(sharedObjectID string, state *SOState, next *SharedObjectConfig, peers []string, admitThrough map[string]uint64) error {
	// Read the operations held above the checkpoint.
	set, err := state.OperationSet(sharedObjectID)
	if err != nil {
		return err
	}

	// Keep each earlier pin a later checkpoint or rejoin has not made redundant.
	pins := make(map[string]*SOOperationPosition, len(next.GetRemovedAuthors())+len(peers))
	for _, pin := range next.GetRemovedAuthors() {
		participates := slices.ContainsFunc(next.GetParticipants(), func(p *SOParticipantConfig) bool { return p.GetPeerId() == pin.GetPeerId() })
		if !participates && !set.Covers(pin.GetPeerId(), pin.GetNonce()) {
			pins[pin.GetPeerId()] = pin
		}
	}

	// Pin each removed peer's latest held operation above the checkpoint.
	for _, peerID := range peers {
		nonce, head := set.AuthorHead(peerID)
		if through, ok := admitThrough[peerID]; ok {
			pin, err := cutPin(set, peerID, through, nonce)
			if err != nil {
				return err
			}
			delete(pins, peerID)
			if pin != nil {
				pins[peerID] = pin
			}
			continue
		}
		if nonce != 0 && !set.Covers(peerID, nonce) {
			pins[peerID] = &SOOperationPosition{PeerId: peerID, Nonce: nonce, OpHash: head}
		}
	}

	// Store them in peer ID order.
	next.RemovedAuthors = slices.SortedFunc(maps.Values(pins), func(a, b *SOOperationPosition) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	})
	return nil
}

// pruneRemovedParticipants removes the targets' grants from every key epoch
// and replaces their proofs that remaining participants depend on, signed by
// the remaining owner of signer. It revokes every invite that names a target,
// since a named peer may redeem its invite again. Their operations stay in the
// set, and replay applies only those the removal pinned.
func pruneRemovedParticipants(sharedObjectID string, state *SOState, targets map[string]struct{}, signer crypto.PrivKey) error {
	// Close the invites that would readmit a target.
	for _, invite := range state.GetInvites() {
		if _, ok := targets[invite.GetTargetPeerId()]; ok {
			invite.Revoked = true
		}
	}

	// Drop the targets' grants and re-wrap the grants they signed.
	signerID, err := peer.IDFromPrivateKey(signer)
	if err != nil {
		return err
	}
	current := state.CurrentKeyEpoch().GetEpoch()
	for _, epoch := range state.GetKeyEpochs() {
		epoch.Grants = slices.DeleteFunc(epoch.Grants, func(grant *SOGrant) bool {
			_, ok := targets[grant.GetPeerId()]
			return ok
		})
		if err := rewrapRemovedGrants(sharedObjectID, epoch, targets, signer, signerID.String(), epoch.GetEpoch() == current); err != nil {
			return errors.Wrapf(err, "key epoch %d", epoch.GetEpoch())
		}
	}

	// Co-sign the held checkpoint when no remaining owner signed it.
	checkpoint := state.GetCheckpoint()
	if checkpoint == nil {
		return nil
	}
	_, signers, err := checkpoint.Verify(sharedObjectID)
	if err != nil {
		return err
	}
	for i, signer := range slices.Backward(signers) {
		if _, removed := targets[signer]; removed {
			checkpoint.Signatures = slices.Delete(checkpoint.Signatures, i, i+1)
		}
	}
	if _, err := checkpoint.ValidateAuthority(sharedObjectID, state.GetConfig()); err == nil {
		return nil
	}
	return checkpoint.CoSign(signer)
}

// rewrapRemovedGrants re-encrypts, as signer, each grant in epoch that a
// target signed. Without its own grant to the epoch's key, signer drops those
// grants instead, unless the epoch is the current one.
func rewrapRemovedGrants(sharedObjectID string, epoch *SOKeyEpoch, targets map[string]struct{}, signer crypto.PrivKey, signerID string, required bool) error {
	var inner *SOGrantInner
	for i := 0; i < len(epoch.Grants); i++ {
		// Keep a grant whose signer remains.
		grant := epoch.Grants[i]
		grantSigner, err := grant.Verify(sharedObjectID)
		if err != nil {
			return err
		}
		if _, removed := targets[grantSigner]; !removed {
			continue
		}

		// Read the epoch key from signer's own grant.
		if inner == nil {
			own := epoch.FindGrant(signerID)
			if own == nil {
				if required {
					return errors.New("remaining owner grant required to replace removed signer grants")
				}
				epoch.Grants = slices.Delete(epoch.Grants, i, i+1)
				i--
				continue
			}
			if inner, err = own.DecryptInnerData(signer, sharedObjectID); err != nil {
				return err
			}
		}

		// Wrap it for the recipient.
		_, recipient, err := peer.ParsePeerIDWithPubKey(grant.GetPeerId())
		if err != nil {
			return err
		}
		if epoch.Grants[i], err = EncryptSOGrant(signer, recipient, sharedObjectID, inner); err != nil {
			return err
		}
	}
	return nil
}

// cutPin returns the pin that admits peerID's operations through nonce
// through, given head, the nonce of its latest held operation. It returns nil
// when the checkpoint covers through, since then no operation above the
// checkpoint is admitted.
func cutPin(set *SOOperationSet, peerID string, through, head uint64) (*SOOperationPosition, error) {
	// Refuse a cut above what the members hold or below the checkpoint.
	if through > head {
		return nil, errors.Errorf("admit-through nonce %d of %s is above its latest operation %d", through, peerID, head)
	}
	if set.Covers(peerID, through+1) {
		return nil, errors.Errorf("the checkpoint covers operations of %s above nonce %d", peerID, through)
	}
	if through == 0 || set.Covers(peerID, through) {
		return nil, nil
	}

	// Name the one operation at the nonce.
	hash, err := set.AuthorAt(peerID, through)
	if err != nil {
		return nil, err
	}
	return &SOOperationPosition{PeerId: peerID, Nonce: through, OpHash: hash}, nil
}

// ParticipantCut describes the operations a removal through a nonce stops
// applying for one peer.
type ParticipantCut struct {
	// PeerID is the removed author.
	PeerID string
	// AdmitThrough is the last nonce of the peer's operations that applies.
	AdmitThrough uint64
	// HeadNonce is the nonce of the peer's latest held operation.
	HeadNonce uint64
	// CheckpointNonce is the nonce through which the checkpoint covers the peer.
	CheckpointNonce uint64
	// CutOperations is the number of the peer's held operations above
	// AdmitThrough.
	CutOperations int
}

// PlanParticipantCuts checks each cut admitThrough names against state as
// RemoveSOParticipantsThrough would and returns them in peer ID order,
// without changing the config.
func PlanParticipantCuts(sharedObjectID string, state *SOState, admitThrough map[string]uint64) ([]ParticipantCut, error) {
	// Read the operations held above the checkpoint.
	set, err := state.OperationSet(sharedObjectID)
	if err != nil {
		return nil, err
	}

	// Check and describe each cut of a current participant.
	participants := make(map[string]struct{}, len(state.GetConfig().GetParticipants()))
	for _, p := range state.GetConfig().GetParticipants() {
		participants[p.GetPeerId()] = struct{}{}
	}
	cuts := make([]ParticipantCut, 0, len(admitThrough))
	for _, peerID := range slices.Sorted(maps.Keys(admitThrough)) {
		if _, ok := participants[peerID]; !ok {
			return nil, errors.Errorf("cannot cut %s: it is not a participant", peerID)
		}
		through := admitThrough[peerID]
		head, _ := set.AuthorHead(peerID)
		if _, err := cutPin(set, peerID, through, head); err != nil {
			return nil, err
		}
		cuts = append(cuts, ParticipantCut{
			PeerID:          peerID,
			AdmitThrough:    through,
			HeadNonce:       head,
			CheckpointNonce: set.CoveredNonce(peerID),
			CutOperations:   set.CountAbove(peerID, through),
		})
	}
	return cuts, nil
}
