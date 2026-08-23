package provider_spacewave

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_control "github.com/s4wave/spacewave/db/world/control"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
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
		if bootstrap.OwnerAccountID != localAccountID {
			return nil, errors.New("friend dm is not ready and caller is not owner")
		}
		state, err := buildStandaloneSpaceInitState(
			ctx,
			cli,
			a.GetLogger(),
			localAccountID,
			bootstrap.SharedObjectID,
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
	ref := a.buildSharedObjectRef(bootstrap.SharedObjectID)
	swSO, relSO, err := a.mountSpaceSO(ctx, bootstrap.SharedObjectID)
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
		if bootstrap.OwnerAccountID == localAccountID &&
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
			a.GetCurrentSessionPeerID(),
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
	bootstrap *FriendDmBootstrap,
	localAccountID string,
	targetAccountID string,
	localPeerID string,
) error {
	if bootstrap == nil {
		return errors.New("friend dm response is missing")
	}
	if localAccountID == "" || targetAccountID == "" || localAccountID == targetAccountID {
		return errors.New("friend dm requires two distinct accounts")
	}
	if len(bootstrap.Accounts) != 2 {
		return errors.New("friend dm response must contain two accounts")
	}
	accountsByID := make(map[string]FriendDmAccount, len(bootstrap.Accounts))
	for _, account := range bootstrap.Accounts {
		if _, ok := accountsByID[account.AccountID]; ok {
			return errors.New("friend dm response contains duplicate account")
		}
		accountsByID[account.AccountID] = account
		if len(account.Sessions) == 0 {
			return errors.Errorf(
				"friend dm account %s has no active sessions",
				account.AccountID,
			)
		}
		for _, sess := range account.Sessions {
			if _, err := session.ExtractPublicKeyFromPeerID(sess.PeerID); err != nil {
				return errors.Wrapf(err, "parse friend dm peer %s", sess.PeerID)
			}
		}
		if len(account.RecoveryKeypairs) == 0 {
			return errors.Errorf(
				"friend dm account %s has no recovery keypairs",
				account.AccountID,
			)
		}
		for _, recoveryPeer := range account.RecoveryKeypairs {
			if _, err := session.ExtractPublicKeyFromPeerID(recoveryPeer.PeerID); err != nil {
				return errors.Wrapf(
					err,
					"parse friend dm recovery peer %s",
					recoveryPeer.PeerID,
				)
			}
		}
	}
	localAccount, localOK := accountsByID[localAccountID]
	targetAccount, targetOK := accountsByID[targetAccountID]
	if !localOK || !targetOK || len(accountsByID) != 2 {
		return errors.New("friend dm response does not contain the authenticated pair")
	}
	localPeerFound := false
	for _, sess := range localAccount.Sessions {
		if sess.PeerID == localPeerID {
			localPeerFound = true
			break
		}
	}
	if !localPeerFound {
		return errors.New("friend dm response does not contain the authenticated session")
	}
	if bootstrap.OwnerAccountID != localAccountID &&
		bootstrap.OwnerAccountID != targetAccountID {
		return errors.New("friend dm owner is outside account pair")
	}
	expectedID := deriveFriendDmSharedObjectID(localAccount, targetAccount)
	if bootstrap.SharedObjectID != expectedID {
		return errors.Errorf(
			"friend dm response has noncanonical shared object id %q",
			bootstrap.SharedObjectID,
		)
	}
	return nil
}

type friendDmParticipant struct {
	peerID    string
	accountID string
	role      sobject.SOParticipantRole
}

type friendDmParticipantPlan struct {
	removals  []string
	additions []friendDmParticipant
}

func buildFriendDmParticipantPlan(
	current []*sobject.SOParticipantConfig,
	accounts []FriendDmAccount,
	localPeerID string,
) (friendDmParticipantPlan, error) {
	desired := make(map[string]string)
	for _, account := range accounts {
		for _, sess := range account.Sessions {
			if _, ok := desired[sess.PeerID]; ok {
				return friendDmParticipantPlan{}, errors.Errorf(
					"friend dm peer %s appears more than once",
					sess.PeerID,
				)
			}
			desired[sess.PeerID] = account.AccountID
		}
	}

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

	for peerID, accountID := range desired {
		role := sobject.SOParticipantRole_SOParticipantRole_WRITER
		if accountID == localAccountID {
			role = sobject.SOParticipantRole_SOParticipantRole_OWNER
		}
		participant := currentByPeer[peerID]
		if participant != nil &&
			participant.GetEntityId() == accountID &&
			participant.GetRole() == role {
			continue
		}
		plan.additions = append(plan.additions, friendDmParticipant{
			peerID:    peerID,
			accountID: accountID,
			role:      role,
		})
	}
	slices.Sort(plan.removals)
	slices.SortFunc(plan.additions, func(a, b friendDmParticipant) int {
		return strings.Compare(a.peerID, b.peerID)
	})
	return plan, nil
}

func reconcileFriendDmParticipants(
	ctx context.Context,
	swSO *SharedObject,
	accounts []FriendDmAccount,
	localPeerID string,
) error {
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
	parsedPubs := make(map[string]crypto.PubKey, len(plan.additions))
	for _, participant := range plan.additions {
		targetPub, err := session.ExtractPublicKeyFromPeerID(participant.peerID)
		if err != nil {
			return errors.Wrapf(err, "parse friend dm peer %s", participant.peerID)
		}
		parsedPubs[participant.peerID] = targetPub
	}
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
	for _, participant := range plan.additions {
		if _, err := swSO.AddParticipant(
			ctx,
			participant.peerID,
			parsedPubs[participant.peerID],
			participant.role,
			participant.accountID,
		); err != nil {
			return errors.Wrapf(err, "add friend dm participant %s", participant.peerID)
		}
	}
	return nil
}

// friendDmChannelMembers returns the sorted unique participant peer IDs of
// the shared object, which are exactly the local and remote DM peers.
func friendDmChannelMembers(ctx context.Context, swSO *SharedObject, localPeerID peer.ID) ([]string, error) {
	state, err := swSO.GetSOHost().GetHostState(ctx)
	if err != nil {
		return nil, err
	}
	if state.GetConfig() == nil {
		return nil, errors.New("friend dm config is missing")
	}
	seen := make(map[string]struct{}, len(state.GetConfig().GetParticipants()))
	var members []string
	for _, participant := range state.GetConfig().GetParticipants() {
		peerID := participant.GetPeerId()
		if peerID == "" {
			continue
		}
		if _, dup := seen[peerID]; dup {
			continue
		}
		seen[peerID] = struct{}{}
		members = append(members, peerID)
	}
	if localPeerID != "" {
		if _, ok := seen[string(localPeerID)]; !ok {
			members = append(members, string(localPeerID))
		}
	}
	slices.Sort(members)
	if len(members) == 0 {
		return nil, errors.New("friend dm channel has no member peers")
	}
	return members, nil
}

func marshalFriendDmChannelWorldOp(opSender peer.ID, memberPeerIds []string) ([]byte, error) {
	if opSender == "" {
		return nil, errors.New("friend dm channel op sender is required")
	}
	op := &spacewave_chat.CreateChatChannelOp{
		ObjectKey:     FriendDmChannelObjectKey,
		Name:          "Direct Messages",
		Timestamp:     timestamppb.Now(),
		MemberPeerIds: memberPeerIds,
	}
	tx, err := world_block_tx.NewTxApplyWorldOp(op, opSender)
	if err != nil {
		return nil, errors.Wrap(err, "build friend dm channel transaction")
	}
	worldOp := &sobject_world_engine.SOWorldOp{
		Body: &sobject_world_engine.SOWorldOp_ApplyTxOp{
			ApplyTxOp: &sobject_world_engine.ApplyTxOp{Tx: tx},
		},
	}
	opData, err := worldOp.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal friend dm channel world operation")
	}
	return opData, nil
}

func ensureFriendDmChannel(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	ref *sobject.SharedObjectRef,
	swSO *SharedObject,
	opSender peer.ID,
) error {
	mounted, bodyRef, err := space.ExMountSpaceSoBody(ctx, b, ref, false, nil)
	if err != nil {
		return err
	}
	defer bodyRef.Release()

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

	memberPeerIds, err := friendDmChannelMembers(ctx, swSO, opSender)
	if err != nil {
		return errors.Wrap(err, "collect friend dm channel members")
	}
	opData, err := marshalFriendDmChannelWorldOp(opSender, memberPeerIds)
	if err != nil {
		return err
	}
	localID, err := swSO.QueueOperation(ctx, opData)
	if err != nil {
		return errors.Wrap(err, "queue friend dm channel operation")
	}
	_, wasRejected, waitErr := swSO.WaitOperation(ctx, localID)
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
	} else if waitErr != nil {
		return errors.Wrap(waitErr, "create friend dm channel")
	}

	projected, err := world_control.WaitForObjectRev(
		ctx,
		le,
		ws,
		FriendDmChannelObjectKey,
		0,
	)
	if err != nil {
		if waitErr != nil {
			return errors.Wrap(waitErr, "wait for friend dm channel projection")
		}
		return errors.Wrap(err, "wait for friend dm channel projection")
	}
	world.ReleaseObjectState(projected)
	return nil
}
