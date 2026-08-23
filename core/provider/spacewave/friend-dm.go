package provider_spacewave

import (
	"context"
	"net/url"
	"path"
	"strings"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// ErrFriendDmNotFound indicates that the Cloud endpoint intentionally hid an
// unauthorized, missing, blocked, or otherwise unusable friendship.
var ErrFriendDmNotFound = errors.New("friend dm not found")

// GetFriendDM reads the authenticated friend-DM proposal without mutating
// Cloud or SharedObject state.
func (c *SessionClient) GetFriendDM(
	ctx context.Context,
	targetAccountID string,
) (*api.GetFriendDmResponse, error) {
	if c == nil {
		return nil, errors.New("session client is required")
	}
	targetAccountID = strings.TrimSpace(targetAccountID)
	if targetAccountID == "" {
		return nil, errors.New("target account id is required")
	}
	data, err := c.doGetBinary(
		ctx,
		path.Join("/api/account/friends", url.PathEscape(targetAccountID), "dm"),
		SeedReasonColdSeed,
	)
	if err != nil {
		return nil, errors.Wrap(err, "get friend dm")
	}
	return decodeFriendDmBootstrap(data)
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
) (*api.GetFriendDmResponse, error) {
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
	body, err := (&api.CreateWithStateRequest{
		DisplayName:    "Friend DM",
		ObjectType:     "space",
		OwnerType:      "account",
		OwnerId:        ownerAccountID,
		AccountPrivate: true,
		ConfigState:    configState,
		RootState:      rootState,
	}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal create request")
	}
	data, err := c.doPostBinary(
		ctx,
		path.Join("/api/account/friends", url.PathEscape(targetAccountID), "dm"),
		body,
		nil,
		SeedReasonMutation,
	)
	if err != nil {
		return nil, errors.Wrap(err, "create friend dm")
	}
	return decodeFriendDmBootstrap(data)
}

// decodeFriendDmBootstrap decodes and sanity-checks a friend-DM bootstrap
// response.
func decodeFriendDmBootstrap(data []byte) (*api.GetFriendDmResponse, error) {
	resp := &api.GetFriendDmResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal friend dm response")
	}
	if !resp.GetFound() {
		return nil, ErrFriendDmNotFound
	}
	if resp.GetSharedObjectId() == "" {
		return nil, errors.New("friend dm response missing shared object id")
	}
	if resp.GetOwnerAccountId() == "" || resp.GetOwnerType() != "account" {
		return nil, errors.New("friend dm response has invalid owner")
	}
	return resp, nil
}
