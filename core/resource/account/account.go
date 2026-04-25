package resource_account

import (
	"context"
	"crypto/rand"
	"net/http"
	"path"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	auth_password "github.com/s4wave/spacewave/auth/method/password"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
)

// AccountResource wraps a provider account for resource access.
type AccountResource struct {
	mux          srpc.Invoker
	account      *provider_spacewave.ProviderAccount
	localAccount *provider_local.ProviderAccount
	stepUpRef    *refcount.Ref[struct{}]
}

// NewAccountResource creates a new AccountResource.
func NewAccountResource(acc provider.ProviderAccount) *AccountResource {
	r := &AccountResource{}
	switch a := acc.(type) {
	case *provider_spacewave.ProviderAccount:
		r.account = a
		r.stepUpRef = a.RetainEntityKeypairStepUp()
	case *provider_local.ProviderAccount:
		r.localAccount = a
	default:
		return nil
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_account.SRPCRegisterAccountResourceService(mux, r)
	})
	return r
}

// Release releases any account-resource-scoped step-up retention.
func (r *AccountResource) Release() {
	if r.stepUpRef != nil {
		r.stepUpRef.Release()
		r.stepUpRef = nil
	}
}

// GetMux returns the rpc mux.
func (r *AccountResource) GetMux() srpc.Invoker {
	return r.mux
}

// WatchAccountInfo streams information about this account.
func (r *AccountResource) WatchAccountInfo(
	req *s4wave_account.WatchAccountInfoRequest,
	strm s4wave_account.SRPCAccountResourceService_WatchAccountInfoStream,
) error {
	if r.localAccount != nil {
		return r.watchLocalAccountInfo(strm)
	}
	return r.watchCloudAccountInfo(strm)
}

func (r *AccountResource) watchLocalAccountInfo(
	strm s4wave_account.SRPCAccountResourceService_WatchAccountInfoStream,
) error {
	ctx, ctxCancel := context.WithCancel(strm.Context())
	defer ctxCancel()

	soRef, err := r.localAccount.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
	}
	so, relSO, err := r.localAccount.MountSharedObject(ctx, soRef, ctxCancel)
	if err != nil {
		return err
	}
	defer relSO()

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, ctxCancel)
	if err != nil {
		return err
	}
	defer relStateCtr()

	var prev *s4wave_account.WatchAccountInfoResponse
	return ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			settings := &account_settings.AccountSettings{}
			if snap != nil {
				rootInner, err := snap.GetRootInner(ctx)
				if err != nil {
					return err
				}
				if data := rootInner.GetStateData(); len(data) > 0 {
					if err := settings.UnmarshalVT(data); err != nil {
						return err
					}
				}
			}
			resp := &s4wave_account.WatchAccountInfoResponse{
				AccountId:    r.localAccount.GetAccountID(),
				EntityId:     settings.GetDisplayName(),
				ProviderId:   r.localAccount.GetProviderID(),
				KeypairCount: uint32(len(settings.GetEntityKeypairs())),
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

func (r *AccountResource) watchCloudAccountInfo(
	strm s4wave_account.SRPCAccountResourceService_WatchAccountInfoStream,
) error {
	ctx := strm.Context()
	bcast := r.account.GetAccountBroadcast()
	var prev *s4wave_account.WatchAccountInfoResponse
	for {
		// Snapshot state and wait channel atomically inside one HoldLock.
		var info *api.AccountStateResponse
		var ch <-chan struct{}
		bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			info = r.account.AccountStateSnapshot()
			ch = getWaitCh()
		})

		if info != nil {
			resp := &s4wave_account.WatchAccountInfoResponse{
				AccountId:     info.AccountId,
				EntityId:      info.EntityId,
				ProviderId:    r.account.GetProviderID(),
				AuthThreshold: info.AuthThreshold,
				KeypairCount:  info.KeypairCount,
			}
			if prev == nil || !resp.EqualVT(prev) {
				if err := strm.Send(resp); err != nil {
					return err
				}
				prev = resp
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// WatchAuthMethods streams the account auth-method rows for this account.
func (r *AccountResource) WatchAuthMethods(
	req *s4wave_account.WatchAuthMethodsRequest,
	strm s4wave_account.SRPCAccountResourceService_WatchAuthMethodsStream,
) error {
	ctx := strm.Context()
	bcast := r.account.GetAccountBroadcast()
	var prev *s4wave_account.WatchAuthMethodsResponse
	for {
		// Snapshot state and wait channel atomically inside one HoldLock.
		var authMethods []*api.AccountAuthMethod
		var valid bool
		var ch <-chan struct{}
		bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			authMethods = r.account.AuthMethodsSnapshot()
			valid = r.account.AccountStateSnapshot() != nil
			ch = getWaitCh()
		})

		if valid {
			resp := &s4wave_account.WatchAuthMethodsResponse{
				AuthMethods: authMethods,
			}
			if prev == nil || !resp.EqualVT(prev) {
				if err := strm.Send(resp); err != nil {
					return err
				}
				prev = resp
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// WatchSessions streams the attached sessions snapshot for this account.
func (r *AccountResource) WatchSessions(
	req *s4wave_account.WatchSessionsRequest,
	strm s4wave_account.SRPCAccountResourceService_WatchSessionsStream,
) error {
	if r.localAccount != nil {
		return r.watchLocalSessions(strm)
	}
	return r.watchCloudSessions(strm)
}

func (r *AccountResource) watchLocalSessions(
	strm s4wave_account.SRPCAccountResourceService_WatchSessionsStream,
) error {
	ctx, ctxCancel := context.WithCancel(strm.Context())
	defer ctxCancel()

	soRef, err := r.localAccount.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
	}
	so, relSO, err := r.localAccount.MountSharedObject(ctx, soRef, ctxCancel)
	if err != nil {
		return err
	}
	defer relSO()

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, ctxCancel)
	if err != nil {
		return err
	}
	defer relStateCtr()

	var prev *s4wave_account.WatchSessionsResponse
	return ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			currentPeerID := r.localAccount.GetMountedSessionPeerID(ctx).String()
			settings := &account_settings.AccountSettings{}
			var devices []*account_settings.PairedDevice
			if snap != nil {
				rootInner, err := snap.GetRootInner(ctx)
				if err != nil {
					return err
				}
				if data := rootInner.GetStateData(); len(data) > 0 {
					if err := settings.UnmarshalVT(data); err != nil {
						return err
					}
				}
				devices = settings.GetPairedDevices()
			}
			presentations := buildSessionPresentationMap(settings)

			sessions := make([]*s4wave_account.AccountSession, 0, len(devices)+1)
			if currentPeerID != "" {
				row := &s4wave_account.AccountSession{
					PeerId:         currentPeerID,
					CurrentSession: true,
					Kind: s4wave_account.
						AccountSessionKind_AccountSessionKind_ACCOUNT_SESSION_KIND_LOCAL_SESSION,
					Label: settings.GetDisplayName(),
				}
				if row.GetLabel() == "" {
					row.Label = "This device"
				}
				applySessionPresentation(row, presentations[currentPeerID])
				sessions = append(sessions, row)
			}
			for _, device := range devices {
				peerID := device.GetPeerId()
				if peerID == "" || peerID == currentPeerID {
					continue
				}
				row := &s4wave_account.AccountSession{
					PeerId: peerID,
					Kind: s4wave_account.
						AccountSessionKind_AccountSessionKind_ACCOUNT_SESSION_KIND_LOCAL_SESSION,
					Label: device.GetDisplayName(),
				}
				if row.GetLabel() == "" {
					row.Label = peerID
				}
				if pairedAt := device.GetPairedAt(); pairedAt > 0 {
					row.CreatedAt = timestamppb.New(time.Unix(pairedAt, 0))
				}
				applySessionPresentation(row, presentations[peerID])
				sessions = append(sessions, row)
			}

			resp := &s4wave_account.WatchSessionsResponse{Sessions: sessions}
			if prev != nil && resp.EqualVT(prev) {
				return nil
			}
			prev = resp
			return strm.Send(resp)
		},
		nil,
	)
}

func (r *AccountResource) watchCloudSessions(
	strm s4wave_account.SRPCAccountResourceService_WatchSessionsStream,
) error {
	ctx, ctxCancel := context.WithCancel(strm.Context())
	defer ctxCancel()

	var (
		stateCtr ccontainer.Watchable[sobject.SharedObjectStateSnapshot]
		relSO    func()
		relCtr   func()
	)
	if ref, err := r.account.GetAccountSettingsRef(ctx); err == nil && ref != nil {
		if so, releaseSO, err := r.account.MountSharedObject(ctx, ref, ctxCancel); err == nil {
			if ctr, releaseCtr, err := so.AccessSharedObjectState(ctx, ctxCancel); err == nil {
				stateCtr = ctr
				relSO = releaseSO
				relCtr = releaseCtr
			} else {
				releaseSO()
			}
		}
	}
	if relCtr != nil {
		defer relCtr()
	}
	if relSO != nil {
		defer relSO()
	}

	bcast := r.account.GetAccountBroadcast()
	var prev *s4wave_account.WatchSessionsResponse
	for {
		var (
			rows  []*api.AccountSessionInfo
			valid bool
			ch    <-chan struct{}
		)
		bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			rows, valid = r.account.SessionsSnapshot()
			ch = getWaitCh()
		})

		if valid {
			metadata := buildSessionPresentationMapFromSnapshot(stateCtr)
			currentPeerID := r.account.GetCurrentSessionPeerID().String()
			resp := &s4wave_account.WatchSessionsResponse{
				Sessions: buildCloudSessionRows(currentPeerID, rows, metadata),
			}
			if prev == nil || !resp.EqualVT(prev) {
				if err := strm.Send(resp); err != nil {
					return err
				}
				prev = resp
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

func buildCloudSessionRows(
	currentPeerID string,
	rows []*api.AccountSessionInfo,
	metadata map[string]*account_settings.SessionPresentation,
) []*s4wave_account.AccountSession {
	out := make([]*s4wave_account.AccountSession, 0, len(rows))
	for _, row := range rows {
		if row == nil || row.GetPeerId() == "" {
			continue
		}
		item := &s4wave_account.AccountSession{
			PeerId:         row.GetPeerId(),
			CurrentSession: row.GetPeerId() == currentPeerID,
			Kind: s4wave_account.
				AccountSessionKind_AccountSessionKind_ACCOUNT_SESSION_KIND_CLOUD_AUTH_SESSION,
			Label: row.GetPeerId(),
		}
		if row.GetCreatedAt() > 0 {
			item.CreatedAt = timestamppb.New(time.UnixMilli(row.GetCreatedAt()))
		}
		if row.GetLastSeen() > 0 {
			item.LastSeenAt = timestamppb.New(time.UnixMilli(row.GetLastSeen()))
		}
		if row.GetDeviceInfo() != "" {
			item.Os = row.GetDeviceInfo()
		}
		applySessionPresentation(item, metadata[row.GetPeerId()])
		out = append(out, item)
	}
	return out
}

func buildSessionPresentationMapFromSnapshot(
	stateCtr ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
) map[string]*account_settings.SessionPresentation {
	if stateCtr == nil {
		return nil
	}
	snap := stateCtr.GetValue()
	if snap == nil {
		return nil
	}
	rootInner, err := snap.GetRootInner(context.Background())
	if err != nil {
		return nil
	}
	settings := &account_settings.AccountSettings{}
	if data := rootInner.GetStateData(); len(data) > 0 {
		if err := settings.UnmarshalVT(data); err != nil {
			return nil
		}
	}
	return buildSessionPresentationMap(settings)
}

func buildSessionPresentationMap(
	settings *account_settings.AccountSettings,
) map[string]*account_settings.SessionPresentation {
	if settings == nil || len(settings.GetSessionPresentations()) == 0 {
		return nil
	}
	out := make(map[string]*account_settings.SessionPresentation, len(settings.GetSessionPresentations()))
	for _, pres := range settings.GetSessionPresentations() {
		if pres == nil || pres.GetPeerId() == "" {
			continue
		}
		out[pres.GetPeerId()] = pres
	}
	return out
}

func applySessionPresentation(
	row *s4wave_account.AccountSession,
	pres *account_settings.SessionPresentation,
) {
	if row == nil || pres == nil {
		return
	}
	if pres.GetLabel() != "" {
		row.Label = pres.GetLabel()
	}
	if pres.GetDeviceType() != "" {
		row.DeviceType = pres.GetDeviceType()
	}
	if pres.GetClientName() != "" {
		row.ClientName = pres.GetClientName()
	}
	if pres.GetOs() != "" {
		row.Os = pres.GetOs()
	}
	if pres.GetLocation() != "" {
		row.Location = pres.GetLocation()
	}
}

// ResolveEntityKey resolves the entity private key from an EntityCredential.
func (r *AccountResource) ResolveEntityKey(ctx context.Context, cred *session.EntityCredential) (bifrost_crypto.PrivKey, peer.ID, error) {
	if cred == nil {
		return nil, "", errors.New("credential is required")
	}
	password := cred.GetPassword()
	pemPrivateKey := cred.GetPemPrivateKey()
	if password != "" {
		info, err := r.account.GetAccountState(ctx)
		if err != nil {
			return nil, "", errors.Wrap(err, "fetch account info")
		}
		_, entityPriv, err := auth_password.BuildParametersWithUsernamePassword(info.EntityId, []byte(password))
		if err != nil {
			return nil, "", errors.Wrap(err, "derive entity key")
		}
		entityPeerID, err := peer.IDFromPrivateKey(entityPriv)
		if err != nil {
			return nil, "", errors.Wrap(err, "derive entity peer ID")
		}
		return entityPriv, entityPeerID, nil
	}
	if len(pemPrivateKey) > 0 {
		privKey, err := keypem.ParsePrivKeyPem(pemPrivateKey)
		if err != nil {
			return nil, "", errors.Wrap(err, "parse PEM private key")
		}
		peerID, err := peer.IDFromPrivateKey(privKey)
		if err != nil {
			return nil, "", errors.Wrap(err, "derive peer ID from PEM key")
		}
		return privKey, peerID, nil
	}
	return nil, "", errors.New("password or pem_private_key is required")
}

// buildMultiSigEnvelope builds the typed MultiSigActionEnvelope bytes that
// entity keys sign. Binding account_id, kind, method, and path to the signed
// bytes prevents replay across accounts or endpoints.
func (r *AccountResource) buildMultiSigEnvelope(kind api.MultiSigActionKind, method, reqPath string, actionBody []byte) ([]byte, error) {
	env := &api.MultiSigActionEnvelope{
		AccountId: r.account.GetAccountID(),
		Kind:      kind,
		Method:    method,
		Path:      reqPath,
		Payload:   actionBody,
	}
	envBytes, err := env.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal multi-sig envelope")
	}
	return envBytes, nil
}

// signAndSubmit builds a multi-sig envelope, signs it with the entity key,
// and submits to the cloud. Returns the parsed MultiSigActionResponse envelope.
func (r *AccountResource) signAndSubmit(
	ctx context.Context,
	method, reqPath string,
	kind api.MultiSigActionKind,
	actionBody []byte,
	entityPriv bifrost_crypto.PrivKey,
	entityPeerID peer.ID,
) (*api.MultiSigActionResponse, error) {
	envelope, err := r.buildMultiSigEnvelope(kind, method, reqPath, actionBody)
	if err != nil {
		return nil, err
	}
	now := timestamppb.New(time.Now().Truncate(time.Millisecond))
	payload := provider_spacewave.BuildMultiSigPayload(now, envelope)
	sig, err := entityPriv.Sign(payload)
	if err != nil {
		return nil, errors.Wrap(err, "sign envelope")
	}
	return r.sendMultiSig(ctx, method, reqPath, envelope, []*api.EntitySignature{{
		PeerId:    entityPeerID.String(),
		Signature: sig,
		SignedAt:  now,
	}})
}

// sendMultiSig wraps envelope bytes and signatures into a MultiSigRequest and
// sends it to the given cloud path without session-key auth headers. Returns
// the parsed MultiSigActionResponse envelope.
func (r *AccountResource) sendMultiSig(
	ctx context.Context,
	method, reqPath string,
	envelope []byte,
	sigs []*api.EntitySignature,
) (*api.MultiSigActionResponse, error) {
	cli := r.account.GetSessionClient()
	msReq := &api.MultiSigRequest{
		Envelope:   envelope,
		Signatures: sigs,
	}
	body, err := marshalMultiSigRequest(msReq)
	if err != nil {
		return nil, err
	}
	return cli.DoMultiSig(ctx, method, reqPath, body)
}

// marshalMultiSigRequest encodes a MultiSigRequest using protobuf binary.
func marshalMultiSigRequest(msReq *api.MultiSigRequest) ([]byte, error) {
	body, err := msReq.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal multi-sig request")
	}
	return body, nil
}

// AddAuthMethod adds a new entity keypair (auth method) to the account.
func (r *AccountResource) AddAuthMethod(
	ctx context.Context,
	req *s4wave_account.AddAuthMethodRequest,
) (*s4wave_account.AddAuthMethodResponse, error) {
	kp := req.GetKeypair()
	if kp == nil {
		return nil, errors.New("keypair is required")
	}
	actionBody, err := (&api.AddKeypairAction{Keypair: kp}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal add keypair action")
	}
	accountID := r.account.GetAccountID()
	reqPath := accountAPIPath(accountID, "keypair", "add")
	envelope, sigs, err := r.resolveOrSignWithTracker(
		ctx,
		req.GetCredential(),
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_ADD_KEYPAIR,
		http.MethodPost,
		reqPath,
		actionBody,
	)
	if err != nil {
		return nil, err
	}
	if _, err := r.sendMultiSig(ctx, http.MethodPost, reqPath, envelope, sigs); err != nil {
		return nil, errors.Wrap(err, "add auth method")
	}
	r.account.BumpLocalEpoch()

	return &s4wave_account.AddAuthMethodResponse{}, nil
}

// RemoveAuthMethod removes an entity keypair from the account.
func (r *AccountResource) RemoveAuthMethod(
	ctx context.Context,
	req *s4wave_account.RemoveAuthMethodRequest,
) (*s4wave_account.RemoveAuthMethodResponse, error) {
	accountID := r.account.GetAccountID()
	reqPath := accountAPIPath(accountID, "keypair", "remove")

	actionBody, err := (&api.RemoveKeypairAction{PeerId: req.GetPeerId()}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal remove keypair action")
	}
	envelope, sigs, err := r.resolveOrSignWithTracker(
		ctx,
		req.GetCredential(),
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_REMOVE_KEYPAIR,
		http.MethodPost,
		reqPath,
		actionBody,
	)
	if err != nil {
		return nil, err
	}
	if _, err := r.sendMultiSig(ctx, http.MethodPost, reqPath, envelope, sigs); err != nil {
		return nil, errors.Wrap(err, "remove auth method")
	}
	r.account.BumpLocalEpoch()

	return &s4wave_account.RemoveAuthMethodResponse{}, nil
}

// SetSecurityLevel updates the auth threshold for the account.
func (r *AccountResource) SetSecurityLevel(
	ctx context.Context,
	req *s4wave_account.SetSecurityLevelRequest,
) (*s4wave_account.SetSecurityLevelResponse, error) {
	accountID := r.account.GetAccountID()
	reqPath := accountAPIPath(accountID, "threshold")

	actionBody, err := (&api.UpdateThresholdAction{Threshold: req.GetThreshold()}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal update threshold action")
	}
	envelope, sigs, err := r.resolveOrSignWithTracker(
		ctx,
		req.GetCredential(),
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_UPDATE_THRESHOLD,
		http.MethodPost,
		reqPath,
		actionBody,
	)
	if err != nil {
		return nil, err
	}
	if _, err := r.sendMultiSig(ctx, http.MethodPost, reqPath, envelope, sigs); err != nil {
		return nil, errors.Wrap(err, "set security level")
	}
	r.account.BumpLocalEpoch()
	return &s4wave_account.SetSecurityLevelResponse{}, nil
}

// RevokeSession revokes a session by peer ID.
//
// When credential is nil and the requested session_peer_id matches the
// current session, uses the session self-revoke endpoint (no entity key
// needed). Otherwise falls through to the entity multi-sig path, with
// tracker fallback.
func (r *AccountResource) RevokeSession(
	ctx context.Context,
	req *s4wave_account.RevokeSessionRequest,
) (*s4wave_account.RevokeSessionResponse, error) {
	cli := r.account.GetSessionClient()

	// Self-revoke path: no credential provided, current session.
	if req.GetCredential() == nil {
		currentPeerID := cli.GetPeerID().String()
		if req.GetSessionPeerId() == currentPeerID {
			if err := cli.SelfRevoke(ctx); err != nil {
				return nil, errors.Wrap(err, "self-revoke session")
			}
			r.account.BumpLocalEpoch()
			return &s4wave_account.RevokeSessionResponse{}, nil
		}
	}

	accountID := r.account.GetAccountID()
	reqPath := accountAPIPath(accountID, "session", req.GetSessionPeerId())

	actionBody, err := (&api.RevokeSessionAction{
		SessionPeerId: req.GetSessionPeerId(),
	}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal revoke session action")
	}
	envelope, sigs, err := r.resolveOrSignWithTracker(
		ctx,
		req.GetCredential(),
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_REVOKE_SESSION,
		http.MethodDelete,
		reqPath,
		actionBody,
	)
	if err != nil {
		return nil, err
	}
	if _, err := r.sendMultiSig(ctx, http.MethodDelete, reqPath, envelope, sigs); err != nil {
		return nil, errors.Wrap(err, "revoke session")
	}
	r.account.BumpLocalEpoch()
	return &s4wave_account.RevokeSessionResponse{}, nil
}

// GenerateBackupKey generates an Ed25519 backup keypair, registers the
// public key with the cloud as a "pem" auth method, and returns the
// private key PEM for the user to download and store safely.
func (r *AccountResource) GenerateBackupKey(
	ctx context.Context,
	req *s4wave_account.GenerateBackupKeyRequest,
) (*s4wave_account.GenerateBackupKeyResponse, error) {
	// Generate a new Ed25519 keypair for the backup key.
	backupPriv, _, err := bifrost_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "generate backup key")
	}
	backupPeerID, err := peer.IDFromPrivateKey(backupPriv)
	if err != nil {
		return nil, errors.Wrap(err, "derive backup peer ID")
	}
	// Build the EntityKeypair and sign AddKeypairAction.
	kp := &session.EntityKeypair{
		PeerId:     backupPeerID.String(),
		AuthMethod: "pem",
	}
	actionBody, err := (&api.AddKeypairAction{Keypair: kp}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal action")
	}

	// Register the backup key with the cloud.
	accountID := r.account.GetAccountID()
	reqPath := accountAPIPath(accountID, "keypair", "add")
	envelope, sigs, err := r.resolveOrSignWithTracker(
		ctx,
		req.GetCredential(),
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_ADD_KEYPAIR,
		http.MethodPost,
		reqPath,
		actionBody,
	)
	if err != nil {
		return nil, err
	}
	if _, err := r.sendMultiSig(ctx, http.MethodPost, reqPath, envelope, sigs); err != nil {
		return nil, errors.Wrap(err, "register backup key")
	}
	r.account.BumpLocalEpoch()

	// Marshal private key to PEM.
	pemData, err := keypem.MarshalPrivKeyPem(backupPriv)
	if err != nil {
		return nil, errors.Wrap(err, "marshal PEM")
	}

	return &s4wave_account.GenerateBackupKeyResponse{
		PemData: pemData,
		PeerId:  backupPeerID.String(),
	}, nil
}

// ChangePassword changes the account password by deriving a new entity
// keypair from the new password, registering it, and removing the old one.
func (r *AccountResource) ChangePassword(
	ctx context.Context,
	req *s4wave_account.ChangePasswordRequest,
) (*s4wave_account.ChangePasswordResponse, error) {
	oldPassword := req.GetOldPassword()
	newPassword := req.GetNewPassword()
	if oldPassword == "" || newPassword == "" {
		return nil, errors.New("old_password and new_password are required")
	}

	// Derive old entity keypair.
	oldPriv, oldPeerID, err := r.ResolveEntityKey(ctx, &session.EntityCredential{
		Credential: &session.EntityCredential_Password{Password: oldPassword},
	})
	if err != nil {
		return nil, errors.Wrap(err, "resolve old entity key")
	}

	// Derive new entity keypair from new password.
	info, err := r.account.GetAccountState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "fetch account info")
	}
	_, newPriv, err := auth_password.BuildParametersWithUsernamePassword(info.EntityId, []byte(newPassword))
	if err != nil {
		return nil, errors.Wrap(err, "derive new entity key")
	}
	newPeerID, err := peer.IDFromPrivateKey(newPriv)
	if err != nil {
		return nil, errors.Wrap(err, "derive new entity peer ID")
	}
	// Add new keypair (signed with old entity key).
	accountID := r.account.GetAccountID()
	kp := &session.EntityKeypair{
		PeerId:     newPeerID.String(),
		AuthMethod: auth_password.MethodID,
	}
	addAction, err := (&api.AddKeypairAction{Keypair: kp}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal add action")
	}
	addPath := accountAPIPath(accountID, "keypair", "add")
	if _, err := r.signAndSubmit(
		ctx,
		http.MethodPost,
		addPath,
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_ADD_KEYPAIR,
		addAction,
		oldPriv,
		oldPeerID,
	); err != nil {
		return nil, errors.Wrap(err, "add new keypair")
	}

	// Remove old keypair (signed with new entity key, now registered).
	removeAction, err := (&api.RemoveKeypairAction{PeerId: oldPeerID.String()}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal remove action")
	}
	removePath := accountAPIPath(accountID, "keypair", "remove")
	if _, err := r.signAndSubmit(
		ctx,
		http.MethodPost,
		removePath,
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_REMOVE_KEYPAIR,
		removeAction,
		newPriv,
		newPeerID,
	); err != nil {
		return nil, errors.Wrap(err, "remove old keypair")
	}

	r.account.BumpLocalEpoch()

	return &s4wave_account.ChangePasswordResponse{}, nil
}

// accountAPIPath builds a canonical account API route.
func accountAPIPath(accountID string, elems ...string) string {
	return path.Join(append([]string{"/api/account", accountID}, elems...)...)
}

// _ is a type assertion
var _ s4wave_account.SRPCAccountResourceServiceServer = ((*AccountResource)(nil))
