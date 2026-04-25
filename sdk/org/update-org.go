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

// UpdateOrgOpID is the operation type ID.
var UpdateOrgOpID = "org/update-organization"

// NewUpdateOrgOpBlock creates an empty block for deserialization.
func NewUpdateOrgOpBlock() block.Block {
	return &UpdateOrgOp{}
}

// GetOperationTypeId returns the operation type ID.
func (o *UpdateOrgOp) GetOperationTypeId() string {
	return UpdateOrgOpID
}

// Validate validates the operation.
func (o *UpdateOrgOp) Validate() error {
	if o.GetOrgObjectKey() == "" {
		return errors.New("org_object_key is required")
	}
	return nil
}

// MarshalBlock marshals the operation to bytes.
func (o *UpdateOrgOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the operation from bytes.
func (o *UpdateOrgOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// ApplyWorldOp applies the update organization operation.
func (o *UpdateOrgOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	objKey := o.GetOrgObjectKey()

	objState, found, err := ws.GetObject(ctx, objKey)
	if err != nil {
		return true, err
	}
	if !found {
		return false, errors.New("organization object not found")
	}

	var state *OrgState
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uErr error
		state, uErr = UnmarshalOrgState(ctx, bcs)
		return uErr
	})
	if err != nil {
		return true, err
	}

	if state == nil {
		state = &OrgState{}
	}

	if err := applyUpdateOrg(state, o); err != nil {
		return false, err
	}

	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(state, true)
		return nil
	})
	if err != nil {
		return true, err
	}

	return false, nil
}

// ApplyWorldObjectOp is not supported for this operation.
func (o *UpdateOrgOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (bool, error) {
	return false, world.ErrUnhandledOp
}

// LookupUpdateOrgOp looks up the update organization operation.
func LookupUpdateOrgOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	if opTypeID == UpdateOrgOpID {
		return &UpdateOrgOp{}, nil
	}
	return nil, nil
}

// generateInvite creates an OrgInvite from a CreateOrgInviteOp.
func generateInvite(op *CreateOrgInviteOp) (*OrgInvite, error) {
	inviteType := op.GetType()
	if inviteType == OrgInviteType_ORG_INVITE_TYPE_UNKNOWN {
		return nil, errors.New("invite type is required")
	}

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

	// Generate invite ID from timestamp
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
	token := join.GetToken()
	accountID := join.GetAccountId()
	if token == "" {
		return errors.New("invite token is required")
	}
	if accountID == "" {
		return errors.New("account_id is required")
	}

	// Check if already a member.
	for _, m := range state.Members {
		if m.GetAccountId() == accountID {
			return errors.New("already a member")
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
		return errors.New("invalid invite token")
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
		return errors.New("invite has reached maximum uses")
	}

	// Increment uses.
	invite.Uses++

	// Add as member.
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

// _ is a type assertion
var _ world.Operation = (*UpdateOrgOp)(nil)
