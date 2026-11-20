package sobject

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"slices"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// BuildSOInviteMessage builds and signs the SOInviteMessage for out-of-band
// distribution along with the on-chain SOInvite metadata.
//
// terms supplies the role, targets, use limit, expiry and admission conditions.
// Generates a random 32-byte token and SHA256 hashes it for on-chain storage.
// The returned SOInvite is stored separately via a signed config chain entry.
func BuildSOInviteMessage(
	sharedObjectID string,
	ownerPrivKey crypto.PrivKey,
	providerID string,
	terms *SOInvite,
) (*SOInviteMessage, *SOInvite, error) {
	// Derive the owner peer that signs the invite.
	ownerPeerID, err := peer.IDFromPrivateKey(ownerPrivKey)
	if err != nil {
		return nil, nil, errors.Wrap(err, "derive owner peer ID")
	}

	// Bind a fresh token to a new invite.
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return nil, nil, errors.Wrap(err, "generate invite token")
	}
	tokenHash := sha256.Sum256(token)
	invite := terms.CloneVT()
	invite.InviteId = ulid.NewULID()
	invite.TokenHash = tokenHash[:]
	invite.Uses = 0
	invite.Revoked = false

	// Sign the message that carries the token out of band.
	msg := &SOInviteMessage{
		InviteId:       invite.GetInviteId(),
		SharedObjectId: sharedObjectID,
		OwnerPeerId:    ownerPeerID.String(),
		ProviderId:     providerID,
		Token:          token,
		Role:           invite.GetRole(),
		TargetPeerId:   invite.GetTargetPeerId(),
		ExpiresAt:      invite.GetExpiresAt(),
		MaxUses:        invite.GetMaxUses(),
	}
	if err := msg.Sign(ownerPrivKey); err != nil {
		return nil, nil, err
	}
	return msg, invite, nil
}

// CreateSOInviteOp creates an invite on the shared object and returns the
// signed SOInviteMessage for out-of-band distribution.
//
// Builds the invite from terms with BuildSOInviteMessage, then stores its
// metadata in SOState.invites via a signed config chain entry.
func (s *SOHost) CreateSOInviteOp(
	ctx context.Context,
	ownerPrivKey crypto.PrivKey,
	providerID string,
	terms *SOInvite,
) (*SOInviteMessage, error) {
	msg, invite, err := BuildSOInviteMessage(s.sharedObjectID, ownerPrivKey, providerID, terms)
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
// Invite identity is checked against the checkpoint held under the provider lock.
func (s *SOHost) CreateInvite(
	ctx context.Context,
	signerPrivKey crypto.PrivKey,
	invite *SOInvite,
) error {
	// Check the invite is well formed.
	if invite == nil {
		return errors.New("invite is nil")
	}
	if invite.GetInviteId() == "" {
		return errors.New("invite_id is required")
	}
	if len(invite.GetTokenHash()) == 0 {
		return errors.New("token_hash is required")
	}
	if invite.GetMaxUses() != 0 && invite.GetUses() > invite.GetMaxUses() {
		return errors.New("invite uses exceeds max uses")
	}

	// Apply the state change with the signed change that authorizes it.
	return s.applyInviteChange(ctx, signerPrivKey, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, func(state *SOState) error {
		if FindInvite(state, invite.GetInviteId()) != nil {
			return errors.New("invite_id already exists")
		}
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
	// Name the invite.
	if inviteID == "" {
		return errors.New("invite_id is required")
	}

	// Apply the state change with the signed change that authorizes it.
	return s.applyInviteChange(ctx, signerPrivKey, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REVOKE_INVITE, func(state *SOState) error {
		// Revoke a live invite.
		inv := FindInvite(state, inviteID)
		if inv == nil {
			return errors.New("invite not found in state")
		}
		if inv.GetRevoked() {
			return errors.New("invite is already revoked")
		}
		inv.Revoked = true
		return nil
	})
}

// IncrementInviteUses increments the uses counter on an invite via a signed
// config chain entry. Returns an error if the invite is invalid, revoked,
// expired, or has reached max_uses.
// Usability is checked under the provider lock before incrementing the counter.
func (s *SOHost) IncrementInviteUses(
	ctx context.Context,
	signerPrivKey crypto.PrivKey,
	inviteID string,
) error {
	// Name the invite.
	if inviteID == "" {
		return errors.New("invite_id is required")
	}

	// Apply the state change with the signed change that authorizes it.
	return s.applyInviteChange(ctx, signerPrivKey, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_INCREMENT_INVITE_USES, func(state *SOState) error {
		// Count a use of a usable invite.
		inv := FindInvite(state, inviteID)
		if inv == nil {
			return errors.New("invite not found in state")
		}
		if err := ValidateInviteUsable(inv); err != nil {
			return err
		}
		inv.Uses++
		return nil
	})
}

// ValidateInviteUsable checks whether an invite is currently usable.
// Returns nil if the invite can accept another use, which is also what
// makes it open to a new peer.
func ValidateInviteUsable(inv *SOInvite) error {
	if err := validateInviteLive(inv); err != nil {
		return err
	}
	if inv.GetMaxUses() != 0 && inv.GetUses() >= inv.GetMaxUses() {
		return errors.New("invite has reached max uses")
	}
	return nil
}

// ValidateInviteRedeemable checks whether peerID may redeem an invite now: a
// targeted invite admits only the peer it names, and every invite must be live
// and below its max_uses. A peer InviteEnrolled reports skips this check.
func ValidateInviteRedeemable(inv *SOInvite, peerID string) error {
	if target := inv.GetTargetPeerId(); target != "" && peerID != target {
		return errors.New("invite is targeted to a different peer")
	}
	return ValidateInviteUsable(inv)
}

// InviteEnrolled reports whether peerID already holds the invite's role or a
// higher one in cfg.
//
// Redeeming the invite again changes nothing for such a peer, so its owner
// answers with the peer's grant whatever has become of the invite since: the
// repeat consumes no use and passes revocation, expiry and max_uses. A Device
// that lost its answer to a power cut or a cancellation redeems again this way.
func InviteEnrolled(cfg *SharedObjectConfig, inv *SOInvite, peerID string) bool {
	held := cfg.GetParticipantRole(peerID)
	return held != SOParticipantRole_SOParticipantRole_UNKNOWN && held >= inv.GetRole()
}

// validateInviteLive checks that an invite is neither revoked nor expired.
func validateInviteLive(inv *SOInvite) error {
	if inv.GetRevoked() {
		return errors.New("invite is revoked")
	}
	if exp := inv.GetExpiresAt(); exp != nil && time.Now().After(exp.AsTime()) {
		return errors.New("invite has expired")
	}
	return nil
}

// InviteRedemption is how a host treats the redemption of a usable invite.
type InviteRedemption int

const (
	// InviteRedemptionRefuse admits nobody and queues nothing.
	InviteRedemptionRefuse InviteRedemption = iota
	// InviteRedemptionAdmit adds the redeemer as a participant.
	InviteRedemptionAdmit
	// InviteRedemptionQueue holds the redemption as a join request.
	InviteRedemptionQueue
)

// RedeemInvite decides the redemption of a usable invite by peerID.
//
// An invite without conditions admits everyone. A participant_of invite admits
// a peer that participates in one of the named Spaces in held, the host's own
// configuration of each Space it holds. An approval_required invite queues
// every redemption it does not admit; otherwise the redemption is refused.
func RedeemInvite(inv *SOInvite, peerID string, held map[string]*SharedObjectConfig) InviteRedemption {
	// An invite without conditions admits everyone.
	participation := inv.GetParticipantOf()
	if participation == nil && !inv.GetApprovalRequired() {
		return InviteRedemptionAdmit
	}

	// A participant of a named held Space is admitted.
	if slices.ContainsFunc(participation.GetSharedObjectIds(), func(id string) bool {
		return slices.ContainsFunc(held[id].GetParticipants(), func(p *SOParticipantConfig) bool {
			return p.GetPeerId() == peerID
		})
	}) {
		return InviteRedemptionAdmit
	}

	// Approval queues everyone else; without it they are refused.
	if inv.GetApprovalRequired() {
		return InviteRedemptionQueue
	}
	return InviteRedemptionRefuse
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

// applyInviteChange signs a change of changeType that keeps the config held at
// the head and applies fn with it, again when a concurrent writer moves the
// head first.
func (s *SOHost) applyInviteChange(ctx context.Context, signerPrivKey crypto.PrivKey, changeType SOConfigChangeType, fn func(state *SOState) error) error {
	_, err := retryConfigChange(ctx, s, func() (struct{}, error) {
		// Sign the change against the current config.
		currentState, err := s.GetHostState(ctx)
		if err != nil {
			return struct{}{}, errors.Wrap(err, "get current state")
		}
		currentCfg := currentState.GetConfig()
		entry, err := BuildSOConfigChange(s.sharedObjectID, currentCfg, currentCfg, changeType, signerPrivKey, nil)
		if err != nil {
			return struct{}{}, errors.Wrap(err, "build config change")
		}

		// Apply it with the state change it authorizes.
		return struct{}{}, s.ApplyConfigChange(ctx, entry, fn)
	})
	return err
}
