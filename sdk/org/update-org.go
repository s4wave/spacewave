package s4wave_org

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Validate validates the operation.
func (o *UpdateOrgOp) Validate() error {
	if o.GetOrgObjectKey() == "" {
		return errors.New("org_object_key is required")
	}
	return nil
}

// ApplyWorldOp applies the update organization operation.
func (o *UpdateOrgOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	// Open the requested organization object for update and release its handle afterward.
	objKey := o.GetOrgObjectKey()
	objState, found, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return true, err
	}
	if !found {
		return false, world.NewOpRefusedError(errors.New("organization object not found"))
	}

	// Read the organization body before applying its update.
	var state *OrgState
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uErr error
		state, uErr = UnmarshalOrgState(ctx, bcs)
		return uErr
	})
	if err != nil {
		return true, err
	}

	// Supply an empty organization body when the object has no state.
	if state == nil {
		state = &OrgState{}
	}

	// Apply the requested change to the organization body.
	if err := applyUpdateOrg(state, o); err != nil {
		return false, err
	}

	// Save the updated organization body in the World object.
	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(state, true)
		return nil
	})
	if err != nil {
		return true, err
	}

	return false, nil
}

// generateInvite creates an OrgInvite from a CreateOrgInviteOp.
func generateInvite(op *CreateOrgInviteOp) (*OrgInvite, error) {
	// Require an invitation type before generating its token.
	inviteType := op.GetType()
	if inviteType == OrgInviteType_ORG_INVITE_TYPE_UNKNOWN {
		return nil, world.NewOpRefusedError(errors.New("invite type is required"))
	}

	// Generate a random token with the length required by the invitation type.
	var token string
	switch inviteType {
	case OrgInviteType_ORG_INVITE_TYPE_CODE:
		// 8-char alphanumeric code
		b := make([]byte, 4)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		token = hex.EncodeToString(b)
	case OrgInviteType_ORG_INVITE_TYPE_LINK, OrgInviteType_ORG_INVITE_TYPE_EMAIL:
		// 32-char hex token
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		token = hex.EncodeToString(b)
	}

	// Generate the invite ID from the timestamp.
	id := strconv.FormatInt(time.Now().UnixNano(), 36)

	return &OrgInvite{
		Id:        id,
		Type:      inviteType,
		Token:     token,
		Config:    op.GetConfig(),
		MaxUses:   op.GetMaxUses(),
		ExpiresAt: op.GetExpiresAt(),
	}, nil
}

// applyJoinViaInvite validates the invite token and adds the joiner as a member.
func applyJoinViaInvite(state *OrgState, join *JoinOrgViaInviteOp) error {
	// Require the invitation token and the account joining the organization.
	token := join.GetToken()
	accountID := join.GetAccountId()
	if token == "" {
		return world.NewOpRefusedError(errors.New("invite token is required"))
	}
	if accountID == "" {
		return world.NewOpRefusedError(errors.New("account_id is required"))
	}

	// Check if the account is already a member.
	for _, m := range state.Members {
		if m.GetAccountId() == accountID {
			return world.NewOpRefusedError(errors.New("already a member"))
		}
	}

	// Find and validate the invite.
	var invite *OrgInvite
	for _, inv := range state.Invites {
		if inv.GetToken() == token {
			invite = inv
			break
		}
	}
	if invite == nil {
		return world.NewOpRefusedError(errors.New("invalid invite token"))
	}

	// Check expiry.
	if invite.GetExpiresAt() != nil {
		expiresAt := invite.GetExpiresAt().AsTime()
		if !expiresAt.IsZero() && time.Now().After(expiresAt) {
			return errors.New("invite has expired")
		}
	}

	// Check max uses.
	if invite.GetMaxUses() > 0 && invite.GetUses() >= invite.GetMaxUses() {
		return world.NewOpRefusedError(errors.New("invite has reached maximum uses"))
	}

	// Increment uses.
	invite.Uses++

	// Add the account as a member.
	joinedAt := join.GetTimestamp()
	if joinedAt == nil {
		joinedAt = timestamppb.Now()
	}
	state.Members = append(state.Members, &OrgMemberInfo{
		AccountId:   accountID,
		DisplayRole: OrgRoleMember,
		JoinedAt:    joinedAt,
	})

	return nil
}

var _ world.Operation = (*UpdateOrgOp)(nil)
