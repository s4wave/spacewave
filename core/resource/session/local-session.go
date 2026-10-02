package resource_session

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	auth_password "github.com/s4wave/spacewave/auth/method/password"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// LocalSessionResource implements LocalSessionResourceService.
type LocalSessionResource struct {
	b       bus.Bus
	session session.Session
}

// NewLocalSessionResource creates a new LocalSessionResource.
func NewLocalSessionResource(b bus.Bus, sess session.Session) *LocalSessionResource {
	return &LocalSessionResource{b: b, session: sess}
}

// resolveEntityKey resolves the entity private key from an EntityCredential.
// For password credentials, derives the key from the provider account ID + password.
// For PEM credentials, parses the raw PEM bytes.
func (r *LocalSessionResource) resolveEntityKey(cred *session.EntityCredential) (peer.ID, error) {
	// Resolve the entity peer ID from a password or a PEM private key.
	if cred == nil {
		return "", errors.New("credential is required")
	}
	password := cred.GetPassword()
	pemPrivateKey := cred.GetPemPrivateKey()
	if password != "" {
		accountID := r.session.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()
		_, entityPriv, err := auth_password.BuildParametersWithUsernamePassword(accountID, []byte(password))
		if err != nil {
			return "", errors.Wrap(err, "derive entity key")
		}
		entityPeerID, err := peer.IDFromPrivateKey(entityPriv)
		if err != nil {
			return "", errors.Wrap(err, "derive entity peer ID")
		}
		return entityPeerID, nil
	}
	if len(pemPrivateKey) > 0 {
		privKey, err := keypem.ParsePrivKeyPem(pemPrivateKey)
		if err != nil {
			return "", errors.Wrap(err, "parse PEM private key")
		}
		peerID, err := peer.IDFromPrivateKey(privKey)
		if err != nil {
			return "", errors.Wrap(err, "derive peer ID from PEM key")
		}
		return peerID, nil
	}
	return "", errors.New("password or pem_private_key is required")
}

// mountAccountSettingsSO mounts the AccountSettings SharedObject for the session.
func (r *LocalSessionResource) mountAccountSettingsSO(ctx context.Context, released func()) (sobject.SharedObject, func(), error) {
	// Mount account settings only for a local provider account.
	localAcc, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount)
	if !ok || localAcc == nil {
		return nil, nil, errors.New("local account settings require local provider account")
	}

	// Resolve the account-settings SharedObject reference.
	soRef, err := localAcc.GetAccountSettingsRef(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Mount that SharedObject and return its release function.
	so, mountRef, err := sobject.ExMountSharedObject(ctx, r.b, soRef, false, released)
	if err != nil {
		return nil, nil, err
	}
	return so, mountRef.Release, nil
}

// AddEntityKeypair derives an entity key from an EntityCredential and adds
// it to the AccountSettings SharedObject.
func (r *LocalSessionResource) AddEntityKeypair(
	ctx context.Context,
	req *s4wave_session.AddLocalEntityKeypairRequest,
) (*s4wave_session.AddLocalEntityKeypairResponse, error) {
	// Resolve the entity peer ID from the credential.
	entityPeerID, err := r.resolveEntityKey(req.GetCredential())
	if err != nil {
		return nil, err
	}

	// Record password or PEM as the auth method.
	authMethod := "password"
	if len(req.GetCredential().GetPemPrivateKey()) > 0 {
		authMethod = "pem"
	}

	// Marshal an add-entity-keypair operation.
	kp := &session.EntityKeypair{
		PeerId:     entityPeerID.String(),
		AuthMethod: authMethod,
	}
	addOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddEntityKeypair{
			AddEntityKeypair: kp,
		},
	}
	opData, err := addOp.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal operation")
	}

	// Mount the account-settings SharedObject.
	so, relSO, err := r.mountAccountSettingsSO(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount account settings")
	}
	defer relSO()

	// Queue the operation on that SharedObject.
	localID, err := so.QueueOperation(ctx, opData)
	if err != nil {
		return nil, errors.Wrap(err, "queue add entity keypair operation")
	}

	// Wait for the operation and clear a rejected result.
	_, wasRejected, err := so.WaitOperation(ctx, localID)
	if err != nil {
		if wasRejected {
			_ = so.ClearOperationResult(ctx, localID)
		}
		return nil, errors.Wrap(err, "add entity keypair")
	}

	return &s4wave_session.AddLocalEntityKeypairResponse{
		PeerId: entityPeerID.String(),
	}, nil
}

// RemoveEntityKeypair removes an entity keypair from the AccountSettings SharedObject.
func (r *LocalSessionResource) RemoveEntityKeypair(
	ctx context.Context,
	req *s4wave_session.RemoveLocalEntityKeypairRequest,
) (*s4wave_session.RemoveLocalEntityKeypairResponse, error) {
	// Require the peer ID to remove.
	peerID := req.GetPeerId()
	if peerID == "" {
		return nil, errors.New("peer_id is required")
	}

	// Marshal a remove-entity-keypair operation.
	rmOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_RemoveEntityKeypair{
			RemoveEntityKeypair: &account_settings.RemoveEntityKeypairOp{
				PeerId: peerID,
			},
		},
	}
	opData, err := rmOp.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal operation")
	}

	// Mount the account-settings SharedObject.
	so, relSO, err := r.mountAccountSettingsSO(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount account settings")
	}
	defer relSO()

	// Queue the operation on that SharedObject.
	localID, err := so.QueueOperation(ctx, opData)
	if err != nil {
		return nil, errors.Wrap(err, "queue remove entity keypair operation")
	}

	// Wait for the operation and clear a rejected result.
	_, wasRejected, err := so.WaitOperation(ctx, localID)
	if err != nil {
		if wasRejected {
			_ = so.ClearOperationResult(ctx, localID)
		}
		return nil, errors.Wrap(err, "remove entity keypair")
	}

	return &s4wave_session.RemoveLocalEntityKeypairResponse{}, nil
}

// SetDisplayName updates the local provider account display name.
func (r *LocalSessionResource) SetDisplayName(
	ctx context.Context,
	req *s4wave_session.SetLocalDisplayNameRequest,
) (*s4wave_session.SetLocalDisplayNameResponse, error) {
	// Collapse whitespace in the requested display name.
	displayName := strings.Join(strings.Fields(req.GetDisplayName()), " ")

	// Marshal an update-display-name operation.
	op := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_UpdateDisplayName{
			UpdateDisplayName: &account_settings.UpdateDisplayNameOp{
				DisplayName: displayName,
			},
		},
	}
	opData, err := op.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal operation")
	}

	// Mount the account-settings SharedObject.
	so, relSO, err := r.mountAccountSettingsSO(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount account settings")
	}
	defer relSO()

	// Queue the operation on that SharedObject.
	localID, err := so.QueueOperation(ctx, opData)
	if err != nil {
		return nil, errors.Wrap(err, "queue update display name operation")
	}

	// Wait for the operation and clear a rejected result.
	_, wasRejected, err := so.WaitOperation(ctx, localID)
	if err != nil {
		if wasRejected {
			_ = so.ClearOperationResult(ctx, localID)
		}
		return nil, errors.Wrap(err, "update display name")
	}

	// Copy the display name onto every session on this account.
	if err := r.syncSessionMetadata(ctx, displayName); err != nil {
		return nil, err
	}

	return &s4wave_session.SetLocalDisplayNameResponse{}, nil
}

// WatchDisplayName streams the local provider account display name.
func (r *LocalSessionResource) WatchDisplayName(
	req *s4wave_session.WatchLocalDisplayNameRequest,
	strm s4wave_session.SRPCLocalSessionResourceService_WatchDisplayNameStream,
) error {
	// Cancel the watch when the stream handler returns.
	ctx, ctxCancel := context.WithCancel(strm.Context())
	defer ctxCancel()

	// Mount the account-settings SharedObject.
	so, relSO, err := r.mountAccountSettingsSO(ctx, ctxCancel)
	if err != nil {
		return err
	}
	defer relSO()

	// Watch that SharedObject's state.
	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, ctxCancel)
	if err != nil {
		return err
	}
	defer relStateCtr()

	// Send the display name whenever account settings change.
	var prev *s4wave_session.WatchLocalDisplayNameResponse
	return ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			// Skip an empty account-settings snapshot.
			if snap == nil {
				return nil
			}

			// Decode account settings from the root inner.
			rootInner, err := snap.GetRootInner(ctx)
			if err != nil {
				return err
			}
			settings := &account_settings.AccountSettings{}
			if data := rootInner.GetStateData(); len(data) > 0 {
				if err := settings.UnmarshalVT(data); err != nil {
					return err
				}
			}

			// Send the display name when it changes.
			resp := &s4wave_session.WatchLocalDisplayNameResponse{
				DisplayName: settings.GetDisplayName(),
			}
			if prev != nil && resp.EqualVT(prev) {
				return nil
			}
			prev = resp
			return strm.Send(resp)
		},
		nil,
	)
}

// syncSessionMetadata updates session metadata for all sessions on the account.
func (r *LocalSessionResource) syncSessionMetadata(ctx context.Context, displayName string) error {
	// Look up the Session controller.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, r.b, "", false, nil)
	if err != nil {
		return err
	}
	defer sessionCtrlRef.Release()

	// List sessions on that controller.
	providerRef := r.session.GetSessionRef().GetProviderResourceRef()
	sessions, err := sessionCtrl.ListSessions(ctx)
	if err != nil {
		return err
	}

	// Update metadata for sessions on this provider account.
	for _, entry := range sessions {
		ref := entry.GetSessionRef().GetProviderResourceRef()
		if ref.GetProviderId() != providerRef.GetProviderId() ||
			ref.GetProviderAccountId() != providerRef.GetProviderAccountId() {
			continue
		}

		meta, err := sessionCtrl.GetSessionMetadata(ctx, entry.GetSessionIndex())
		if err != nil {
			return err
		}
		if meta == nil {
			meta = &session.SessionMetadata{}
		}
		meta.DisplayName = displayName
		meta.ProviderDisplayName = "Local"
		meta.ProviderId = providerRef.GetProviderId()
		meta.ProviderAccountId = providerRef.GetProviderAccountId()
		if err := sessionCtrl.UpdateSessionMetadata(ctx, entry.GetSessionRef(), meta); err != nil {
			return err
		}
	}

	return nil
}

// _ is a type assertion
var _ s4wave_session.SRPCLocalSessionResourceServiceServer = (*LocalSessionResource)(nil)
