package provider_spacewave

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
)

// LeaveSharedObject removes this account from a cloud shared object. The
// calling session must be an owner: it removes the account's other sessions,
// then transfers ownership to successor, or to the default successor when
// empty, in a change carrying its own leave consent. It returns once the
// transfer commits; the successor's session commits the departure.
//
// The successor must belong to another account. With no other account the
// object would stay billed to an account that can no longer open it, so the
// leave is refused and the object should be deleted instead.
func (a *ProviderAccount) LeaveSharedObject(ctx context.Context, id, successor string) error {
	// Mount the object for the duration of the departure.
	mounted, release, err := a.MountSharedObject(ctx, a.buildSharedObjectRef(id), nil)
	if err != nil {
		return err
	}
	defer release()
	so, ok := mounted.(*SharedObject)
	if !ok {
		return errors.New("leave requires a cloud shared object")
	}
	return so.leaveAsOwner(ctx, successor)
}

// leaveAsOwner removes the local account's other peers, then transfers
// ownership away from the local peer with its leave consent.
func (s *SharedObject) leaveAsOwner(ctx context.Context, successor string) error {
	// Read the latest configuration.
	if _, _, _, err := s.loadLatestConfigState(ctx); err != nil {
		return err
	}
	host := s.GetSOHost()
	state, err := host.GetHostState(ctx)
	if err != nil {
		return err
	}

	// Split the participants into the local peer, its account's other peers and
	// the peers of other accounts.
	self := s.localPid.String()
	var owner, others bool
	var siblings []string
	for _, p := range state.GetConfig().GetParticipants() {
		switch {
		case p.GetPeerId() == self:
			owner = sobject.IsOwner(p.GetRole())
		case p.GetEntityId() == s.host.selfEntityID:
			siblings = append(siblings, p.GetPeerId())
		default:
			others = true
		}
	}

	// Only an owner can hand the object to another account.
	if !owner {
		return errors.New("cloud departure requires the owner role")
	}
	if !others {
		return errors.New("no other account remains in the space: delete it instead")
	}
	if successor == self || slices.Contains(siblings, successor) {
		return errors.New("successor must belong to another account")
	}

	// The account leaves with every session, not only this device.
	if err := s.retryConfigConflicts(ctx, func() error {
		_, err := sobject.RemoveSOParticipants(ctx, host, siblings, s.privKey, nil)
		return err
	}); err != nil {
		return errors.Wrap(err, "remove account sessions")
	}

	// Consent binds the configuration head, so each attempt signs it anew.
	return s.retryConfigConflicts(ctx, func() error {
		// Sign consent at the current head.
		state, err := host.GetHostState(ctx)
		if err != nil {
			return err
		}
		request, err := sobject.BuildSOLeaveRequest(s.GetSharedObjectID(), state.GetConfig().GetConfigChainHash(), s.privKey)
		if err != nil {
			return err
		}

		// Promote the successor with the consent.
		_, err = sobject.TransferSOOwnership(ctx, host, s.privKey, successor, request)
		return err
	})
}
