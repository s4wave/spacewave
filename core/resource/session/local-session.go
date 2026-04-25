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
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
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

// ExportBackupKey generates a backup Ed25519 keypair, adds both the
// password-derived entity keypair and the backup keypair to the
// AccountSettings SharedObject, and returns the backup private key as PEM.
// The envelope watcher will automatically rewrap with the new keypairs.
func (r *LocalSessionResource) ExportBackupKey(
	ctx context.Context,
	req *s4wave_session.ExportBackupKeyRequest,
) (*s4wave_session.ExportBackupKeyResponse, error) {
	if req.GetPassword() == "" {
		return nil, errors.New("password is required")
	}

	// Add the password-derived entity keypair.
	passwordCred := &session.EntityCredential{
		Credential: &session.EntityCredential_Password{Password: req.GetPassword()},
	}
	_, err := r.AddEntityKeypair(ctx, &s4wave_session.AddLocalEntityKeypairRequest{
		Credential: passwordCred,
	})
	if err != nil {
		return nil, errors.Wrap(err, "add password entity keypair")
	}

	// Generate a new Ed25519 backup keypair.
	backupPriv, _, err := bifrost_crypto.GenerateEd25519Key(nil)
	if err != nil {
		return nil, errors.Wrap(err, "generate backup keypair")
	}

	backupPeerID, err := peer.IDFromPrivateKey(backupPriv)
	if err != nil {
		return nil, errors.Wrap(err, "derive backup peer ID")
	}

	// Add the backup keypair to AccountSettings.
	kp := &session.EntityKeypair{
		PeerId:     backupPeerID.String(),
		AuthMethod: "pem",
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

	so, relSO, err := r.mountAccountSettingsSO(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount account settings")
	}
	defer relSO()

	localID, err := so.QueueOperation(ctx, opData)
	if err != nil {
		return nil, errors.Wrap(err, "queue add backup keypair operation")
	}
	_, wasRejected, err := so.WaitOperation(ctx, localID)
	if err != nil {
		if wasRejected {
			_ = so.ClearOperationResult(ctx, localID)
		}
		return nil, errors.Wrap(err, "add backup keypair")
	}

	// Export the backup private key as PEM.
	pemData, err := keypem.MarshalPrivKeyPem(backupPriv)
	if err != nil {
		return nil, errors.Wrap(err, "marshal backup PEM")
	}

	return &s4wave_session.ExportBackupKeyResponse{
		PemData: pemData,
		PeerId:  backupPeerID.String(),
	}, nil
}

// resolveEntityKey resolves the entity private key from an EntityCredential.
// For password credentials, derives the key from the provider account ID + password.
// For PEM credentials, parses the raw PEM bytes.
func (r *LocalSessionResource) resolveEntityKey(cred *session.EntityCredential) (peer.ID, error) {
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
	localAcc, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount)
	if !ok || localAcc == nil {
		return nil, nil, errors.New("local account settings require local provider account")
	}

	soRef, err := localAcc.GetAccountSettingsRef(ctx)
	if err != nil {
		return nil, nil, err
	}

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
	entityPeerID, err := r.resolveEntityKey(req.GetCredential())
	if err != nil {
		return nil, err
	}

	authMethod := "password"
	if len(req.GetCredential().GetPemPrivateKey()) > 0 {
		authMethod = "pem"
	}

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

	so, relSO, err := r.mountAccountSettingsSO(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount account settings")
	}
	defer relSO()

	localID, err := so.QueueOperation(ctx, opData)
	if err != nil {
		return nil, errors.Wrap(err, "queue add entity keypair operation")
	}

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
	peerID := req.GetPeerId()
	if peerID == "" {
		return nil, errors.New("peer_id is required")
	}

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

	so, relSO, err := r.mountAccountSettingsSO(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount account settings")
	}
	defer relSO()

	localID, err := so.QueueOperation(ctx, opData)
	if err != nil {
		return nil, errors.Wrap(err, "queue remove entity keypair operation")
	}

	_, wasRejected, err := so.WaitOperation(ctx, localID)
	if err != nil {
		if wasRejected {
			_ = so.ClearOperationResult(ctx, localID)
		}
		return nil, errors.Wrap(err, "remove entity keypair")
	}

	return &s4wave_session.RemoveLocalEntityKeypairResponse{}, nil
}

// WatchEntityKeypairs streams entity keypairs from the AccountSettings SO.
func (r *LocalSessionResource) WatchEntityKeypairs(
	req *s4wave_session.WatchLocalEntityKeypairsRequest,
	strm s4wave_session.SRPCLocalSessionResourceService_WatchEntityKeypairsStream,
) error {
	ctx, ctxCancel := context.WithCancel(strm.Context())
	defer ctxCancel()

	so, relSO, err := r.mountAccountSettingsSO(ctx, ctxCancel)
	if err != nil {
		return err
	}
	defer relSO()

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, ctxCancel)
	if err != nil {
		return err
	}
	defer relStateCtr()

	var prev *s4wave_session.WatchLocalEntityKeypairsResponse
	return ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			if snap == nil {
				return nil
			}
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
			resp := &s4wave_session.WatchLocalEntityKeypairsResponse{
				Keypairs: settings.GetEntityKeypairs(),
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

// SetDisplayName updates the local provider account display name.
func (r *LocalSessionResource) SetDisplayName(
	ctx context.Context,
	req *s4wave_session.SetLocalDisplayNameRequest,
) (*s4wave_session.SetLocalDisplayNameResponse, error) {
	displayName := strings.Join(strings.Fields(req.GetDisplayName()), " ")

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

	so, relSO, err := r.mountAccountSettingsSO(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount account settings")
	}
	defer relSO()

	localID, err := so.QueueOperation(ctx, opData)
	if err != nil {
		return nil, errors.Wrap(err, "queue update display name operation")
	}

	_, wasRejected, err := so.WaitOperation(ctx, localID)
	if err != nil {
		if wasRejected {
			_ = so.ClearOperationResult(ctx, localID)
		}
		return nil, errors.Wrap(err, "update display name")
	}

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
	ctx, ctxCancel := context.WithCancel(strm.Context())
	defer ctxCancel()

	so, relSO, err := r.mountAccountSettingsSO(ctx, ctxCancel)
	if err != nil {
		return err
	}
	defer relSO()

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, ctxCancel)
	if err != nil {
		return err
	}
	defer relStateCtr()

	var prev *s4wave_session.WatchLocalDisplayNameResponse
	return ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			if snap == nil {
				return nil
			}
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
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, r.b, "", false, nil)
	if err != nil {
		return err
	}
	defer sessionCtrlRef.Release()

	providerRef := r.session.GetSessionRef().GetProviderResourceRef()
	sessions, err := sessionCtrl.ListSessions(ctx)
	if err != nil {
		return err
	}

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
var _ s4wave_session.SRPCLocalSessionResourceServiceServer = ((*LocalSessionResource)(nil))
