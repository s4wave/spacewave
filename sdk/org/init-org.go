package s4wave_org

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// InitOrganizationOpID is the operation type ID.
var InitOrganizationOpID = "org/init-organization"

// OrgObjectKey is the default organization object key within the SO world.
const OrgObjectKey = "org/state"

// OrgRoleOwner is the display role for organization owners.
const OrgRoleOwner = "Owner"

// OrgRoleMember is the display role for organization members.
const OrgRoleMember = "Member"

// NewInitOrganizationOpBlock creates an empty block for deserialization.
func NewInitOrganizationOpBlock() block.Block {
	return &InitOrganizationOp{}
}

// GetOperationTypeId returns the operation type ID.
func (o *InitOrganizationOp) GetOperationTypeId() string {
	return InitOrganizationOpID
}

// Validate validates the operation.
func (o *InitOrganizationOp) Validate() error {
	return nil
}

// MarshalBlock marshals the operation to bytes.
func (o *InitOrganizationOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the operation from bytes.
func (o *InitOrganizationOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// ApplyWorldOp applies the init organization operation.
func (o *InitOrganizationOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	objKey := o.GetOrgObjectKey()
	if objKey == "" {
		objKey = OrgObjectKey
	}

	state := &OrgState{
		DisplayName: o.GetDisplayName(),
		CreatedAt:   o.GetTimestamp(),
		Members: []*OrgMemberInfo{{
			AccountId:   o.GetCreatorAccountId(),
			DisplayRole: OrgRoleOwner,
			JoinedAt:    o.GetTimestamp(),
		}},
	}
	if _, _, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(state, true)
		return nil
	}); err != nil {
		return true, err
	}
	if err := world_types.SetObjectType(ctx, ws, objKey, OrganizationTypeID); err != nil {
		return true, err
	}

	return false, nil
}

// ApplyWorldObjectOp is not supported for this operation.
func (o *InitOrganizationOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (bool, error) {
	return false, world.ErrUnhandledOp
}

// LookupInitOrganizationOp looks up the init organization operation.
func LookupInitOrganizationOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	if opTypeID == InitOrganizationOpID {
		return &InitOrganizationOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = (*InitOrganizationOp)(nil)
