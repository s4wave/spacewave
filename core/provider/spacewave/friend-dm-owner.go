package provider_spacewave

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_control "github.com/s4wave/spacewave/db/world/control"
	"github.com/s4wave/spacewave/net/crypto"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	"github.com/sirupsen/logrus"
)

// FriendDmChannelObjectKey is the one canonical ChatChannel object key in a
// friend DM Space. Each DM has its own World, so the key is deterministic per
// Space without creating a second transcript.
const FriendDmChannelObjectKey = "chat/channel/dm"

// FriendDmOpenResult contains the mounted-space identity for a friend DM.
type FriendDmOpenResult struct {
	SharedObjectRef  *sobject.SharedObjectRef
	SharedObjectMeta *sobject.SharedObjectMeta
}

// OpenFriendDM authorizes, creates, or opens the canonical friend DM Space.
// Only the Cloud-selected account owner mutates participant grants; every
// authenticated writer or owner ensures the deterministic channel.
func (a *ProviderAccount) OpenFriendDM(
	ctx context.Context,
	targetAccountID string,
) (*FriendDmOpenResult, error) {
	// Resolve the authenticated cloud client.
	cli := a.GetSessionClient()
	if cli == nil {
		return nil, errors.New("session client not available")
	}

	// Load and validate the friend-DM bootstrap record.
	targetAccountID = strings.TrimSpace(targetAccountID)
	bootstrap, err := cli.GetFriendDM(ctx, targetAccountID)
	if err != nil {
		return nil, err
	}

	// Resolve local account and session identity.
	localAccountID := a.GetAccountID()
	localPeerID := a.GetCurrentSessionPeerID().String()
	if err := validateFriendDmBootstrap(
		bootstrap,
		localAccountID,
		targetAccountID,
		localPeerID,
	); err != nil {
		return nil, err
	}

	// Create the canonical Space when bootstrap is not ready.
	if !bootstrap.Ready {
		if bootstrap.OwnerAccountId != localAccountID {
			return nil, errors.New("friend dm is not ready and caller is not owner")
		}
		state, err := buildStandaloneSpaceInitState(
			ctx,
			cli,
			a.GetLogger(),
			localAccountID,
			"",
			bootstrap.SharedObjectId,
			cli.priv,
			buildStandaloneSpaceInitStepFactorySet(),
			true,
			bootstrap.Accounts,
		)
		if err != nil {
			return nil, errors.Wrap(err, "build friend dm initial state")
		}
		configState, rootState, err := marshalFriendDmInitialState(state)
		if err != nil {
			return nil, errors.Wrap(err, "marshal friend dm initial state")
		}
		created, err := cli.CreateFriendDMWithState(
			ctx,
			targetAccountID,
			localAccountID,
			configState,
			rootState,
		)
		conflicted := false
		if err != nil {
			var cloudErr *cloudError
			if !errors.As(err, &cloudErr) ||
				cloudErr.StatusCode != http.StatusConflict {
				return nil, err
			}
			conflicted = true
			created, err = cli.GetFriendDM(ctx, targetAccountID)
			if err != nil {
				return nil, errors.Wrap(err, "reload friend dm after conflict")
			}
		}
		if err := validateFriendDmBootstrap(
			created,
			localAccountID,
			targetAccountID,
			localPeerID,
		); err != nil {
			return nil, err
		}
		if !created.Ready {
			readyErr := "friend dm create did not produce ready state"
			if conflicted {
				readyErr = "friend dm conflict did not produce ready state"
			}
			return nil, errors.New(readyErr)
		}
		bootstrap = created
	}
	if !bootstrap.Ready {
		return nil, errors.New("friend dm is not ready")
	}

	// Mount the canonical shared object.
	ref := a.buildSharedObjectRef(bootstrap.SharedObjectId)
	swSO, relSO, err := a.mountSpaceSO(ctx, bootstrap.SharedObjectId)
	if err != nil {
		return nil, err
	}
	defer relSO()

	// Read participant state before applying owner-authorized actions.
	state, err := swSO.GetSOHost().GetHostState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "read friend dm state")
	}

	// Reconcile grants and the deterministic channel for writable participants.
	localParticipant := participantConfigForPeer(state.GetConfig(), localPeerID)
	if localParticipant != nil && sobject.CanWriteOps(localParticipant.GetRole()) {
		if bootstrap.OwnerAccountId == localAccountID &&
			sobject.IsOwner(localParticipant.GetRole()) {
			if err := reconcileFriendDmParticipants(
				ctx,
				swSO,
				bootstrap.Accounts,
				localPeerID,
			); err != nil {
				return nil, errors.Wrap(err, "reconcile friend dm participants")
			}
		}
		if err := ensureFriendDmChannel(
			ctx,
			a.GetLogger(),
			a.p.b,
			ref,
			swSO,
		); err != nil {
			return nil, errors.Wrap(err, "ensure friend dm channel")
		}
	}

	// Build the shared-object metadata response.
	meta, err := space.NewSharedObjectMeta("Friend DM")
	if err != nil {
		return nil, errors.Wrap(err, "build friend dm metadata")
	}

	// Return the mounted Space identity.
	return &FriendDmOpenResult{
		SharedObjectRef:  ref,
		SharedObjectMeta: meta,
	}, nil
}

func validateFriendDmBootstrap(
	bootstrap *api.GetFriendDmResponse,
	localAccountID string,
	targetAccountID string,
	localPeerID string,
) error {
	// Require a bootstrap response with two distinct accounts.
	if bootstrap == nil {
		return errors.New("friend dm response is missing")
	}
	if localAccountID == "" || targetAccountID == "" || localAccountID == targetAccountID {
		return errors.New("friend dm requires two distinct accounts")
	}
	if len(bootstrap.Accounts) != 2 {
		return errors.New("friend dm response must contain two accounts")
	}

	// Validate each account's sessions and recovery peers for uniqueness.
	accountsByID := make(map[string]*api.FriendDmAccount, len(bootstrap.Accounts))
	sessionPeers := make(map[string]struct{})
	recoveryPeers := make(map[string]struct{})
	for _, account := range bootstrap.Accounts {
		if _, ok := accountsByID[account.GetAccountId()]; ok {
			return errors.New("friend dm response contains duplicate account")
		}
		accountsByID[account.GetAccountId()] = account
		if len(account.Sessions) == 0 {
			return errors.Errorf(
				"friend dm account %s has no active sessions",
				account.GetAccountId(),
			)
		}
		for _, sess := range account.Sessions {
			if _, ok := sessionPeers[sess.GetPeerId()]; ok {
				return errors.Errorf(
					"friend dm response contains duplicate session peer %s",
					sess.GetPeerId(),
				)
			}
			sessionPeers[sess.GetPeerId()] = struct{}{}
			if _, err := session.ExtractPublicKeyFromPeerID(sess.GetPeerId()); err != nil {
				return errors.Wrapf(err, "parse friend dm peer %s", sess.GetPeerId())
			}
		}
		if len(account.RecoveryKeypairs) == 0 {
			return errors.Errorf(
				"friend dm account %s has no recovery keypairs",
				account.GetAccountId(),
			)
		}
		for _, recoveryPeer := range account.RecoveryKeypairs {
			if _, ok := recoveryPeers[recoveryPeer.GetPeerId()]; ok {
				return errors.Errorf(
					"friend dm response contains duplicate recovery peer %s",
					recoveryPeer.GetPeerId(),
				)
			}
			recoveryPeers[recoveryPeer.GetPeerId()] = struct{}{}
			if _, err := session.ExtractPublicKeyFromPeerID(recoveryPeer.GetPeerId()); err != nil {
				return errors.Wrapf(
					err,
					"parse friend dm recovery peer %s",
					recoveryPeer.GetPeerId(),
				)
			}
		}
	}

	// Require both authenticated accounts and the local session in the response.
	localAccount, localOK := accountsByID[localAccountID]
	targetAccount, targetOK := accountsByID[targetAccountID]
	if !localOK || !targetOK || len(accountsByID) != 2 {
		return errors.New("friend dm response does not contain the authenticated pair")
	}
	localPeerFound := false
	for _, sess := range localAccount.Sessions {
		if sess.GetPeerId() == localPeerID {
			localPeerFound = true
			break
		}
	}
	if !localPeerFound {
		return errors.New("friend dm response does not contain the authenticated session")
	}

	// Verify the owner account and canonical shared object ID.
	if bootstrap.OwnerAccountId != localAccountID &&
		bootstrap.OwnerAccountId != targetAccountID {
		return errors.New("friend dm owner is outside account pair")
	}
	expectedID := deriveFriendDmSharedObjectID(localAccount, targetAccount)
	if bootstrap.SharedObjectId != expectedID {
		return errors.Errorf(
			"friend dm response has noncanonical shared object id %q",
			bootstrap.SharedObjectId,
		)
	}
	return nil
}

type friendDmParticipant struct {
	peerID    string
	accountID string
	username  string
	role      sobject.SOParticipantRole
}

type friendDmParticipantPlan struct {
	removals  []string
	additions []friendDmParticipant
}

func buildFriendDmParticipantPlan(
	current []*sobject.SOParticipantConfig,
	accounts []*api.FriendDmAccount,
	localPeerID string,
) (friendDmParticipantPlan, error) {
	// Map each active session peer to its account and each account to its
	// username, rejecting duplicate peers.
	desired := make(map[string]string)
	usernames := make(map[string]string, len(accounts))
	for _, account := range accounts {
		usernames[account.GetAccountId()] = account.GetEntityId()
		for _, sess := range account.Sessions {
			if _, ok := desired[sess.GetPeerId()]; ok {
				return friendDmParticipantPlan{}, errors.Errorf(
					"friend dm peer %s appears more than once",
					sess.GetPeerId(),
				)
			}
			desired[sess.GetPeerId()] = account.GetAccountId()
		}
	}

	// Index current participants and verify the local owner is authoritative.
	currentByPeer := make(map[string]*sobject.SOParticipantConfig, len(current))
	for _, participant := range current {
		currentByPeer[participant.GetPeerId()] = participant
	}
	localAccountID, localPresent := desired[localPeerID]
	if !localPresent {
		return friendDmParticipantPlan{}, errors.New("local owner peer is not an active session")
	}
	if participant := currentByPeer[localPeerID]; participant == nil ||
		participant.GetEntityId() != localAccountID ||
		!sobject.IsOwner(participant.GetRole()) {
		return friendDmParticipantPlan{}, errors.New("local owner participant is not authoritative")
	}

	// Remove current participants that no longer match the desired plan.
	plan := friendDmParticipantPlan{}
	for _, participant := range current {
		accountID, ok := desired[participant.GetPeerId()]
		if participant.GetPeerId() == localPeerID && (!ok || accountID != localAccountID) {
			return friendDmParticipantPlan{}, errors.New("cannot remove local owner participant")
		}
		if ok && accountID == participant.GetEntityId() {
			desiredRole := sobject.SOParticipantRole_SOParticipantRole_WRITER
			if accountID == localAccountID {
				desiredRole = sobject.SOParticipantRole_SOParticipantRole_OWNER
			}
			if participant.GetRole() == desiredRole {
				continue
			}
		}
		plan.removals = append(plan.removals, participant.GetPeerId())
	}

	// Add desired participants missing or changed in the current config.
	for peerID, accountID := range desired {
		role := sobject.SOParticipantRole_SOParticipantRole_WRITER
		if accountID == localAccountID {
			role = sobject.SOParticipantRole_SOParticipantRole_OWNER
		}
		participant := currentByPeer[peerID]
		if participant != nil &&
			participant.GetEntityId() == accountID &&
			participant.GetRole() == role &&
			participant.GetUsername() == usernames[accountID] {
			continue
		}
		plan.additions = append(plan.additions, friendDmParticipant{
			peerID:    peerID,
			accountID: accountID,
			username:  usernames[accountID],
			role:      role,
		})
	}

	// Return the plan with deterministic ordering.
	slices.Sort(plan.removals)
	slices.SortFunc(plan.additions, func(a, b friendDmParticipant) int {
		return strings.Compare(a.peerID, b.peerID)
	})
	return plan, nil
}

func reconcileFriendDmParticipants(
	ctx context.Context,
	swSO *SharedObject,
	accounts []*api.FriendDmAccount,
	localPeerID string,
) error {
	// Read the verified config and build the participant plan.
	state, err := swSO.GetSOHost().GetHostState(ctx)
	if err != nil {
		return err
	}
	if state.GetConfig() == nil {
		return errors.New("friend dm config is missing")
	}
	plan, err := buildFriendDmParticipantPlan(
		state.GetConfig().GetParticipants(),
		accounts,
		localPeerID,
	)
	if err != nil {
		return err
	}

	// Parse public keys for the participants being added.
	parsedPubs := make(map[string]crypto.PubKey, len(plan.additions))
	for _, participant := range plan.additions {
		targetPub, err := session.ExtractPublicKeyFromPeerID(participant.peerID)
		if err != nil {
			return errors.Wrapf(err, "parse friend dm peer %s", participant.peerID)
		}
		parsedPubs[participant.peerID] = targetPub
	}

	// Remove stale participants with an owner revocation.
	for _, peerID := range plan.removals {
		if _, err := swSO.RemoveParticipantWithRevocation(
			ctx,
			peerID,
			&sobject.SORevocationInfo{
				Reason: sobject.SORevocationReason_SO_REVOCATION_REASON_OWNER_REMOVED,
			},
		); err != nil {
			return errors.Wrapf(err, "remove stale participant %s", peerID)
		}
	}

	// Add each planned participant with its role and account.
	for _, participant := range plan.additions {
		if _, err := swSO.AddParticipant(
			ctx,
			participant.peerID,
			parsedPubs[participant.peerID],
			participant.role,
			participant.accountID,
			participant.username,
		); err != nil {
			return errors.Wrapf(err, "add friend dm participant %s", participant.peerID)
		}
	}
	return nil
}

// marshalFriendDmChannelWorldOp builds the channel creation operation. It is
// an authenticated World operation, so replay takes the sender from the
// verified SharedObject signer rather than the transaction. storageGeneration
// is the accepted storage generation of the World.
func marshalFriendDmChannelWorldOp(storageGeneration uint64) ([]byte, error) {
	// Build the transaction that creates the channel.
	op := &spacewave_chat.CreateChatChannelOp{
		ObjectKey: FriendDmChannelObjectKey,
		Name:      "Direct Messages",
		Timestamp: timestamppb.Now(),
	}
	tx, err := world_block_tx.NewTxApplyWorldOp(op, "")
	if err != nil {
		return nil, errors.Wrap(err, "build friend dm channel transaction")
	}

	// Wrap it in a World operation on storageGeneration.
	worldOp := &sobject_world_engine.SOWorldOp{
		Body: &sobject_world_engine.SOWorldOp_ApplyTxOp{
			ApplyTxOp: &sobject_world_engine.ApplyTxOp{Tx: tx, StorageGeneration: storageGeneration},
		},
	}
	opData, err := worldOp.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal friend dm channel world operation")
	}
	return opData, nil
}

// ensureFriendDmChannel creates the direct message channel when the Space
// World lacks it, and waits for the World to project the channel.
func ensureFriendDmChannel(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	ref *sobject.SharedObjectRef,
	swSO *SharedObject,
) error {
	// Mount the Space body to read its World.
	mounted, bodyRef, err := space.ExMountSpaceSoBody(ctx, b, ref, false, nil)
	if err != nil {
		return err
	}
	defer bodyRef.Release()

	// Skip creation when the channel exists.
	ws := world.NewEngineWorldState(
		mounted.GetSharedObjectBody().GetWorldEngine(),
		false,
	)
	found, err := ws.HasObject(ctx, FriendDmChannelObjectKey)
	if err != nil {
		return errors.Wrap(err, "check friend dm channel")
	}
	if found {
		return nil
	}

	// Build the creation on the accepted storage generation.
	snap, err := swSO.GetSharedObjectState(ctx)
	if err != nil {
		return err
	}
	state, err := sobject_world_engine.ReadInnerState(ctx, snap)
	if err != nil {
		return err
	}
	opData, err := marshalFriendDmChannelWorldOp(state.GetStorageGeneration())
	if err != nil {
		return err
	}

	// Queue it and wait for the decision. A rejection is fine when another
	// device created the channel first.
	localID, err := swSO.QueueOperation(ctx, opData)
	if err != nil {
		return errors.Wrap(err, "queue friend dm channel operation")
	}
	_, wasRejected, waitErr := swSO.WaitOperation(ctx, localID)
	if !wasRejected && waitErr != nil {
		return errors.Wrap(waitErr, "create friend dm channel")
	}
	if wasRejected {
		if err := swSO.ClearOperationResult(ctx, localID); err != nil {
			return errors.Wrap(err, "clear friend dm channel rejection")
		}
		found, err := ws.HasObject(ctx, FriendDmChannelObjectKey)
		if err != nil {
			return errors.Wrap(err, "recheck friend dm channel after rejection")
		}
		if found {
			return nil
		}
	}

	// Wait for the World to project the channel.
	projected, err := world_control.WaitForObjectRev(
		ctx,
		le,
		ws,
		FriendDmChannelObjectKey,
		0,
	)
	world.ReleaseObjectState(projected)
	if err != nil {
		if waitErr != nil {
			return errors.Wrap(waitErr, "wait for friend dm channel projection")
		}
		return errors.Wrap(err, "wait for friend dm channel projection")
	}
	return nil
}
