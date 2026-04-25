package s4wave_org

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// ApplyOrgSOOp applies an org SO operation to the current state.
// stateData is the current marshaled OrgState (nil for uninitialized).
// opData is the marshaled OrgSOOp.
// Returns the next marshaled OrgState.
func ApplyOrgSOOp(ctx context.Context, stateData []byte, opData []byte, sender peer.ID) ([]byte, error) {
	op := &OrgSOOp{}
	if err := op.UnmarshalVT(opData); err != nil {
		return nil, errors.Wrap(err, "unmarshal org op")
	}

	state := &OrgState{}
	if len(stateData) > 0 {
		if err := state.UnmarshalVT(stateData); err != nil {
			return nil, errors.Wrap(err, "unmarshal org state")
		}
	}

	switch body := op.GetBody().(type) {
	case *OrgSOOp_InitOrg:
		if err := applyInitOrg(state, body.InitOrg); err != nil {
			return nil, err
		}
	case *OrgSOOp_UpdateOrg:
		if err := applyUpdateOrg(state, body.UpdateOrg); err != nil {
			return nil, err
		}
	case *OrgSOOp_DeleteOrg:
		if err := applyDeleteOrg(state, body.DeleteOrg); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unknown org op type")
	}

	return state.MarshalVT()
}

// applyInitOrg applies an InitOrganizationOp to the state.
func applyInitOrg(state *OrgState, op *InitOrganizationOp) error {
	state.DisplayName = op.GetDisplayName()
	state.CreatedAt = op.GetTimestamp()
	state.Members = []*OrgMemberInfo{{
		AccountId:   op.GetCreatorAccountId(),
		DisplayRole: OrgRoleOwner,
		JoinedAt:    op.GetTimestamp(),
	}}
	return nil
}

// applyUpdateOrg applies an UpdateOrgOp to the state.
func applyUpdateOrg(state *OrgState, op *UpdateOrgOp) error {
	switch body := op.GetBody().(type) {
	case *UpdateOrgOp_UpdateDisplayName:
		state.DisplayName = body.UpdateDisplayName.GetDisplayName()
	case *UpdateOrgOp_AddMember:
		member := body.AddMember.GetMember()
		if member == nil {
			return errors.New("member is required")
		}
		for _, m := range state.Members {
			if m.GetAccountId() == member.GetAccountId() {
				return errors.New("member already exists")
			}
		}
		state.Members = append(state.Members, member.CloneVT())
	case *UpdateOrgOp_RemoveMember:
		accountID := body.RemoveMember.GetAccountId()
		idx := slices.IndexFunc(state.Members, func(m *OrgMemberInfo) bool { return m.GetAccountId() == accountID })
		if idx < 0 {
			return errors.New("member not found")
		}
		state.Members = append(state.Members[:idx], state.Members[idx+1:]...)
	case *UpdateOrgOp_AddChildSo:
		soID := body.AddChildSo.GetSharedObjectId()
		for _, c := range state.ChildSharedObjects {
			if c.GetSharedObjectId() == soID {
				return errors.New("child SO already exists")
			}
		}
		state.ChildSharedObjects = append(state.ChildSharedObjects, &OrgChildRef{SharedObjectId: soID})
	case *UpdateOrgOp_RemoveChildSo:
		soID := body.RemoveChildSo.GetSharedObjectId()
		idx := slices.IndexFunc(state.ChildSharedObjects, func(c *OrgChildRef) bool { return c.GetSharedObjectId() == soID })
		if idx < 0 {
			return errors.New("child SO not found")
		}
		state.ChildSharedObjects = append(state.ChildSharedObjects[:idx], state.ChildSharedObjects[idx+1:]...)
	case *UpdateOrgOp_CreateInvite:
		invite, err := generateInvite(body.CreateInvite)
		if err != nil {
			return err
		}
		state.Invites = append(state.Invites, invite)
	case *UpdateOrgOp_RevokeInvite:
		invID := body.RevokeInvite.GetInviteId()
		idx := slices.IndexFunc(state.Invites, func(inv *OrgInvite) bool { return inv.GetId() == invID })
		if idx < 0 {
			return errors.New("invite not found")
		}
		state.Invites = append(state.Invites[:idx], state.Invites[idx+1:]...)
	case *UpdateOrgOp_JoinViaInvite:
		if err := applyJoinViaInvite(state, body.JoinViaInvite); err != nil {
			return err
		}
	default:
		return errors.New("unknown update body")
	}
	return nil
}

// applyDeleteOrg applies a DeleteOrganizationOp to the state.
func applyDeleteOrg(state *OrgState, _ *DeleteOrganizationOp) error {
	if len(state.GetChildSharedObjects()) > 0 {
		return errors.New("cannot delete organization with child shared objects")
	}
	// Reset the state to empty.
	state.Reset()
	return nil
}
