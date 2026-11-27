package provider_spacewave

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// EntityClient uses the entity keypair for registration flows.
type EntityClient struct {
	*SignedHTTPClient
}

// NewEntityClient constructs an entityClient from a peer (entity identity).
func NewEntityClient(
	httpCli *http.Client,
	endpoint string,
	signingEnvPfx string,
	p peer.Peer,
) *EntityClient {
	return &EntityClient{
		SignedHTTPClient: &SignedHTTPClient{
			httpCli: httpCli,
			baseURL: endpoint,
			envPfx:  normalizeSigningEnvPrefix(signingEnvPfx),
			signer:  newSessionSigner("", p.GetPeerID(), 0, nil, nil),
		},
	}
}

// NewEntityClientDirect constructs an entityClient with a pre-resolved key and peer ID.
func NewEntityClientDirect(
	httpCli *http.Client,
	endpoint string,
	signingEnvPfx string,
	priv crypto.PrivKey,
	pid peer.ID,
) *EntityClient {
	return &EntityClient{
		SignedHTTPClient: &SignedHTTPClient{
			httpCli: httpCli,
			baseURL: endpoint,
			envPfx:  normalizeSigningEnvPrefix(signingEnvPfx),
			signer:  newSessionSigner("", pid, 0, priv, nil),
		},
	}
}

// NewEntityClientSigner constructs an EntityClient backed by a request-signing callback.
func NewEntityClientSigner(
	httpCli *http.Client,
	endpoint string,
	signingEnvPfx string,
	pid peer.ID,
	sign SigningFunc,
) *EntityClient {
	return &EntityClient{
		SignedHTTPClient: &SignedHTTPClient{
			httpCli: httpCli,
			baseURL: endpoint,
			envPfx:  normalizeSigningEnvPrefix(signingEnvPfx),
			signer:  newSessionSigner("", pid, 0, nil, sign),
		},
	}
}

// initPrivKey initializes the private key from the peer if not already set.
func (c *EntityClient) initPrivKey(ctx context.Context, p peer.Peer) error {
	// Load the entity key from the peer once.
	if c.signer.PrivKey() != nil {
		return nil
	}

	// Resolve the key before publishing the signer.
	priv, err := p.GetPrivKey(ctx)
	if err != nil {
		return errors.Wrap(err, "get entity private key")
	}
	c.signer = newSessionSigner("", c.signer.GetPeerID(), 0, priv, nil)
	return nil
}

// RegisterAccount registers an account with the Spacewave cloud.
//
// Returns the server-generated account ID.
func (c *EntityClient) RegisterAccount(ctx context.Context, entityID, authMethod string, authParams []byte, turnstileToken string) (string, error) {
	// Require the entity key that signs the registration.
	priv := c.signer.PrivKey()
	if priv == nil {
		return "", errors.New("no private key configured")
	}

	// Encode the account with its first entity keypair.
	req := &api.RegisterAccountRequest{
		EntityId: entityID,
		Keypairs: []*session.EntityKeypair{{
			PeerId:     c.signer.GetPeerID().String(),
			AuthMethod: authMethod,
			AuthParams: authParams,
		}},
	}
	body, err := req.MarshalVT()
	if err != nil {
		return "", errors.Wrap(err, "marshal register request")
	}

	// Register the account.
	respBody, err := c.doPostBinary(ctx, "/api/account/register", body, registrationHeaders(turnstileToken), SeedReasonMutation)
	if err != nil {
		return "", errors.Wrap(err, "register account")
	}

	// Read the server-generated account ID.
	var resp api.RegisterAccountResponse
	if err := resp.UnmarshalVT(respBody); err != nil {
		return "", errors.Wrap(err, "unmarshal register response")
	}
	if resp.GetAccountId() == "" {
		return "", errors.New("server response missing account_id")
	}
	return resp.GetAccountId(), nil
}

// RegisterSession registers a session with the Spacewave cloud.
func (c *EntityClient) RegisterSession(ctx context.Context, p peer.Peer, sessionPeerID string, deviceInfo string) error {
	if err := c.initPrivKey(ctx, p); err != nil {
		return err
	}

	return c.RegisterSessionDirect(ctx, sessionPeerID, deviceInfo)
}

// RegisterSessionDirect registers a session without requiring a peer.Peer.
// The private key must already be set on the client.
func (c *EntityClient) RegisterSessionDirect(ctx context.Context, sessionPeerID, deviceInfo string) error {
	_, err := c.RegisterSessionDirectWithResponse(ctx, sessionPeerID, deviceInfo, "", "")
	return err
}

// RegisterSessionDirectWithResponse registers a session and returns the
// response, which includes the account ID. The private key must already be set.
func (c *EntityClient) RegisterSessionDirectWithResponse(ctx context.Context, sessionPeerID, deviceInfo, entityID, turnstileToken string) (*api.RegisterSessionResponse, error) {
	req := &api.RegisterSessionRequest{
		SessionPeerId: sessionPeerID,
		DeviceInfo:    deviceInfo,
		EntityId:      entityID,
	}
	return c.RegisterSessionWithRequest(ctx, req, turnstileToken)
}

// RegisterSessionWithRequest registers a session using the generated Cloud
// request shape. Existing callers should leave Type unset for USER defaults.
// SpaceLink approval can set APP/DEVICE type, label, and future request fields
// without adding another positional helper.
func (c *EntityClient) RegisterSessionWithRequest(ctx context.Context, req *api.RegisterSessionRequest, turnstileToken string) (*api.RegisterSessionResponse, error) {
	// Encode the registration request.
	if req == nil {
		return nil, errors.New("session registration request is required")
	}
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal session request")
	}

	// Register the session, mapping unknown entity and keypair errors.
	respBody, err := c.doPostBinary(ctx, "/api/account/session/register", body, registrationHeaders(turnstileToken), SeedReasonMutation)
	if err != nil {
		var ce *cloudError
		if errors.As(err, &ce) {
			switch ce.Code {
			case "unknown_entity":
				return nil, ErrUnknownEntity
			case "unknown_keypair":
				return nil, ErrUnknownKeypair
			}
		}
		return nil, errors.Wrap(err, "register session")
	}

	// Decode the registration response.
	var resp api.RegisterSessionResponse
	if err := resp.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal session response")
	}
	return &resp, nil
}

// registrationHeaders returns the Turnstile token and device type headers of
// an account or session registration, omitting the ones that are unset.
func registrationHeaders(turnstileToken string) map[string]string {
	headers := make(map[string]string)
	if turnstileToken != "" {
		headers["X-Turnstile-Token"] = turnstileToken
	}
	if deviceTypeValue != "" {
		headers["X-Device-Type"] = deviceTypeValue
	}
	return headers
}

// RollbackSessionRegistration removes a just-created APP/DEVICE registration
// row through the Cloud registration rollback endpoint. Reused rows and USER
// sessions are preserved by the Cloud endpoint.
func (c *EntityClient) RollbackSessionRegistration(ctx context.Context, sessionPeerID string) error {
	_, err := c.doDelete(ctx, "/api/account/session/"+url.PathEscape(sessionPeerID)+"/registration", SeedReasonMutation)
	if err != nil {
		return errors.Wrap(err, "rollback session registration")
	}
	return nil
}

// signMultiSig signs envelope bytes with each entity key using the multi-sig
// signing context.
func (c *EntityClient) signMultiSig(
	envelope []byte,
	keys []crypto.PrivKey,
	peerIDs []string,
) ([]*api.EntitySignature, error) {
	// Require one peer ID per signing key.
	if len(keys) != len(peerIDs) {
		return nil, errors.New("keys and peerIDs length mismatch")
	}

	// Sign the time-stamped payload with each key.
	now := timestamppb.New(time.Now().Truncate(time.Millisecond))
	payload := BuildMultiSigPayload(now, envelope)
	sigs := make([]*api.EntitySignature, len(keys))
	for i, key := range keys {
		sig, err := key.Sign(payload)
		if err != nil {
			return nil, errors.Wrap(err, "sign envelope")
		}
		sigs[i] = &api.EntitySignature{
			PeerId:    peerIDs[i],
			Signature: sig,
			SignedAt:  now,
		}
	}
	return sigs, nil
}

// doMultiSig builds, signs, and sends a typed multi-sig envelope to the given
// path. Returns the parsed MultiSigActionResponse envelope.
func (c *EntityClient) doMultiSig(
	ctx context.Context,
	method string,
	accountID string,
	reqPath string,
	kind api.MultiSigActionKind,
	payload []byte,
	keys []crypto.PrivKey,
	peerIDs []string,
) (*api.MultiSigActionResponse, error) {
	// Encode the envelope and sign it with each entity key.
	envelope := &api.MultiSigActionEnvelope{
		AccountId: accountID,
		Kind:      kind,
		Method:    method,
		Path:      reqPath,
		Payload:   payload,
	}
	envBytes, err := envelope.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal multi-sig envelope")
	}
	sigs, err := c.signMultiSig(envBytes, keys, peerIDs)
	if err != nil {
		return nil, err
	}

	// Send the envelope with its signatures.
	body, err := (&api.MultiSigRequest{Envelope: envBytes, Signatures: sigs}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal multi-sig request")
	}
	return c.sendMultiSig(ctx, method, reqPath, body, "")
}

// postMultiSig builds, signs, and posts a typed multi-sig envelope to the
// given path. Returns the parsed MultiSigActionResponse envelope.
func (c *EntityClient) postMultiSig(
	ctx context.Context,
	accountID string,
	reqPath string,
	kind api.MultiSigActionKind,
	payload []byte,
	keys []crypto.PrivKey,
	peerIDs []string,
) (*api.MultiSigActionResponse, error) {
	return c.doMultiSig(ctx, http.MethodPost, accountID, reqPath, kind, payload, keys, peerIDs)
}

// AddKeypair adds an entity keypair to the account and returns the per-action
// result.
func (c *EntityClient) AddKeypair(
	ctx context.Context,
	accountID string,
	keypair *session.EntityKeypair,
	entityKeys []crypto.PrivKey,
	entityPeerIDs []string,
) (*api.KeypairAddResult, error) {
	// Encode the action and send it with the entity signatures.
	payload, err := (&api.AddKeypairAction{Keypair: keypair}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal add keypair action")
	}
	resp, err := c.postMultiSig(
		ctx,
		accountID,
		path.Join("/api/account", accountID, "keypair", "add"),
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_ADD_KEYPAIR,
		payload,
		entityKeys,
		entityPeerIDs,
	)
	if err != nil {
		return nil, err
	}

	// Return the per-action result.
	result := resp.GetKeypairAdd()
	if result == nil {
		return nil, errors.New("multi-sig response missing keypair add result")
	}
	return result, nil
}

// RemoveKeypair removes an entity keypair from the account and returns the
// per-action result.
func (c *EntityClient) RemoveKeypair(
	ctx context.Context,
	accountID string,
	peerIDToRemove string,
	entityKeys []crypto.PrivKey,
	entityPeerIDs []string,
) (*api.KeypairRemoveResult, error) {
	// Encode the action and send it with the entity signatures.
	payload, err := (&api.RemoveKeypairAction{PeerId: peerIDToRemove}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal remove keypair action")
	}
	resp, err := c.postMultiSig(
		ctx,
		accountID,
		path.Join("/api/account", accountID, "keypair", "remove"),
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_REMOVE_KEYPAIR,
		payload,
		entityKeys,
		entityPeerIDs,
	)
	if err != nil {
		return nil, err
	}

	// Return the per-action result.
	result := resp.GetKeypairRemove()
	if result == nil {
		return nil, errors.New("multi-sig response missing keypair remove result")
	}
	return result, nil
}

// UpdateThreshold updates the auth threshold for the account and returns the
// per-action result.
func (c *EntityClient) UpdateThreshold(
	ctx context.Context,
	accountID string,
	threshold uint32,
	entityKeys []crypto.PrivKey,
	entityPeerIDs []string,
) (*api.ThresholdChangeResult, error) {
	// Encode the action and send it with the entity signatures.
	payload, err := (&api.UpdateThresholdAction{Threshold: threshold}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal update threshold action")
	}
	resp, err := c.postMultiSig(
		ctx,
		accountID,
		path.Join("/api/account", accountID, "threshold"),
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_UPDATE_THRESHOLD,
		payload,
		entityKeys,
		entityPeerIDs,
	)
	if err != nil {
		return nil, err
	}

	// Return the per-action result.
	result := resp.GetThresholdChange()
	if result == nil {
		return nil, errors.New("multi-sig response missing threshold change result")
	}
	return result, nil
}

// RevokeSession revokes a session by peer ID and returns the per-action result.
func (c *EntityClient) RevokeSession(
	ctx context.Context,
	accountID string,
	sessionPeerID string,
	entityKeys []crypto.PrivKey,
	entityPeerIDs []string,
) (*api.SessionRevokeResult, error) {
	// Encode the action and send it with the entity signatures.
	payload, err := (&api.RevokeSessionAction{SessionPeerId: sessionPeerID}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal revoke session action")
	}
	resp, err := c.doMultiSig(
		ctx,
		http.MethodDelete,
		accountID,
		path.Join("/api/account", accountID, "session", sessionPeerID),
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_REVOKE_SESSION,
		payload,
		entityKeys,
		entityPeerIDs,
	)
	if err != nil {
		return nil, err
	}

	// Return the per-action result.
	result := resp.GetSessionRevoke()
	if result == nil {
		return nil, errors.New("multi-sig response missing session revoke result")
	}
	return result, nil
}

// DeleteAccount sends a signed account deletion request to the cloud and
// returns the per-action result.
func (c *EntityClient) DeleteAccount(
	ctx context.Context,
	accountID string,
	entityKeys []crypto.PrivKey,
	entityPeerIDs []string,
) (*api.AccountDeleteResult, error) {
	// Encode the action and send it with the entity signatures.
	payload, err := (&api.DeleteAccountAction{}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal delete account action")
	}
	resp, err := c.doMultiSig(
		ctx,
		http.MethodDelete,
		accountID,
		path.Join("/api/account", accountID, "delete"),
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_DELETE_ACCOUNT,
		payload,
		entityKeys,
		entityPeerIDs,
	)
	if err != nil {
		return nil, err
	}

	// Return the per-action result.
	result := resp.GetAccountDelete()
	if result == nil {
		return nil, errors.New("multi-sig response missing account delete result")
	}
	return result, nil
}
