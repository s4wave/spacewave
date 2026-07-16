package provider_spacewave

import (
	"context"
	"net/url"
	"path"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// FriendDmSession identifies an active session that may join a friend DM.
type FriendDmSession struct {
	PeerID string
}

// FriendDmRecoveryKeypair identifies an entity recovery keypair peer.
type FriendDmRecoveryKeypair struct {
	PeerID string
}

// FriendDmAccount carries the current account generation, active sessions, and
// entity recovery keypair peers.
type FriendDmAccount struct {
	AccountID        string
	EntityUUID       string
	Epoch            uint64
	Sessions         []FriendDmSession
	RecoveryKeypairs []FriendDmRecoveryKeypair
}

// FriendDmBootstrap is the authenticated Cloud response for a friend DM.
type FriendDmBootstrap struct {
	SharedObjectID string
	Ready          bool
	OwnerAccountID string
	OwnerType      string
	Accounts       []FriendDmAccount
}

// ErrFriendDmNotFound indicates that the Cloud endpoint intentionally hid an
// unauthorized, missing, blocked, or otherwise unusable friendship.
var ErrFriendDmNotFound = errors.New("friend dm not found")

// GetFriendDM reads the authenticated friend-DM proposal without mutating
// Cloud or SharedObject state.
func (c *SessionClient) GetFriendDM(
	ctx context.Context,
	targetAccountID string,
) (*FriendDmBootstrap, error) {
	if c == nil {
		return nil, errors.New("session client is required")
	}
	targetAccountID = strings.TrimSpace(targetAccountID)
	if targetAccountID == "" {
		return nil, errors.New("target account id is required")
	}
	data, err := c.doGet(
		ctx,
		path.Join("/api/account/friends", url.PathEscape(targetAccountID), "dm"),
		SeedReasonColdSeed,
	)
	if err != nil {
		return nil, errors.Wrap(err, "get friend dm")
	}
	return parseFriendDmBootstrap(data)
}

// CreateFriendDMWithState publishes complete signed initial state through the
// reserved friend-DM route. The route re-authorizes the friendship and
// derives the canonical SharedObject ID; callers must handle a 409 by
// reloading GetFriendDM.
func (c *SessionClient) CreateFriendDMWithState(
	ctx context.Context,
	targetAccountID string,
	ownerAccountID string,
	configState []byte,
	rootState []byte,
) (*FriendDmBootstrap, error) {
	if c == nil {
		return nil, errors.New("session client is required")
	}
	targetAccountID = strings.TrimSpace(targetAccountID)
	ownerAccountID = strings.TrimSpace(ownerAccountID)
	if targetAccountID == "" || ownerAccountID == "" {
		return nil, errors.New("friend dm account ids are required")
	}
	configRequest := &api.PostConfigStateRequest{}
	if err := configRequest.UnmarshalVT(configState); err != nil {
		return nil, errors.Wrap(err, "unmarshal friend dm config state")
	}
	if len(configRequest.GetConfigChange()) == 0 || configRequest.GetKeyEpoch() == nil {
		return nil, errors.New("friend dm config state wrapper is incomplete")
	}
	rootRequest := &api.PostRootRequest{}
	if err := rootRequest.UnmarshalVT(rootState); err != nil {
		return nil, errors.Wrap(err, "unmarshal friend dm root state")
	}
	if rootRequest.GetRoot() == nil {
		return nil, errors.New("friend dm root state wrapper is incomplete")
	}
	body, err := marshalCreateWithStateBody(
		"Friend DM",
		"space",
		"account",
		ownerAccountID,
		true,
		configState,
		rootState,
	)
	if err != nil {
		return nil, err
	}
	data, err := c.doPost(
		ctx,
		path.Join("/api/account/friends", url.PathEscape(targetAccountID), "dm"),
		"application/json",
		body,
		map[string]string{"Accept": "application/json"},
		SeedReasonMutation,
	)
	if err != nil {
		return nil, errors.Wrap(err, "create friend dm")
	}
	return parseFriendDmBootstrap(data)
}

func parseFriendDmBootstrap(data []byte) (*FriendDmBootstrap, error) {
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		return nil, errors.Wrap(err, "parse friend dm response")
	}
	sharedObjectID := string(value.GetStringBytes("sharedObjectId"))
	if sharedObjectID == "" {
		if value.Exists("found") && !value.GetBool("found") {
			return nil, ErrFriendDmNotFound
		}
		return nil, errors.New("friend dm response missing shared object id")
	}
	readyValue := value.Get("ready")
	if readyValue == nil ||
		(readyValue.Type() != fastjson.TypeTrue && readyValue.Type() != fastjson.TypeFalse) {
		return nil, errors.New("friend dm response has invalid ready state")
	}
	ownerAccountID := string(value.GetStringBytes("ownerAccountId"))
	ownerType := string(value.GetStringBytes("ownerType"))
	if ownerAccountID == "" || ownerType != "account" {
		return nil, errors.New("friend dm response has invalid owner")
	}

	accountValues := value.GetArray("accounts")
	if len(accountValues) != 2 {
		return nil, errors.New("friend dm response must contain two accounts")
	}
	accounts := make([]FriendDmAccount, 0, len(accountValues))
	seenAccounts := make(map[string]struct{}, len(accountValues))
	allSeenPeers := make(map[string]struct{})
	for _, accountValue := range accountValues {
		accountID := string(accountValue.GetStringBytes("accountId"))
		entityUUID := string(accountValue.GetStringBytes("entityUuid"))
		if accountID == "" || entityUUID == "" || !accountValue.Exists("epoch") {
			return nil, errors.New("friend dm response has invalid account")
		}
		if _, ok := seenAccounts[accountID]; ok {
			return nil, errors.New("friend dm response contains duplicate account")
		}
		seenAccounts[accountID] = struct{}{}
		sessionsValue := accountValue.Get("sessions")
		if sessionsValue == nil || sessionsValue.Type() != fastjson.TypeArray {
			return nil, errors.New("friend dm response has invalid sessions")
		}
		sessionValues := sessionsValue.GetArray()
		sessions := make([]FriendDmSession, 0, len(sessionValues))
		seenPeers := make(map[string]struct{}, len(sessionValues))
		for _, sessionValue := range sessionValues {
			peerID := string(sessionValue.GetStringBytes("peerId"))
			if peerID == "" {
				return nil, errors.New("friend dm response has invalid session peer")
			}
			if _, ok := seenPeers[peerID]; ok {
				return nil, errors.New("friend dm response contains duplicate session peer")
			}
			if _, ok := allSeenPeers[peerID]; ok {
				return nil, errors.New("friend dm response contains duplicate session peer")
			}
			seenPeers[peerID] = struct{}{}
			allSeenPeers[peerID] = struct{}{}
			sessions = append(sessions, FriendDmSession{PeerID: peerID})
		}
		recoveryValue := accountValue.Get("recoveryKeypairs")
		if recoveryValue == nil ||
			recoveryValue.Type() != fastjson.TypeArray ||
			len(recoveryValue.GetArray()) == 0 {
			return nil, errors.New("friend dm response has invalid recovery keypairs")
		}
		recoveryKeypairs := make([]FriendDmRecoveryKeypair, 0, len(recoveryValue.GetArray()))
		seenRecoveryPeers := make(map[string]struct{}, len(recoveryKeypairs))
		for _, recoveryValue := range recoveryValue.GetArray() {
			peerID := string(recoveryValue.GetStringBytes("peerId"))
			if peerID == "" {
				return nil, errors.New("friend dm response has invalid recovery peer")
			}
			if _, ok := seenRecoveryPeers[peerID]; ok {
				return nil, errors.New("friend dm response contains duplicate recovery peer")
			}
			seenRecoveryPeers[peerID] = struct{}{}
			recoveryKeypairs = append(recoveryKeypairs, FriendDmRecoveryKeypair{PeerID: peerID})
		}
		accounts = append(accounts, FriendDmAccount{
			AccountID:        accountID,
			EntityUUID:       entityUUID,
			Epoch:            accountValue.GetUint64("epoch"),
			Sessions:         sessions,
			RecoveryKeypairs: recoveryKeypairs,
		})
	}
	if _, ok := seenAccounts[ownerAccountID]; !ok {
		return nil, errors.New("friend dm owner is outside account pair")
	}
	return &FriendDmBootstrap{
		SharedObjectID: sharedObjectID,
		Ready:          value.GetBool("ready"),
		OwnerAccountID: ownerAccountID,
		OwnerType:      ownerType,
		Accounts:       accounts,
	}, nil
}
