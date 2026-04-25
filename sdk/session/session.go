package s4wave_session

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	session "github.com/s4wave/spacewave/core/session"
)

// Session is a session resource that provides access to session functionality.
// The MountSession directive will remain active until this resource is released.
//
// This Go SDK implementation wraps SessionResourceService.
type Session struct {
	client  *resource_client.Client
	ref     resource_client.ResourceRef
	service SRPCSessionResourceServiceClient
}

// NewSession creates a new Session resource wrapper.
func NewSession(client *resource_client.Client, ref resource_client.ResourceRef) (*Session, error) {
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	return &Session{
		client:  client,
		ref:     ref,
		service: NewSRPCSessionResourceServiceClient(srpcClient),
	}, nil
}

// GetResourceRef returns the resource reference.
func (s *Session) GetResourceRef() resource_client.ResourceRef {
	return s.ref
}

// Release releases the resource reference.
func (s *Session) Release() {
	s.ref.Release()
}

// GetSessionInfo returns information about this session.
func (s *Session) GetSessionInfo(ctx context.Context) (*GetSessionInfoResponse, error) {
	return s.service.GetSessionInfo(ctx, &GetSessionInfoRequest{})
}

// CreateSpace creates a new Space as a SharedObject within the Session.
// ownerType is "account" or "organization"; ownerID is the principal id
// (account id or organization id).
func (s *Session) CreateSpace(ctx context.Context, spaceName, ownerType, ownerID string) (*CreateSpaceResponse, error) {
	return s.service.CreateSpace(ctx, &CreateSpaceRequest{
		SpaceName: spaceName,
		OwnerType: ownerType,
		OwnerId:   ownerID,
	})
}

// DeleteSpace deletes a space and its associated storage.
func (s *Session) DeleteSpace(ctx context.Context, sharedObjectID string) (*DeleteSpaceResponse, error) {
	return s.service.DeleteSpace(ctx, &DeleteSpaceRequest{SharedObjectId: sharedObjectID})
}

// WatchResourcesList returns a stream of the full spaces list snapshots.
func (s *Session) WatchResourcesList(ctx context.Context) (SRPCSessionResourceService_WatchResourcesListClient, error) {
	return s.service.WatchResourcesList(ctx, &WatchResourcesListRequest{})
}

// WatchSharedObjectHealth returns a stream of SharedObject health by SharedObject ID.
func (s *Session) WatchSharedObjectHealth(ctx context.Context, sharedObjectID string) (SRPCSessionResourceService_WatchSharedObjectHealthClient, error) {
	return s.service.WatchSharedObjectHealth(ctx, &WatchSharedObjectHealthRequest{SharedObjectId: sharedObjectID})
}

// WatchSyncStatus returns a stream of session sync status snapshots.
func (s *Session) WatchSyncStatus(ctx context.Context) (SRPCSessionResourceService_WatchSyncStatusClient, error) {
	return s.service.WatchSyncStatus(ctx, &WatchSyncStatusRequest{})
}

// MountSharedObject mounts a shared object within the session by ID.
// Returns the response containing the resource ID and shared object metadata.
func (s *Session) MountSharedObject(ctx context.Context, sharedObjectID string) (*MountSharedObjectResponse, error) {
	return s.service.MountSharedObject(ctx, &MountSharedObjectRequest{SharedObjectId: sharedObjectID})
}

// WatchLockState returns a stream of the current lock state and updates on changes.
func (s *Session) WatchLockState(ctx context.Context) (SRPCSessionResourceService_WatchLockStateClient, error) {
	return s.service.WatchLockState(ctx, &WatchLockStateRequest{})
}

// SetLockMode changes the session lock mode.
func (s *Session) SetLockMode(ctx context.Context, mode session.SessionLockMode, pin []byte) error {
	_, err := s.service.SetLockMode(ctx, &SetLockModeRequest{Mode: mode, Pin: pin})
	return err
}

// UnlockSession unlocks a PIN-locked session with the given PIN.
func (s *Session) UnlockSession(ctx context.Context, pin []byte) error {
	_, err := s.service.UnlockSession(ctx, &UnlockSessionRequest{Pin: pin})
	return err
}

// LockSession locks a running session, scrubbing the privkey and requiring PIN re-entry.
func (s *Session) LockSession(ctx context.Context) error {
	_, err := s.service.LockSession(ctx, &LockSessionRequest{})
	return err
}

// GetTransferInventory returns the list of spaces on a session for transfer planning.
func (s *Session) GetTransferInventory(ctx context.Context, sessionIdx uint32) (*GetTransferInventoryResponse, error) {
	return s.service.GetTransferInventory(ctx, &GetTransferInventoryRequest{SessionIndex: sessionIdx})
}

// StartTransfer starts a transfer operation between two sessions.
func (s *Session) StartTransfer(ctx context.Context, req *StartTransferRequest) error {
	_, err := s.service.StartTransfer(ctx, req)
	return err
}

// WatchTransferProgress streams transfer state updates for an active transfer.
func (s *Session) WatchTransferProgress(ctx context.Context) (SRPCSessionResourceService_WatchTransferProgressClient, error) {
	return s.service.WatchTransferProgress(ctx, &WatchTransferProgressRequest{})
}

// CancelTransfer stops an in-progress transfer.
func (s *Session) CancelTransfer(ctx context.Context) error {
	_, err := s.service.CancelTransfer(ctx, &CancelTransferRequest{})
	return err
}

// WatchPairingStatus streams pairing state changes during device linking.
func (s *Session) WatchPairingStatus(ctx context.Context) (SRPCSessionResourceService_WatchPairingStatusClient, error) {
	return s.service.WatchPairingStatus(ctx, &WatchPairingStatusRequest{})
}

// CreateLocalPairingOffer generates a WebRTC SDP offer for no-cloud pairing.
func (s *Session) CreateLocalPairingOffer(ctx context.Context) (*CreateLocalPairingOfferResponse, error) {
	return s.service.CreateLocalPairingOffer(ctx, &CreateLocalPairingOfferRequest{})
}

// AcceptLocalPairingOffer accepts a remote offer and returns an answer.
func (s *Session) AcceptLocalPairingOffer(ctx context.Context, offerPayload string) (*AcceptLocalPairingOfferResponse, error) {
	return s.service.AcceptLocalPairingOffer(ctx, &AcceptLocalPairingOfferRequest{OfferPayload: offerPayload})
}

// AcceptLocalPairingAnswer accepts a remote answer to complete the connection.
func (s *Session) AcceptLocalPairingAnswer(ctx context.Context, answerPayload string) (*AcceptLocalPairingAnswerResponse, error) {
	return s.service.AcceptLocalPairingAnswer(ctx, &AcceptLocalPairingAnswerRequest{AnswerPayload: answerPayload})
}

// CreateSpaceInvite creates an invite for a space shared object.
func (s *Session) CreateSpaceInvite(ctx context.Context, req *CreateSpaceInviteRequest) (*CreateSpaceInviteResponse, error) {
	return s.service.CreateSpaceInvite(ctx, req)
}

// RevokeSpaceInvite revokes an invite on a space shared object.
func (s *Session) RevokeSpaceInvite(ctx context.Context, spaceID string, inviteID string) error {
	_, err := s.service.RevokeSpaceInvite(ctx, &RevokeSpaceInviteRequest{SpaceId: spaceID, InviteId: inviteID})
	return err
}

// JoinSpaceViaInvite joins a space using an out-of-band invite message.
func (s *Session) JoinSpaceViaInvite(ctx context.Context, req *JoinSpaceViaInviteRequest) (*JoinSpaceViaInviteResponse, error) {
	return s.service.JoinSpaceViaInvite(ctx, req)
}

// GeneratePairingCode generates a short pairing code for cloud-relay device linking.
func (s *Session) GeneratePairingCode(ctx context.Context) (*GeneratePairingCodeResponse, error) {
	return s.service.GeneratePairingCode(ctx, &GeneratePairingCodeRequest{})
}

// CompletePairing resolves a pairing code from the other device to link.
func (s *Session) CompletePairing(ctx context.Context, code string) (*CompletePairingResponse, error) {
	return s.service.CompletePairing(ctx, &CompletePairingRequest{Code: code})
}

// GetSASEmoji derives the 6-emoji SAS verification sequence for a remote peer.
func (s *Session) GetSASEmoji(ctx context.Context, remotePeerID string) (*GetSASEmojiResponse, error) {
	return s.service.GetSASEmoji(ctx, &GetSASEmojiRequest{RemotePeerId: remotePeerID})
}

// ConfirmSASMatch sends the user's SAS verification decision to the remote peer.
func (s *Session) ConfirmSASMatch(ctx context.Context, confirmed bool) error {
	_, err := s.service.ConfirmSASMatch(ctx, &ConfirmSASMatchRequest{Confirmed: confirmed})
	return err
}

// ConfirmPairing finalizes a verified pairing and persists the remote peer.
func (s *Session) ConfirmPairing(ctx context.Context, remotePeerID string, displayName string) error {
	_, err := s.service.ConfirmPairing(ctx, &ConfirmPairingRequest{
		RemotePeerId: remotePeerID,
		DisplayName:  displayName,
	})
	return err
}
