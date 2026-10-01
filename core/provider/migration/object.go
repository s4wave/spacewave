// Package provider_migration composes authorized account resource movement.
package provider_migration

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// CheckObject requires the current participant to authorize new account peers.
// Existing third-party permissions remain part of the verified configuration.
func CheckObject(ctx context.Context, object sobject.SharedObject, peers []string) error {
	// Read the object's host state.
	host, ok := object.(sobject.InviteHost)
	if !ok {
		return errors.New("SharedObject cannot authorize account migration")
	}
	state, err := host.GetSOHost().GetHostState(ctx)
	if err != nil {
		return err
	}

	// Track the incoming peers not already participants.
	remaining := make(map[string]bool, len(peers))
	for _, id := range peers {
		if _, _, err := peer.ParsePeerIDWithPubKey(id); err != nil {
			return err
		}
		remaining[id] = true
	}

	// Remove existing participants and detect the local owner role.
	owner := false
	for _, participant := range state.GetConfig().GetParticipants() {
		delete(remaining, participant.GetPeerId())
		if participant.GetPeerId() == object.GetPeerID().String() {
			owner = participant.GetRole() == sobject.SOParticipantRole_SOParticipantRole_OWNER
		}
	}

	// Accept the object when every incoming peer is already a participant.
	if len(remaining) == 0 {
		return nil
	}
	if !owner {
		return errors.New("the Space owner must authorize the destination account before this Space can move")
	}
	if len(state.GetConfig().GetParticipants())+len(remaining) > sobject.MaxParticipants {
		return errors.New("the destination account exceeds this Space's participant limit")
	}
	return nil
}

// AuthorizeObject extends the existing signed lineage without replacing roles,
// grants, roots, or external participants. Existing peers keep their exact role.
func AuthorizeObject(ctx context.Context, object sobject.SharedObject, peers []string, entityID, username string) (*sobject.SOState, error) {
	if err := CheckObject(ctx, object, peers); err != nil {
		return nil, err
	}
	host := object.(sobject.InviteHost)
	for _, id := range peers {
		state, err := host.GetSOHost().GetHostState(ctx)
		if err != nil {
			return nil, err
		}
		present := false
		for _, participant := range state.GetConfig().GetParticipants() {
			if participant.GetPeerId() == id {
				present = true
				break
			}
		}
		if present {
			continue
		}
		_, public, err := peer.ParsePeerIDWithPubKey(id)
		if err != nil {
			return nil, err
		}
		if remote, ok := object.(interface {
			AddParticipant(context.Context, string, crypto.PubKey, sobject.SOParticipantRole, string, string) (*sobject.SOGrant, error)
		}); ok {
			_, err = remote.AddParticipant(ctx, id, public, sobject.SOParticipantRole_SOParticipantRole_OWNER, entityID, username)
		} else {
			_, err = sobject.AddSOParticipant(ctx, host.GetSOHost(), object.GetSharedObjectID(), host.GetPrivKey(), object.GetPeerID().String(), id, public, sobject.SOParticipantRole_SOParticipantRole_OWNER, entityID, username)
		}
		if err != nil {
			return nil, err
		}
	}
	return host.GetSOHost().GetHostState(ctx)
}
