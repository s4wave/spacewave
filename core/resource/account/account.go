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
	s4wave_command "github.com/s4wave/spacewave/sdk/command"
)

// AccountResource wraps a provider account for resource access.
type AccountResource struct {
	mux                 srpc.Invoker
	account             *provider_spacewave.ProviderAccount
	localAccount        *provider_local.ProviderAccount
	localEntityKeyStore *provider_spacewave.EntityKeyStore
	stepUpRef           *refcount.Ref[struct{}]
}

var errCloudAccountRequired = errors.New("account operation requires a cloud account")

func (r *AccountResource) requireCloudAccount() (*provider_spacewave.ProviderAccount, error) {
	if r == nil || r.account == nil {
		return nil, errCloudAccountRequired
	}
	return r.account, nil
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
		r.localEntityKeyStore = provider_spacewave.NewEntityKeyStore()
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
	var prev *s4wave_account.WatchAccountInfoResponse
	return watchCloudBcast(
		strm.Context(),
		r.account.GetAccountBroadcast(),
		func() (*api.AccountStateResponse, bool) {
			info := r.account.AccountStateSnapshot()
			return info, info != nil
		},
		func(info *api.AccountStateResponse) *s4wave_account.WatchAccountInfoResponse {
			return &s4wave_account.WatchAccountInfoResponse{
				AccountId:     info.AccountId,
				EntityId:      info.EntityId,
				ProviderId:    r.account.GetProviderID(),
				AuthThreshold: info.AuthThreshold,
				KeypairCount:  info.KeypairCount,
			}
		},
		func(resp *s4wave_account.WatchAccountInfoResponse) bool {
			if prev != nil && resp.EqualVT(prev) {
				return false
			}
			prev = resp
			return true
		},
		strm.Send,
	)
}

// WatchKeybindingOverrides streams account-scope keybinding overrides.
func (r *AccountResource) WatchKeybindingOverrides(
	req *s4wave_account.WatchKeybindingOverridesRequest,
	strm s4wave_account.SRPCAccountResourceService_WatchKeybindingOverridesStream,
) error {
	if r.localAccount == nil {
		return strm.Send(&s4wave_account.WatchKeybindingOverridesResponse{
			OverrideSet: &s4wave_command.KeybindingOverrideSet{Version: 1},
			ReadOnly:    true,
		})
	}

	ctx, ctxCancel := context.WithCancel(strm.Context())
	defer ctxCancel()

	_, relSO, stateCtr, relStateCtr, err := r.mountLocalAccountSettingsState(ctx, ctxCancel)
	if err != nil {
		return err
	}
	defer relSO()
	defer relStateCtr()

	var prev *s4wave_account.WatchKeybindingOverridesResponse
	return ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			settings, err := decodeLocalAccountSettingsSnapshot(ctx, snap)
			if err != nil {
				return err
			}
			overrideSet := settings.GetKeybindingOverrides()
			if overrideSet == nil {
				overrideSet = &s4wave_command.KeybindingOverrideSet{Version: 1}
			}
			resp := &s4wave_account.WatchKeybindingOverridesResponse{
				OverrideSet: overrideSet.CloneVT(),
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

// ReplaceKeybindingOverrideSet atomically applies a complete account layer replacement.
func (r *AccountResource) ReplaceKeybindingOverrideSet(
	ctx context.Context,
	req *s4wave_account.ReplaceKeybindingOverrideSetRequest,
) (*s4wave_account.ReplaceKeybindingOverrideSetResponse, error) {
	if r.localAccount == nil {
		return nil, errors.New("account keybinding overrides require a local account")
	}
	if req.GetExpectedOverrideSet() == nil {
		return nil, errors.New("expected account keybinding override set is required")
	}
	if err := account_settings.ValidateKeybindingOverrideSet(req.GetOverrideSet()); err != nil {
		return nil, err
	}
	op := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_ReplaceKeybindingOverrideSet{
			ReplaceKeybindingOverrideSet: &account_settings.ReplaceKeybindingOverrideSetOp{
				ExpectedOverrideSet: req.GetExpectedOverrideSet(),
				OverrideSet:         req.GetOverrideSet(),
			},
		},
	}
	if err := r.queueLocalAccountSettingsOp(ctx, op); err != nil {
		return nil, err
	}
	return &s4wave_account.ReplaceKeybindingOverrideSetResponse{}, nil
}

func (r *AccountResource) mountLocalAccountSettingsState(
	ctx context.Context,
	release func(),
) (
	sobject.SharedObject,
	func(),
	ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
	func(),
	error,
) {
	soRef, err := r.localAccount.GetAccountSettingsRef(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	so, relSO, err := r.localAccount.MountSharedObject(ctx, soRef, release)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, release)
	if err != nil {
		relSO()
		return nil, nil, nil, nil, err
	}
	return so, relSO, stateCtr, relStateCtr, nil
}

func (r *AccountResource) queueLocalAccountSettingsOp(
	ctx context.Context,
	op *account_settings.AccountSettingsOp,
) error {
	soRef, err := r.localAccount.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
	}
	so, relSO, err := r.localAccount.MountSharedObject(ctx, soRef, nil)
	if err != nil {
		return err
	}
	defer relSO()

	opData, err := op.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal account settings op")
	}
	localID, err := so.QueueOperation(ctx, opData)
	if err != nil {
		return errors.Wrap(err, "queue account settings op")
	}
	_, rejected, err := so.WaitOperation(ctx, localID)
	if rejected {
		_ = so.ClearOperationResult(ctx, localID)
		if err == nil {
			return errors.New("account settings operation rejected")
		}
	}
	if err != nil {
		return errors.Wrap(err, "wait for account settings op")
	}
	return nil
}

func decodeLocalAccountSettingsSnapshot(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
) (*account_settings.AccountSettings, error) {
	settings := &account_settings.AccountSettings{}
	if snap == nil {
		return settings, nil
	}
	rootInner, err := snap.GetRootInner(ctx)
	if err != nil {
		return nil, err
	}
	if rootInner == nil || len(rootInner.GetStateData()) == 0 {
		return settings, nil
	}
	if err := settings.UnmarshalVT(rootInner.GetStateData()); err != nil {
		return nil, err
	}
	return settings, nil
}

// watchCloudBcast drives a bcast-based watch loop. snapshot reads
// broadcast-guarded state and the wait channel atomically inside one HoldLock
// per iteration; it must not read state guarded by other locks since that
// would risk missed-wakeup races. build runs outside the lock and produces
// the response sent to the client when changed returns true. send is only
// invoked when snapshot returns valid=true and changed returns true. The
// loop exits when ctx is canceled or send returns an error.
func watchCloudBcast[S any, T any](
	ctx context.Context,
	bcast accountBroadcaster,
	snapshot func() (state S, valid bool),
	build func(state S) T,
	changed func(resp T) bool,
	send func(resp T) error,
) error {
	for {
		var (
			state S
			valid bool
			ch    <-chan struct{}
		)
		bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			state, valid = snapshot()
			ch = getWaitCh()
		})

		if valid {
			resp := build(state)
			if changed(resp) {
				if err := send(resp); err != nil {
					return err
				}
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// accountBroadcaster is the subset of broadcast.Broadcast used by
// watchCloudBcast. It is satisfied by *broadcast.Broadcast.
type accountBroadcaster interface {
	HoldLock(cb func(broadcast func(), getWaitCh func() <-chan struct{}))
}

// WatchAuthMethods streams the account auth-method rows for this account.
func (r *AccountResource) WatchAuthMethods(
	req *s4wave_account.WatchAuthMethodsRequest,
	strm s4wave_account.SRPCAccountResourceService_WatchAuthMethodsStream,
) error {
	acc, err := r.requireCloudAccount()
	if err != nil {
		return err
	}

	var prev *s4wave_account.WatchAuthMethodsResponse
	return watchCloudBcast(
		strm.Context(),
		acc.GetAccountBroadcast(),
		func() ([]*api.AccountAuthMethod, bool) {
			authMethods := acc.AuthMethodsSnapshot()
			valid := acc.AccountStateSnapshot() != nil
			return authMethods, valid
		},
		func(authMethods []*api.AccountAuthMethod) *s4wave_account.WatchAuthMethodsResponse {
			return &s4wave_account.WatchAuthMethodsResponse{
				AuthMethods: authMethods,
			}
		},
		func(resp *s4wave_account.WatchAuthMethodsResponse) bool {
			if prev != nil && resp.EqualVT(prev) {
				return false
			}
			prev = resp
			return true
		},
		strm.Send,
	)
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

	var prev *s4wave_account.WatchSessionsResponse
	return watchCloudBcast(
		ctx,
		r.account.GetAccountBroadcast(),
		func() ([]*api.AccountSessionInfo, bool) {
			return r.account.SessionsSnapshot()
		},
		func(rows []*api.AccountSessionInfo) *s4wave_account.WatchSessionsResponse {
			metadata := buildSessionPresentationMapFromSnapshot(stateCtr)
			currentPeerID := r.account.GetCurrentSessionPeerID().String()
			return &s4wave_account.WatchSessionsResponse{
				Sessions: buildCloudSessionRows(currentPeerID, rows, metadata),
			}
		},
		func(resp *s4wave_account.WatchSessionsResponse) bool {
			if prev != nil && resp.EqualVT(prev) {
				return false
			}
			prev = resp
			return true
		},
		strm.Send,
	)
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
	if r.localAccount != nil {
		return r.resolveLocalEntityKey(cred)
	}
	if cred == nil {
		return nil, "", errors.New("credential is required")
	}
	acc, err := r.requireCloudAccount()
	if err != nil {
		return nil, "", errors.Wrap(err, "entity key resolution")
	}
	password := cred.GetPassword()
	pemPrivateKey := cred.GetPemPrivateKey()
	if password != "" {
		info, err := acc.GetAccountState(ctx)
		if err != nil {
			return nil, "", errors.Wrap(err, "fetch account info")
		}
		return derivePasswordEntityKey(info.EntityId, []byte(password))
	}
	if len(pemPrivateKey) > 0 {
		return parsePemEntityKey(pemPrivateKey)
	}
	return nil, "", errors.New("password or pem_private_key is required")
}

func (r *AccountResource) resolveLocalEntityKey(cred *session.EntityCredential) (bifrost_crypto.PrivKey, peer.ID, error) {
	if cred == nil {
		return nil, "", errors.New("credential is required")
	}
	password := cred.GetPassword()
	pemPrivateKey := cred.GetPemPrivateKey()
	if password != "" {
		return derivePasswordEntityKey(r.localAccount.GetAccountID(), []byte(password))
	}
	if len(pemPrivateKey) > 0 {
		return parsePemEntityKey(pemPrivateKey)
	}
	return nil, "", errors.New("password or pem_private_key is required")
}

func derivePasswordEntityKey(username string, password []byte) (bifrost_crypto.PrivKey, peer.ID, error) {
	_, entityPriv, err := auth_password.BuildParametersWithUsernamePassword(username, password)
	if err != nil {
		return nil, "", errors.Wrap(err, "derive entity key")
	}
	entityPeerID, err := peer.IDFromPrivateKey(entityPriv)
	if err != nil {
		return nil, "", errors.Wrap(err, "derive entity peer ID")
	}
	return entityPriv, entityPeerID, nil
}

func parsePemEntityKey(pemPrivateKey []byte) (bifrost_crypto.PrivKey, peer.ID, error) {
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

// buildMultiSigEnvelope builds the typed MultiSigActionEnvelope bytes that
// entity keys sign. Binding account_id, kind, method, and path to the signed
// bytes prevents replay across accounts or endpoints.
func (r *AccountResource) buildMultiSigEnvelope(kind api.MultiSigActionKind, method, reqPath string, actionBody []byte) ([]byte, error) {
	acc, err := r.requireCloudAccount()
	if err != nil {
		return nil, err
	}
	env := &api.MultiSigActionEnvelope{
		AccountId: acc.GetAccountID(),
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
	acc, err := r.requireCloudAccount()
	if err != nil {
		return nil, err
	}
	cli := acc.GetSessionClient()
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

// submitTrackedAction resolves or signs the entity multi-sig envelope for an
// account API mutation, submits it through the multi-sig endpoint, and bumps
// the local epoch on success. wrapMsg is used to wrap a non-nil send error.
func (r *AccountResource) submitTrackedAction(
	ctx context.Context,
	cred *session.EntityCredential,
	kind api.MultiSigActionKind,
	method, reqPath string,
	actionBody []byte,
	wrapMsg string,
) error {
	acc, err := r.requireCloudAccount()
	if err != nil {
		return err
	}
	envelope, sigs, err := r.resolveOrSignWithStore(ctx, cred, kind, method, reqPath, actionBody)
	if err != nil {
		return err
	}
	if _, err := r.sendMultiSig(ctx, method, reqPath, envelope, sigs); err != nil {
		return errors.Wrap(err, wrapMsg)
	}
	acc.BumpLocalEpoch()
	return nil
}

// AddAuthMethod adds a new entity keypair (auth method) to the account.
func (r *AccountResource) AddAuthMethod(
	ctx context.Context,
	req *s4wave_account.AddAuthMethodRequest,
) (*s4wave_account.AddAuthMethodResponse, error) {
	acc, err := r.requireCloudAccount()
	if err != nil {
		return nil, err
	}
	kp := req.GetKeypair()
	if kp == nil {
		return nil, errors.New("keypair is required")
	}
	actionBody, err := (&api.AddKeypairAction{Keypair: kp}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal add keypair action")
	}
	reqPath := accountAPIPath(acc.GetAccountID(), "keypair", "add")
	if err := r.submitTrackedAction(
		ctx,
		req.GetCredential(),
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_ADD_KEYPAIR,
		http.MethodPost,
		reqPath,
		actionBody,
		"add auth method",
	); err != nil {
		return nil, err
	}
	return &s4wave_account.AddAuthMethodResponse{}, nil
}

// RemoveAuthMethod removes an entity keypair from the account.
func (r *AccountResource) RemoveAuthMethod(
	ctx context.Context,
	req *s4wave_account.RemoveAuthMethodRequest,
) (*s4wave_account.RemoveAuthMethodResponse, error) {
	acc, err := r.requireCloudAccount()
	if err != nil {
		return nil, err
	}
	actionBody, err := (&api.RemoveKeypairAction{PeerId: req.GetPeerId()}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal remove keypair action")
	}
	reqPath := accountAPIPath(acc.GetAccountID(), "keypair", "remove")
	if err := r.submitTrackedAction(
		ctx,
		req.GetCredential(),
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_REMOVE_KEYPAIR,
		http.MethodPost,
		reqPath,
		actionBody,
		"remove auth method",
	); err != nil {
		return nil, err
	}
	return &s4wave_account.RemoveAuthMethodResponse{}, nil
}

// SetSecurityLevel updates the auth threshold for the account.
func (r *AccountResource) SetSecurityLevel(
	ctx context.Context,
	req *s4wave_account.SetSecurityLevelRequest,
) (*s4wave_account.SetSecurityLevelResponse, error) {
	acc, err := r.requireCloudAccount()
	if err != nil {
		return nil, err
	}
	actionBody, err := (&api.UpdateThresholdAction{Threshold: req.GetThreshold()}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal update threshold action")
	}
	reqPath := accountAPIPath(acc.GetAccountID(), "threshold")
	if err := r.submitTrackedAction(
		ctx,
		req.GetCredential(),
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_UPDATE_THRESHOLD,
		http.MethodPost,
		reqPath,
		actionBody,
		"set security level",
	); err != nil {
		return nil, err
	}
	return &s4wave_account.SetSecurityLevelResponse{}, nil
}

// RevokeSession revokes a session by peer ID.
//
// When credential is nil and the requested session_peer_id matches the
// current session, uses the session self-revoke endpoint (no entity key
// needed). Otherwise falls through to the entity multi-sig path, with
// key-store fallback.
func (r *AccountResource) RevokeSession(
	ctx context.Context,
	req *s4wave_account.RevokeSessionRequest,
) (*s4wave_account.RevokeSessionResponse, error) {
	acc, err := r.requireCloudAccount()
	if err != nil {
		return nil, errors.Wrap(err, "session revoke")
	}
	cli := acc.GetSessionClient()

	// Self-revoke path: no credential provided, current session.
	if req.GetCredential() == nil {
		currentPeerID := cli.GetPeerID().String()
		if req.GetSessionPeerId() == currentPeerID {
			if err := cli.SelfRevoke(ctx); err != nil {
				return nil, errors.Wrap(err, "self-revoke session")
			}
			acc.BumpLocalEpoch()
			return &s4wave_account.RevokeSessionResponse{}, nil
		}
	}

	actionBody, err := (&api.RevokeSessionAction{
		SessionPeerId: req.GetSessionPeerId(),
	}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal revoke session action")
	}
	reqPath := accountAPIPath(acc.GetAccountID(), "session", req.GetSessionPeerId())
	if err := r.submitTrackedAction(
		ctx,
		req.GetCredential(),
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_REVOKE_SESSION,
		http.MethodDelete,
		reqPath,
		actionBody,
		"revoke session",
	); err != nil {
		return nil, err
	}
	return &s4wave_account.RevokeSessionResponse{}, nil
}

// GenerateBackupKey generates an Ed25519 backup keypair, registers the public
// key with the account, and returns the private key PEM for the user to store
// safely.
func (r *AccountResource) GenerateBackupKey(
	ctx context.Context,
	req *s4wave_account.GenerateBackupKeyRequest,
) (*s4wave_account.GenerateBackupKeyResponse, error) {
	if r.localAccount != nil {
		return r.generateLocalBackupKey(ctx)
	}
	acc, err := r.requireCloudAccount()
	if err != nil {
		return nil, err
	}
	backupPeerID, pemData, err := generateBackupEntityKey()
	if err != nil {
		return nil, err
	}
	kp := &session.EntityKeypair{
		PeerId:     backupPeerID.String(),
		AuthMethod: "pem",
	}
	actionBody, err := (&api.AddKeypairAction{Keypair: kp}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal action")
	}

	reqPath := accountAPIPath(acc.GetAccountID(), "keypair", "add")
	if err := r.submitTrackedAction(
		ctx,
		req.GetCredential(),
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_ADD_KEYPAIR,
		http.MethodPost,
		reqPath,
		actionBody,
		"register backup key",
	); err != nil {
		return nil, err
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
	if r.localAccount != nil {
		return r.changeLocalPassword(ctx, req)
	}
	acc, err := r.requireCloudAccount()
	if err != nil {
		return nil, err
	}
	oldPassword := req.GetOldPassword()
	newPassword := req.GetNewPassword()
	if oldPassword == "" || newPassword == "" {
		return nil, errors.New("old_password and new_password are required")
	}

	oldPriv, oldPeerID, err := r.ResolveEntityKey(ctx, &session.EntityCredential{
		Credential: &session.EntityCredential_Password{Password: oldPassword},
	})
	if err != nil {
		return nil, errors.Wrap(err, "resolve old entity key")
	}

	info, err := acc.GetAccountState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "fetch account info")
	}
	newPriv, newPeerID, err := derivePasswordEntityKey(info.EntityId, []byte(newPassword))
	if err != nil {
		return nil, errors.Wrap(err, "derive new entity key")
	}
	accountID := acc.GetAccountID()
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

	acc.BumpLocalEpoch()

	return &s4wave_account.ChangePasswordResponse{}, nil
}

func (r *AccountResource) generateLocalBackupKey(
	ctx context.Context,
) (*s4wave_account.GenerateBackupKeyResponse, error) {
	backupPeerID, pemData, err := generateBackupEntityKey()
	if err != nil {
		return nil, err
	}
	if err := r.addLocalEntityKeypair(ctx, &session.EntityKeypair{
		PeerId:     backupPeerID.String(),
		AuthMethod: "pem",
	}); err != nil {
		return nil, errors.Wrap(err, "add backup keypair")
	}

	return &s4wave_account.GenerateBackupKeyResponse{
		PemData: pemData,
		PeerId:  backupPeerID.String(),
	}, nil
}

func (r *AccountResource) changeLocalPassword(
	ctx context.Context,
	req *s4wave_account.ChangePasswordRequest,
) (*s4wave_account.ChangePasswordResponse, error) {
	oldPassword := req.GetOldPassword()
	newPassword := req.GetNewPassword()
	if oldPassword == "" || newPassword == "" {
		return nil, errors.New("old_password and new_password are required")
	}

	_, oldPeerID, err := r.ResolveEntityKey(ctx, &session.EntityCredential{
		Credential: &session.EntityCredential_Password{Password: oldPassword},
	})
	if err != nil {
		return nil, errors.Wrap(err, "resolve old entity key")
	}
	_, newPeerID, err := r.ResolveEntityKey(ctx, &session.EntityCredential{
		Credential: &session.EntityCredential_Password{Password: newPassword},
	})
	if err != nil {
		return nil, errors.Wrap(err, "derive new entity key")
	}
	if err := r.addLocalEntityKeypair(ctx, &session.EntityKeypair{
		PeerId:     newPeerID.String(),
		AuthMethod: auth_password.MethodID,
	}); err != nil {
		return nil, errors.Wrap(err, "add new keypair")
	}
	if err := r.removeLocalEntityKeypair(ctx, oldPeerID.String()); err != nil {
		return nil, errors.Wrap(err, "remove old keypair")
	}
	return &s4wave_account.ChangePasswordResponse{}, nil
}

func (r *AccountResource) addLocalEntityKeypair(ctx context.Context, kp *session.EntityKeypair) error {
	return r.queueLocalAccountSettingsOp(ctx, &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddEntityKeypair{
			AddEntityKeypair: kp,
		},
	})
}

func (r *AccountResource) removeLocalEntityKeypair(ctx context.Context, peerID string) error {
	return r.queueLocalAccountSettingsOp(ctx, &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_RemoveEntityKeypair{
			RemoveEntityKeypair: &account_settings.RemoveEntityKeypairOp{
				PeerId: peerID,
			},
		},
	})
}

func generateBackupEntityKey() (peer.ID, []byte, error) {
	backupPriv, _, err := bifrost_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return "", nil, errors.Wrap(err, "generate backup key")
	}
	backupPeerID, err := peer.IDFromPrivateKey(backupPriv)
	if err != nil {
		return "", nil, errors.Wrap(err, "derive backup peer ID")
	}
	pemData, err := keypem.MarshalPrivKeyPem(backupPriv)
	if err != nil {
		return "", nil, errors.Wrap(err, "marshal PEM")
	}
	return backupPeerID, pemData, nil
}

// accountAPIPath builds a canonical account API route.
func accountAPIPath(accountID string, elems ...string) string {
	return path.Join(append([]string{"/api/account", accountID}, elems...)...)
}

// _ is a type assertion
var _ s4wave_account.SRPCAccountResourceServiceServer = ((*AccountResource)(nil))
