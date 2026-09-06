package resource_provider

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_provider_local "github.com/s4wave/spacewave/sdk/provider/local"
	"github.com/sirupsen/logrus"
)

// localSpaceLinkNetworkTimeout bounds contact with the approving peer.
const localSpaceLinkNetworkTimeout = 10 * time.Second

// LocalProviderResource retains Device Sessions until its Resource is released.
type LocalProviderResource struct {
	// ProviderResource serves common provider operations.
	*ProviderResource
	// b resolves the local Session controller and mounted Sessions.
	b bus.Bus
	// provider creates and reopens independently keyed local accounts.
	provider *provider_local.Provider

	// deviceSessionsMu guards retained Sessions and the release fence.
	deviceSessionsMu sync.Mutex
	// deviceSessions bridges completed enrollment to other Session consumers.
	deviceSessions map[string]directive.Reference
	// released prevents an in-flight enrollment from retaining a late Session.
	released bool
}

// NewLocalProviderResource creates a new LocalProviderResource.
func NewLocalProviderResource(pr *ProviderResource, _ *logrus.Entry, b bus.Bus, prov *provider_local.Provider) *LocalProviderResource {
	return &LocalProviderResource{
		ProviderResource: pr,
		b:                b,
		provider:         prov,
	}
}

// Release drops every Session retained by completed enrollment. An in-flight
// completion must release its late mount instead of reviving this Resource.
func (s *LocalProviderResource) Release() {
	// Withdraw retained references before invoking their teardown callbacks.
	s.deviceSessionsMu.Lock()
	s.released = true
	refs := s.deviceSessions
	s.deviceSessions = nil
	s.deviceSessionsMu.Unlock()

	// Session teardown may resolve other resources and must run outside the lock.
	for _, ref := range refs {
		ref.Release()
	}
}

// CreateAccount creates a ProviderAccount and Session on the local provider.
func (s *LocalProviderResource) CreateAccount(
	ctx context.Context,
	req *s4wave_provider_local.CreateAccountRequest,
) (*s4wave_provider_local.CreateAccountResponse, error) {
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	// Create an independent local account before registering its Session.
	sessRef, err := s.provider.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		return nil, err
	}

	// Publish the new Session metadata through the controller.
	meta := &session.SessionMetadata{
		ProviderDisplayName: "Local",
		ProviderId:          "local",
		ProviderAccountId:   sessRef.GetProviderResourceRef().GetProviderAccountId(),
		CreatedAt:           time.Now().UnixMilli(),
	}
	listEntry, err := sessionCtrl.RegisterSession(ctx, sessRef, meta)
	if err != nil {
		return nil, err
	}

	// Return the registered Session to the caller.
	return &s4wave_provider_local.CreateAccountResponse{SessionListEntry: listEntry}, nil
}

// CompleteSpaceLinkEnrollment creates or reopens the caller's own local
// session from the supplied Device key and joins the target Space through the
// one-use targeted invite from a local SpaceLink approval.
func (s *LocalProviderResource) CompleteSpaceLinkEnrollment(
	ctx context.Context,
	req *s4wave_provider_local.CompleteSpaceLinkEnrollmentRequest,
) (*s4wave_provider_local.CompleteSpaceLinkEnrollmentResponse, error) {
	// Reject new enrollment after the Resource's retained Sessions were released.
	s.deviceSessionsMu.Lock()
	released := s.released
	s.deviceSessionsMu.Unlock()
	if released {
		return nil, errors.New("provider resource is released")
	}

	// Require a targeted invitation and prove the supplied Session key matches it.
	invite := req.GetInvite()
	if invite == nil {
		return nil, errors.New("invite is required")
	}
	if len(req.GetSessionPemPrivateKey()) == 0 {
		return nil, errors.New("session_pem_private_key is required")
	}
	sessionKey, err := keypem.ParsePrivKeyPem(req.GetSessionPemPrivateKey())
	if err != nil {
		return nil, errors.Wrap(err, "parse session key")
	}
	sessionPeerID, err := peer.IDFromPrivateKey(sessionKey)
	if err != nil {
		return nil, errors.Wrap(err, "derive session peer id")
	}
	if req.GetSessionPeerId() != "" && req.GetSessionPeerId() != sessionPeerID.String() {
		return nil, errors.New("session key does not match expected session peer id")
	}
	if invite.GetTargetPeerId() != "" && invite.GetTargetPeerId() != sessionPeerID.String() {
		return nil, errors.New("invite targets a different peer")
	}

	// Retain the Session inventory while resolving or registering this identity.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	// Reuse this Device identity without replacing another account.
	listEntry, err := s.lookupLocalSessionByPeerID(ctx, sessionCtrl, sessionPeerID.String())
	if err != nil {
		return nil, err
	}
	if listEntry == nil {
		sessRef, err := s.provider.CreateLocalAccountAndSessionWithKey(ctx, "", req.GetSessionPemPrivateKey())
		if err != nil {
			return nil, errors.Wrap(err, "create local session")
		}
		meta := &session.SessionMetadata{
			ProviderDisplayName: "Local",
			ProviderId:          sessRef.GetProviderResourceRef().GetProviderId(),
			ProviderAccountId:   sessRef.GetProviderResourceRef().GetProviderAccountId(),
			CreatedAt:           time.Now().UnixMilli(),
		}
		listEntry, err = sessionCtrl.RegisterSession(ctx, sessRef, meta)
		if err != nil {
			return nil, errors.Wrap(err, "register session")
		}
	}

	// Retain the selected account through invite redemption and transport setup.
	sessRef := listEntry.GetSessionRef()
	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	accIface, accRel, err := s.provider.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		return nil, errors.Wrap(err, "access provider account")
	}
	defer accRel()
	localAcc, ok := accIface.(*provider_local.ProviderAccount)
	if !ok {
		return nil, errors.New("unexpected provider account type")
	}

	// Redeem only once; retries reuse the account's existing grant and transport.
	networkCtx, networkCancel := context.WithTimeout(ctx, localSpaceLinkNetworkTimeout)
	defer networkCancel()
	if !localAccountHasSharedObject(localAcc, invite.GetSharedObjectId()) {
		if _, err := localAcc.JoinViaInvite(networkCtx, sessionKey, invite, ""); err != nil {
			return nil, errors.Wrap(err, "join space via invite")
		}
	} else {
		if err := localAcc.EnsureConfiguredSessionTransport(networkCtx, sessionKey); err != nil {
			return nil, errors.Wrap(err, "start session transport")
		}
		if !localAcc.IsP2PSyncRunning() {
			if err := localAcc.StartPersistentP2PSync(ctx, localAcc.GetSessionTransport()); err != nil {
				return nil, errors.Wrap(err, "start P2P sync")
			}
		}
	}

	// Keep the approving peer reachable while the completed Session is retained.
	ownerPeerID, err := peer.IDB58Decode(invite.GetOwnerPeerId())
	if err != nil {
		return nil, errors.Wrap(err, "parse invite owner peer id")
	}
	if err := localAcc.RetainP2PPeer(networkCtx, ownerPeerID); err != nil {
		return nil, errors.Wrap(err, "retain invite owner link")
	}
	if err := s.retainDeviceSession(ctx, sessRef); err != nil {
		return nil, errors.Wrap(err, "retain completed Device session")
	}

	// Publish enrollment only after its Session remains retained.
	return &s4wave_provider_local.CompleteSpaceLinkEnrollmentResponse{SessionListEntry: listEntry}, nil
}

// retainDeviceSession keeps a completed Device session mounted for the local
// provider resource lifetime. This bridges the RPC response to the daemon's
// capacity observer without dropping its transport in between.
func (s *LocalProviderResource) retainDeviceSession(ctx context.Context, sessRef *session.SessionRef) error {
	// Acquire a Session reference before entering the release fence.
	_, ref, err := session.ExMountSession(ctx, s.b, sessRef, false, nil)
	if err != nil {
		return err
	}
	if ref == nil {
		return errors.New("completed Device session could not be mounted")
	}

	// Keep one mount per account, or release a late acquisition after closure.
	key := sessRef.GetProviderResourceRef().GetProviderAccountId()
	s.deviceSessionsMu.Lock()
	if s.released {
		s.deviceSessionsMu.Unlock()
		ref.Release()
		return errors.New("provider resource is released")
	}
	if s.deviceSessions == nil {
		s.deviceSessions = make(map[string]directive.Reference)
	}
	if _, exists := s.deviceSessions[key]; exists {
		s.deviceSessionsMu.Unlock()
		ref.Release()
		return nil
	}
	s.deviceSessions[key] = ref
	s.deviceSessionsMu.Unlock()
	return nil
}

// lookupLocalSessionByPeerID returns the registered local session whose
// mounted identity matches peerID. Missing is not an error.
func (s *LocalProviderResource) lookupLocalSessionByPeerID(
	ctx context.Context,
	sessionCtrl session.SessionController,
	peerID string,
) (*session.SessionListEntry, error) {
	if peerID == "" {
		return nil, nil
	}

	// Match identity against registered Sessions rather than caller-selected accounts.
	entries, err := sessionCtrl.ListSessions(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "list sessions")
	}
	for _, entry := range entries {
		ref := entry.GetSessionRef()
		if ref == nil {
			continue
		}
		provRef := ref.GetProviderResourceRef()
		if provRef.GetProviderId() != "local" {
			continue
		}

		// Retain each candidate account only while checking its Session identity.
		accountID := provRef.GetProviderAccountId()
		accIface, accRel, err := s.provider.AccessProviderAccount(ctx, accountID, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "access local account %q while matching session identity", accountID)
		}
		localAcc, ok := accIface.(*provider_local.ProviderAccount)
		if !ok {
			accRel()
			return nil, errors.Errorf("local account %q has unexpected type", accountID)
		}
		sess, sessRel, err := localAcc.MountSession(ctx, ref, nil)
		if err != nil {
			accRel()
			return nil, errors.Wrapf(err, "mount local session for account %q while matching identity", accountID)
		}
		match := sess.GetPeerId().String() == peerID
		sessRel()
		accRel()
		if match {
			return entry, nil
		}
	}
	return nil, nil
}

// localAccountHasSharedObject reports whether the account already lists the
// shared object, so a later enrollment retry can remount without consuming
// the one-use invite again.
func localAccountHasSharedObject(localAcc *provider_local.ProviderAccount, soID string) bool {
	if soID == "" {
		return false
	}
	for _, entry := range localAcc.GetSOListCtr().GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == soID {
			return true
		}
	}
	return false
}

// _ verifies the local provider Resource contract.
var _ s4wave_provider_local.SRPCLocalProviderResourceServiceServer = (*LocalProviderResource)(nil)
