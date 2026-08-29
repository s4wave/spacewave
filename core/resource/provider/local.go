package resource_provider

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_provider_local "github.com/s4wave/spacewave/sdk/provider/local"
	"github.com/sirupsen/logrus"
)

// LocalProviderResource implements the LocalProviderResourceService.
type LocalProviderResource struct {
	*ProviderResource
	le       *logrus.Entry
	b        bus.Bus
	provider *provider_local.Provider
}

// NewLocalProviderResource creates a new LocalProviderResource.
func NewLocalProviderResource(pr *ProviderResource, le *logrus.Entry, b bus.Bus, prov *provider_local.Provider) *LocalProviderResource {
	return &LocalProviderResource{
		ProviderResource: pr,
		le:               le,
		b:                b,
		provider:         prov,
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

	sessRef, err := s.provider.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		return nil, err
	}

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

	return &s4wave_provider_local.CreateAccountResponse{SessionListEntry: listEntry}, nil
}

// CompleteSpaceLinkEnrollment creates or reopens the caller's own local
// session from the supplied Device key and joins the target Space through the
// one-use targeted invite from a local SpaceLink approval.
func (s *LocalProviderResource) CompleteSpaceLinkEnrollment(
	ctx context.Context,
	req *s4wave_provider_local.CompleteSpaceLinkEnrollmentRequest,
) (*s4wave_provider_local.CompleteSpaceLinkEnrollmentResponse, error) {
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

	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

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
	if !localAccountHasSharedObject(localAcc, invite.GetSharedObjectId()) {
		if _, err := localAcc.JoinViaInvite(ctx, sessionKey, invite, ""); err != nil {
			return nil, errors.Wrap(err, "join space via invite")
		}
	} else {
		if err := localAcc.EnsureSessionTransport(ctx, sessionKey, ""); err != nil {
			return nil, errors.Wrap(err, "start session transport")
		}
		if err := localAcc.StartP2PSync(context.WithoutCancel(ctx), localAcc.GetSessionTransport()); err != nil {
			return nil, errors.Wrap(err, "start P2P sync")
		}
	}
	ownerPeerID, err := peer.IDB58Decode(invite.GetOwnerPeerId())
	if err != nil {
		return nil, errors.Wrap(err, "parse invite owner peer id")
	}
	if err := localAcc.RetainP2PPeer(ctx, ownerPeerID); err != nil {
		return nil, errors.Wrap(err, "retain invite owner link")
	}

	return &s4wave_provider_local.CompleteSpaceLinkEnrollmentResponse{SessionListEntry: listEntry}, nil
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

// _ is a type assertion
var _ s4wave_provider_local.SRPCLocalProviderResourceServiceServer = (*LocalProviderResource)(nil)
