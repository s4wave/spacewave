package sobject

import (
	"context"
	"crypto/rand"
	"slices"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/zeebo/blake3"
)

// CreateSOInviteOp creates an invite on the shared object and returns the
// signed SOInviteMessage for out-of-band distribution.
//
// Generates a random 32-byte token, BLAKE3 hashes it for on-chain storage,
// builds and signs the SOInviteMessage, then stores the invite metadata in
// SOState.invites via a signed config chain entry.
func BuildSOInviteMessage(
	sharedObjectID string,
	ownerPrivKey crypto.PrivKey,
	role SOParticipantRole,
	providerID string,
	targetPeerID string,
	maxUses uint32,
	expiresAt *timestamppb.Timestamp,
) (*SOInviteMessage, *SOInvite, error) {
	ownerPeerID, err := peer.IDFromPrivateKey(ownerPrivKey)
	if err != nil {
		return nil, nil, errors.Wrap(err, "derive owner peer ID")
	}

	inviteID := ulid.NewULID()

	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return nil, nil, errors.Wrap(err, "generate invite token")
	}

	tokenHashArr := blake3.Sum256(token)
	tokenHash := tokenHashArr[:]

	msg := &SOInviteMessage{
		InviteId:       inviteID,
		SharedObjectId: sharedObjectID,
		OwnerPeerId:    ownerPeerID.String(),
		ProviderId:     providerID,
		Token:          token,
		Role:           role,
		TargetPeerId:   targetPeerID,
		ExpiresAt:      expiresAt,
		MaxUses:        maxUses,
	}

	data, err := msg.MarshalVT()
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal invite message for signing")
	}
	sig, err := peer.NewSignature(
		"sobject invite",
		ownerPrivKey,
		hash.HashType_HashType_BLAKE3,
		data,
		true,
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "sign invite message")
	}
	msg.Signature = sig

	return msg, &SOInvite{
		InviteId:     inviteID,
		TokenHash:    tokenHash,
		Role:         role,
		TargetPeerId: targetPeerID,
		MaxUses:      maxUses,
		ExpiresAt:    expiresAt,
	}, nil
}

// CreateSOInviteOp creates an invite on the shared object and returns the
// signed SOInviteMessage for out-of-band distribution.
//
// Generates a random 32-byte token, BLAKE3 hashes it for on-chain storage,
// builds and signs the SOInviteMessage, then stores the invite metadata in
// SOState.invites via a signed config chain entry.
func (s *SOHost) CreateSOInviteOp(
	ctx context.Context,
	ownerPrivKey crypto.PrivKey,
	role SOParticipantRole,
	providerID string,
	targetPeerID string,
	maxUses uint32,
	expiresAt *timestamppb.Timestamp,
) (*SOInviteMessage, error) {
	msg, invite, err := BuildSOInviteMessage(
		s.sharedObjectID,
		ownerPrivKey,
		role,
		providerID,
		targetPeerID,
		maxUses,
		expiresAt,
	)
	if err != nil {
		return nil, err
	}

	if err := s.CreateInvite(ctx, ownerPrivKey, invite); err != nil {
		return nil, errors.Wrap(err, "store invite on-chain")
	}

	return msg, nil
}

// CreateInvite creates a new invite on the shared object via a signed config
// chain entry. The invite is appended to SOState.invites. The config itself
// does not change; the chain entry records the authorized operation.
func (s *SOHost) CreateInvite(
	ctx context.Context,
	signerPrivKey crypto.PrivKey,
	invite *SOInvite,
) error {
	if invite == nil {
		return errors.New("invite is nil")
	}
	if invite.GetInviteId() == "" {
		return errors.New("invite_id is required")
	}
	if len(invite.GetTokenHash()) == 0 {
		return errors.New("token_hash is required")
	}

	currentState, err := s.GetHostState(ctx)
	if err != nil {
		return errors.Wrap(err, "get current state")
	}

	// Check for duplicate invite_id.
	for _, existing := range currentState.GetInvites() {
		if existing.GetInviteId() == invite.GetInviteId() {
			return errors.New("invite_id already exists")
		}
	}

	currentCfg := currentState.GetConfig()
	entry, err := BuildSOConfigChange(currentCfg, currentCfg, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, signerPrivKey, nil)
	if err != nil {
		return errors.Wrap(err, "build config change")
	}

	return s.ApplyConfigChange(ctx, entry, func(state *SOState) error {
		state.Invites = append(state.Invites, invite.CloneVT())
		return nil
	})
}

// RevokeInvite revokes an existing invite on the shared object via a signed
// config chain entry. Sets revoked=true on the matching invite.
func (s *SOHost) RevokeInvite(
	ctx context.Context,
	signerPrivKey crypto.PrivKey,
	inviteID string,
) error {
	if inviteID == "" {
		return errors.New("invite_id is required")
	}

	currentState, err := s.GetHostState(ctx)
	if err != nil {
		return errors.Wrap(err, "get current state")
	}

	// Verify the invite exists and is not already revoked.
	found := false
	for _, inv := range currentState.GetInvites() {
		if inv.GetInviteId() == inviteID {
			if inv.GetRevoked() {
				return errors.New("invite is already revoked")
			}
			found = true
			break
		}
	}
	if !found {
		return errors.New("invite not found")
	}

	currentCfg := currentState.GetConfig()
	entry, err := BuildSOConfigChange(currentCfg, currentCfg, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REVOKE_INVITE, signerPrivKey, nil)
	if err != nil {
		return errors.Wrap(err, "build config change")
	}

	return s.ApplyConfigChange(ctx, entry, func(state *SOState) error {
		for _, inv := range state.GetInvites() {
			if inv.GetInviteId() == inviteID {
				inv.Revoked = true
				return nil
			}
		}
		return errors.New("invite not found in state")
	})
}

// IncrementInviteUses increments the uses counter on an invite via a signed
// config chain entry. Returns an error if the invite is invalid, revoked,
// expired, or has reached max_uses.
func (s *SOHost) IncrementInviteUses(
	ctx context.Context,
	signerPrivKey crypto.PrivKey,
	inviteID string,
) error {
	if inviteID == "" {
		return errors.New("invite_id is required")
	}

	currentState, err := s.GetHostState(ctx)
	if err != nil {
		return errors.Wrap(err, "get current state")
	}

	// Validate the invite.
	var target *SOInvite
	for _, inv := range currentState.GetInvites() {
		if inv.GetInviteId() == inviteID {
			target = inv
			break
		}
	}
	if target == nil {
		return errors.New("invite not found")
	}
	if err := ValidateInviteUsable(target); err != nil {
		return err
	}

	currentCfg := currentState.GetConfig()
	entry, err := BuildSOConfigChange(currentCfg, currentCfg, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_INCREMENT_INVITE_USES, signerPrivKey, nil)
	if err != nil {
		return errors.Wrap(err, "build config change")
	}

	return s.ApplyConfigChange(ctx, entry, func(state *SOState) error {
		for _, inv := range state.GetInvites() {
			if inv.GetInviteId() == inviteID {
				inv.Uses++
				return nil
			}
		}
		return errors.New("invite not found in state")
	})
}

// ValidateInviteUsable checks whether an invite is currently usable.
// Returns nil if the invite can accept another use.
func ValidateInviteUsable(inv *SOInvite) error {
	if inv.GetRevoked() {
		return errors.New("invite is revoked")
	}
	if exp := inv.GetExpiresAt(); exp != nil && time.Now().After(exp.AsTime()) {
		return errors.New("invite has expired")
	}
	if inv.GetMaxUses() != 0 && inv.GetUses() >= inv.GetMaxUses() {
		return errors.New("invite has reached max uses")
	}
	return nil
}

// FindInvite returns the invite with the given ID from the state, or nil.
func FindInvite(state *SOState, inviteID string) *SOInvite {
	idx := slices.IndexFunc(state.GetInvites(), func(inv *SOInvite) bool {
		return inv.GetInviteId() == inviteID
	})
	if idx == -1 {
		return nil
	}
	return state.GetInvites()[idx]
}
