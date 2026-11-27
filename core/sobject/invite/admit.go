package sobject_invite

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// Redeemer is the verified identity behind a redemption: the session peer that
// redeemed the invite and the storage peer that writes for it, which may be the
// same peer.
type Redeemer struct {
	// PeerID is the session peer that signed the join response.
	PeerID peer.ID
	// PubKey is the session peer's public key.
	PubKey crypto.PubKey
	// StoragePeerID is the peer that writes the Space for the session.
	StoragePeerID peer.ID
	// StoragePubKey is the storage peer's public key.
	StoragePubKey crypto.PubKey
}

// ValidateRedeemer verifies a redemption's join responses, which must both
// identify the same invite.
func ValidateRedeemer(joinResp, storageJoinResp *sobject.SOJoinResponse) (*Redeemer, error) {
	// Verify the session peer's signature.
	peerID, pubKey, err := ValidateJoinResponse(joinResp)
	if err != nil {
		return nil, err
	}

	// Verify the storage peer's signature over the same invite.
	if storageJoinResp == nil {
		return nil, errors.New("storage_join_response is required")
	}
	storagePeerID, storagePubKey, err := ValidateJoinResponse(storageJoinResp)
	if err != nil {
		return nil, errors.Wrap(err, "validate storage join response")
	}
	if storageJoinResp.GetInviteId() != joinResp.GetInviteId() {
		return nil, errors.New("storage join response invite ID mismatch")
	}
	return &Redeemer{
		PeerID:        peerID,
		PubKey:        pubKey,
		StoragePeerID: storagePeerID,
		StoragePubKey: storagePubKey,
	}, nil
}

// Admit enrolls a redeemer of result's invite at the invite's role and counts
// the use. It returns the session peer's grant, nil when that peer already
// held one, the outcome and the role the peer holds after enrollment.
//
// Admission is the redemption's effect, whether the invite admitted the peer
// at once or an owner approved its queued request. The caller decides that the
// redeemer may join. Admit checks nothing about the invite.
func Admit(
	ctx context.Context,
	enroll EnrollFn,
	result *InviteLookupResult,
	r *Redeemer,
) (*sobject.SOGrant, sobject.SOParticipantAddOutcome, sobject.SOParticipantRole, error) {
	// Enroll the session peer before counting the use, so a failed enrollment
	// does not burn a use of a limited invite.
	grant, outcome, role, err := enroll(ctx, result, r.PeerID, r.PubKey)
	if err != nil {
		return nil, 0, 0, errors.Wrap(err, "enroll participant")
	}

	// The storage peer writes for the session, which never builds on the
	// checkpointer's edits, so the session leaves the trimming roster.
	if r.StoragePeerID != r.PeerID {
		if _, _, _, err := enroll(ctx, result, r.StoragePeerID, r.StoragePubKey); err != nil {
			return nil, 0, 0, errors.Wrap(err, "enroll storage participant")
		}
		if _, err := sobject.DropFromSORoster(ctx, result.Host, []string{r.PeerID.String()}, result.OwnerPrivKey); err != nil {
			return nil, 0, 0, errors.Wrap(err, "drop session from trimming roster")
		}
	}

	// Enrollment succeeded, so the redemption consumes a use.
	mutator := result.InviteMutator
	if mutator == nil {
		mutator = result.Host
	}
	if err := mutator.IncrementInviteUses(ctx, result.OwnerPrivKey, result.Invite.GetInviteId()); err != nil {
		return nil, 0, 0, errors.Wrap(err, "increment invite uses")
	}
	return grant, outcome, role, nil
}
