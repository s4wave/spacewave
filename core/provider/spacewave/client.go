package provider_spacewave

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	"github.com/s4wave/spacewave/core/provider/spacewave/entitykeystore"
	"github.com/s4wave/spacewave/core/provider/spacewave/syncprogress"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/httpclient"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// maxResponseBodySize is the maximum HTTP response body size (10 MiB).
const maxResponseBodySize = 10 * 1024 * 1024

const targetedInvitationSignatureContext = "spacewave targeted invitation envelope v1"

// readResponseBody reads an HTTP response body with a size limit.
// Returns an error if the body exceeds maxResponseBodySize.
func readResponseBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize+1))
	if err != nil {
		return nil, errors.Wrap(err, "read response body")
	}
	if int64(len(body)) > maxResponseBodySize {
		return nil, errors.New("response body exceeds maximum size")
	}
	return body, nil
}

type cloudError = clouderror.Error

// parseCloudResponseError parses a cloud API error response and retry hints.
func parseCloudResponseError(resp *http.Response, body []byte) *cloudError {
	return clouderror.ParseResponse(resp, body)
}

// isNonRetryableCloudError checks if an error is a non-retryable cloud error.
func isNonRetryableCloudError(err error) bool {
	return clouderror.IsNonRetryable(err)
}

// isUnauthCloudError checks if an error indicates a stale session key
// (recoverable via reauthentication) as opposed to a deleted account.
func isUnauthCloudError(err error) bool {
	return clouderror.IsUnauth(err)
}

// isAccountDeletedCloudError checks if an error indicates the account is gone
// and the deletion cascade (GC sweep, status flip, routine restart) should
// run. This is strictly narrower than isNonRetryableCloudError: only codes
// in deletedCodes qualify, not arbitrary non-retryable responses.
func isAccountDeletedCloudError(err error) bool {
	return clouderror.IsAccountDeleted(err)
}

// isBlockedCloudError checks if an error indicates a resource is blocked
// (e.g. DMCA takedown). These errors are permanent until the user manually
// retries after the block is lifted.
func isBlockedCloudError(err error) bool {
	return clouderror.IsBlocked(err)
}

// isCloudAccessGatedError checks if a cloud error depends on account/resource
// access state and should wait for an invalidation instead of retrying.
func isCloudAccessGatedError(err error) bool {
	return clouderror.IsAccessGated(err)
}

// isDirtySyncGatedCloudError checks if dirty sync should idle for access state.
func isDirtySyncGatedCloudError(err error) bool {
	return isCloudAccessGatedError(err)
}

// IsCloudErrorStatus returns true when err is a cloud error with the given HTTP status.
func IsCloudErrorStatus(err error, statusCode int) bool {
	return clouderror.IsStatus(err, statusCode)
}

// syncPushPack describes one pack upload to a block store.
type syncPushPack struct {
	// packID is the content-derived pack ID.
	packID string
	// blockCount is the number of blocks in the pack.
	blockCount int
	// bodyHash is the SHA-256 digest of the pack bytes.
	bodyHash []byte
	// bloomFilter is the serialized pack bloom filter.
	bloomFilter []byte
	// bloomFormatVersion identifies the bloom encoding.
	bloomFormatVersion uint32
	// replacedPackIDs are committed packs the upload supersedes atomically.
	// Empty for an ordinary push.
	replacedPackIDs []string
}

// signedHeaders is the list of headers that are signed when present on a request.
var signedHeaders = []string{
	"content-type",
	"x-block-count",
	"x-bloom-filter",
	"x-pack-id",
}

// SigningFunc signs a Spacewave request payload for peerID.
type SigningFunc func(ctx context.Context, payload []byte) ([]byte, error)

// SignedHTTPClient is the base layer for Ed25519-signed HTTP requests.
type SignedHTTPClient struct {
	// httpCli is the underlying HTTP client
	httpCli *http.Client
	// baseURL is the API base URL
	baseURL string
	// envPfx is the environment prefix for signed payloads
	envPfx string
	// priv is the Ed25519 private key for signing
	priv crypto.PrivKey
	// peerID is the peer ID (base58 encoded for headers)
	peerID peer.ID
	// sign signs payloads when the private key is held elsewhere.
	sign SigningFunc
}

// signRequest signs an HTTP request with the Ed25519 private key.
func (c *SignedHTTPClient) signRequest(req *http.Request, body []byte) error {
	h := sha256.Sum256(body)
	return c.signRequestPrecomputed(req, h[:], int64(len(body)))
}

// signRequestPrecomputed signs an HTTP request using a pre-computed body hash and content length.
func (c *SignedHTTPClient) signRequestPrecomputed(req *http.Request, bodyHash []byte, contentLength int64) error {
	// Refuse to sign without a key or an external signer.
	if c.priv == nil && c.sign == nil {
		return ErrSigningUnavailable
	}

	// Collect signed headers (only those present on the request).
	keys := make([]string, 0, len(signedHeaders))
	for _, name := range signedHeaders {
		if req.Header.Get(name) != "" {
			keys = append(keys, name)
		}
	}
	slices.Sort(keys)
	var hdrs strings.Builder
	for i, k := range keys {
		if i > 0 {
			hdrs.WriteByte(',')
		}
		hdrs.WriteString(k)
		hdrs.WriteByte('=')
		hdrs.WriteString(req.Header.Get(k))
	}

	// Stamp the request time and encode the body hash.
	timestampMs := time.Now().UnixMilli()
	bodyHashHex := hex.EncodeToString(bodyHash)

	// Build the signing payload proto and serialize to binary.
	// Proto binary serialization is deterministic - both Go and TS produce identical bytes.
	payload := &api.SigningPayload{
		EnvPrefix:     c.envPfx,
		Method:        req.Method,
		Path:          req.URL.Path,
		TimestampMs:   timestampMs,
		ContentLength: contentLength,
		BodyHashHex:   bodyHashHex,
		SignedHeaders: hdrs.String(),
	}
	payloadBytes, err := payload.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal signing payload")
	}

	// Sign with the held key, or with the external signer when one is set.
	signPayload := func(payload []byte) ([]byte, error) {
		return c.priv.Sign(payload)
	}
	if c.sign != nil {
		reqCtx := req.Context()
		signPayload = func(payload []byte) ([]byte, error) {
			return c.sign(reqCtx, payload)
		}
	}
	sig, err := signPayload(payloadBytes)
	if err != nil {
		return errors.Wrap(err, "sign payload")
	}

	// Set auth headers.
	req.Header.Set("X-Peer-ID", c.peerID.String())
	req.Header.Set("X-Timestamp", strconv.FormatInt(timestampMs, 10))
	req.Header.Set("X-Sw-Hash", bodyHashHex)
	req.Header.Set("X-Signature", base64.StdEncoding.EncodeToString(sig))
	if len(keys) > 0 {
		req.Header.Set("X-Signed-Headers", strings.Join(keys, ","))
	}

	return nil
}

// rateLimitRetries bounds how many times Do resends a rate limited request,
// and rateLimitMaxWait bounds the delay it waits before each resend.
const (
	rateLimitRetries = 3
	rateLimitMaxWait = time.Minute
)

// Do signs and executes an HTTP request. The Cloud rejects a rate_limited
// request before acting on it, so Do resends it after the delay the Cloud
// names, up to rateLimitRetries times while that delay is within
// rateLimitMaxWait. Otherwise, or when the request context ends during the
// delay, the 429 response is returned to the caller.
func (c *SignedHTTPClient) Do(req *http.Request) (*http.Response, error) {
	// Read the body once so each attempt can sign and send it.
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, errors.Wrap(err, "read request body")
		}
	}

	for attempt := 0; ; attempt++ {
		// Sign and send this attempt.
		if body != nil {
			req.Body = io.NopCloser(bytes.NewReader(body))
		}
		if err := c.signRequest(req, body); err != nil {
			return nil, err
		}
		resp, err := c.httpCli.Do(req) //nolint:gosec // This outbound HTTP client intentionally accepts caller-selected requests.
		if err != nil || resp.StatusCode != http.StatusTooManyRequests || attempt == rateLimitRetries {
			return resp, err
		}

		// Return the response unless it names a short rate limit delay.
		delay, err := rateLimitDelay(resp)
		if err != nil {
			return nil, err
		}
		if delay <= 0 || delay > rateLimitMaxWait {
			return resp, nil
		}

		// Wait out the delay before resending.
		select {
		case <-req.Context().Done():
			return resp, nil
		case <-time.After(delay):
		}
	}
}

// rateLimitDelay returns the delay a retryable rate_limited response names,
// or zero for any other response. It restores resp.Body for the caller.
func rateLimitDelay(resp *http.Response) (time.Duration, error) {
	// Read the body and restore it for the caller.
	body, err := readResponseBody(resp)
	_ = resp.Body.Close()
	if err != nil {
		return 0, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))

	// Report the delay only for a retryable rate limit.
	ce := parseCloudResponseError(resp, body)
	if ce.Code != "rate_limited" || !ce.Retryable {
		return 0, nil
	}
	return time.Duration(ce.RetryAfterSeconds) * time.Second, nil
}

// doPost signs and executes a POST request, returning the response body.
// reason tags the request with X-Alpha-Seed-Reason when non-empty.
func (c *SignedHTTPClient) doPost(ctx context.Context, path string, contentType string, body []byte, headers map[string]string, reason SeedReason) ([]byte, error) {
	// Build the request at the joined path with its content type and headers.
	reqURL, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return nil, errors.Wrap(err, "build URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return c.send(req, reason)
}

// doPostBinary signs and executes a POST request with protobuf binary content
// type, returning the response body. The body should be the marshaled proto
// (Proto.MarshalVT()) and the response body is the raw proto-binary bytes for
// the caller to UnmarshalVT into the typed response message.
//
// headers carries any extra request headers (e.g., X-Turnstile-Token).
// reason tags the request with X-Alpha-Seed-Reason when non-empty.
func (c *SignedHTTPClient) doPostBinary(ctx context.Context, path string, body []byte, headers map[string]string, reason SeedReason) ([]byte, error) {
	binHeaders := map[string]string{"Accept": "application/octet-stream"}
	maps.Copy(binHeaders, headers)
	return c.doPost(ctx, path, "application/octet-stream", body, binHeaders, reason)
}

// doDelete signs and executes a DELETE request, returning the response body.
// reason tags the request with X-Alpha-Seed-Reason when non-empty.
func (c *SignedHTTPClient) doDelete(ctx context.Context, path string, reason SeedReason) ([]byte, error) {
	return c.doResolved(ctx, http.MethodDelete, path, "application/octet-stream", reason)
}

// doGet signs and executes a GET request, returning the response body.
// reason tags the request with X-Alpha-Seed-Reason when non-empty.
func (c *SignedHTTPClient) doGet(ctx context.Context, path string, reason SeedReason) ([]byte, error) {
	return c.doResolved(ctx, http.MethodGet, path, "", reason)
}

// doGetBinary signs and executes a GET request, advertising protobuf binary on
// the response. Returns the response body for the caller to UnmarshalVT into
// the typed response message.
//
// reason tags the request with X-Alpha-Seed-Reason when non-empty.
func (c *SignedHTTPClient) doGetBinary(ctx context.Context, path string, reason SeedReason) ([]byte, error) {
	return c.doResolved(ctx, http.MethodGet, path, "application/octet-stream", reason)
}

// doResolved signs and executes a bodiless request at path, which may carry a
// query string, resolved against the base URL. accept sets the Accept header
// when non-empty.
func (c *SignedHTTPClient) doResolved(ctx context.Context, method, path, accept string, reason SeedReason) ([]byte, error) {
	// Resolve the path and its query against the base URL.
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, errors.Wrap(err, "parse base URL")
	}
	ref, err := url.Parse(path)
	if err != nil {
		return nil, errors.Wrap(err, "parse path")
	}

	// Build the request and send it.
	req, err := http.NewRequestWithContext(ctx, method, base.ResolveReference(ref).String(), nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	return c.send(req, reason)
}

// send signs and executes req, tagging it with reason when non-empty. It
// returns the body of a 2xx response, or the cloud error any other status
// carries.
func (c *SignedHTTPClient) send(req *http.Request, reason SeedReason) ([]byte, error) {
	// Tag and send the request.
	if reason != "" {
		req.Header.Set(SeedReasonHeader, string(reason))
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the body and report a non-2xx status as the cloud error.
	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseCloudResponseError(resp, respBody)
	}
	return respBody, nil
}

// postMessage signs and posts req to path and decodes the response into resp.
func (c *SignedHTTPClient) postMessage(ctx context.Context, path string, req, resp protobuf_go_lite.Message, reason SeedReason) error {
	body, err := req.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal request")
	}
	return c.postBody(ctx, path, body, resp, reason)
}

// postBody signs and posts an encoded body to path and decodes the response
// into resp.
func (c *SignedHTTPClient) postBody(ctx context.Context, path string, body []byte, resp protobuf_go_lite.Message, reason SeedReason) error {
	data, err := c.doPostBinary(ctx, path, body, nil, reason)
	return decodeResponse(resp, data, err)
}

// getMessage signs and sends a GET to path and decodes the response into resp.
func (c *SignedHTTPClient) getMessage(ctx context.Context, path string, resp protobuf_go_lite.Message, reason SeedReason) error {
	data, err := c.doGetBinary(ctx, path, reason)
	return decodeResponse(resp, data, err)
}

// deleteMessage signs and sends a DELETE to path and decodes the response
// into resp.
func (c *SignedHTTPClient) deleteMessage(ctx context.Context, path string, resp protobuf_go_lite.Message, reason SeedReason) error {
	data, err := c.doDelete(ctx, path, reason)
	return decodeResponse(resp, data, err)
}

// decodeResponse decodes a response body into resp, returning the request
// error unchanged when the request failed.
func decodeResponse(resp protobuf_go_lite.Message, data []byte, err error) error {
	if err != nil {
		return err
	}
	return errors.Wrap(resp.UnmarshalVT(data), "unmarshal response")
}

// sendMultiSig sends an encoded MultiSigRequest without a session signature,
// since multi-sig routes authenticate by the entity signatures in the body,
// and decodes the MultiSigActionResponse. reason tags the request when
// non-empty.
func (c *SignedHTTPClient) sendMultiSig(ctx context.Context, method, reqPath string, body []byte, reason SeedReason) (*api.MultiSigActionResponse, error) {
	// Build the unsigned request.
	reqURL, err := url.JoinPath(c.baseURL, reqPath)
	if err != nil {
		return nil, errors.Wrap(err, "build URL")
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "create request")
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	if reason != "" {
		req.Header.Set(SeedReasonHeader, string(reason))
	}

	// Send it and report a status other than 200 as the cloud error.
	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "multi-sig request")
	}
	defer httpclient.DrainAndCloseResponseBody(resp)
	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseCloudResponseError(resp, respBody)
	}

	// Decode the action response; an empty body carries no result.
	out := &api.MultiSigActionResponse{}
	if err := out.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal multi-sig response")
	}
	return out, nil
}

// MultiSigContext is the signing context for multi-sig actions.
const MultiSigContext = entitykeystore.MultiSigContext

// BuildMultiSigPayload constructs the signing payload for a multi-sig action.
// The signing payload is: MultiSigContext || Timestamp.toBinary(signedAt) ||
// envelope. Must produce identical bytes as the TS server.
func BuildMultiSigPayload(signedAt *timestamppb.Timestamp, envelope []byte) []byte {
	return entitykeystore.BuildMultiSigPayload(signedAt, envelope)
}

// EntityClient uses the entity keypair for registration flows.
type EntityClient struct {
	*SignedHTTPClient
}

// DefaultSigningEnvPrefix is the default request-signing environment prefix.
const DefaultSigningEnvPrefix = "spacewave"

func normalizeSigningEnvPrefix(signingEnvPfx string) string {
	if signingEnvPfx == "" {
		return DefaultSigningEnvPrefix
	}
	return signingEnvPfx
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
			peerID:  p.GetPeerID(),
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
			priv:    priv,
			peerID:  pid,
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
			peerID:  pid,
			sign:    sign,
		},
	}
}

// initPrivKey initializes the private key from the peer if not already set.
func (c *EntityClient) initPrivKey(ctx context.Context, p peer.Peer) error {
	// Load the entity key from the peer once.
	if c.priv != nil {
		return nil
	}
	priv, err := p.GetPrivKey(ctx)
	if err != nil {
		return errors.Wrap(err, "get entity private key")
	}
	c.priv = priv
	return nil
}

// RegisterAccount registers an account with the Spacewave cloud.
//
// Returns the server-generated account ID.
func (c *EntityClient) RegisterAccount(ctx context.Context, entityID, authMethod string, authParams []byte, turnstileToken string) (string, error) {
	// Require the entity key that signs the registration.
	if c.priv == nil {
		return "", errors.New("no private key configured")
	}

	// Encode the account with its first entity keypair.
	req := &api.RegisterAccountRequest{
		EntityId: entityID,
		Keypairs: []*session.EntityKeypair{{
			PeerId:     c.peerID.String(),
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
// request shape. Existing callers should leave Type unset for USER defaults;
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

// SessionClient uses the session keypair for authenticated API calls.
type SessionClient struct {
	*SignedHTTPClient

	// readGrantsMtx guards readGrants.
	readGrantsMtx sync.Mutex
	// readGrants caches pack read grants by resource and pack ID.
	readGrants map[readGrantKey]*packfile.ReadGrant

	// recoveryKeypairs is the owning account's keypair listing cache, or nil
	// for a client no account configured.
	recoveryKeypairs *recoveryKeypairCache
}

// NewSessionClient constructs a SessionClient with the given session key.
func NewSessionClient(
	httpCli *http.Client,
	endpoint string,
	signingEnvPfx string,
	priv crypto.PrivKey,
	peerIDStr string,
) *SessionClient {
	var pid peer.ID
	if peerIDStr != "" {
		pid, _ = peer.IDB58Decode(peerIDStr)
	}
	return &SessionClient{
		SignedHTTPClient: &SignedHTTPClient{
			httpCli: httpCli,
			baseURL: endpoint,
			envPfx:  normalizeSigningEnvPrefix(signingEnvPfx),
			priv:    priv,
			peerID:  pid,
		},
	}
}

// NewSessionClientSigner constructs a SessionClient backed by a request-signing callback.
func NewSessionClientSigner(
	httpCli *http.Client,
	endpoint string,
	signingEnvPfx string,
	peerIDStr string,
	sign SigningFunc,
) *SessionClient {
	var pid peer.ID
	if peerIDStr != "" {
		pid, _ = peer.IDB58Decode(peerIDStr)
	}
	return &SessionClient{
		SignedHTTPClient: &SignedHTTPClient{
			httpCli: httpCli,
			baseURL: endpoint,
			envPfx:  normalizeSigningEnvPrefix(signingEnvPfx),
			peerID:  pid,
			sign:    sign,
		},
	}
}

// GetAdminJSON sends a signed admin GET request and returns the JSON response body.
// requestPath is relative to /api/admin and may carry a query string. A path
// that leaves /api/admin is rejected.
func (c *SessionClient) GetAdminJSON(ctx context.Context, requestPath string) ([]byte, error) {
	// Parse the route and its query, refusing a reference to another host.
	ref, err := url.Parse(requestPath)
	if err != nil {
		return nil, errors.Wrap(err, "parse admin path")
	}
	if ref.IsAbs() || ref.Host != "" {
		return nil, errors.New("admin path must be relative")
	}

	// Resolve the route below /api/admin and send it with the query unchanged.
	adminPath := path.Join("/api/admin", ref.Path)
	if adminPath != "/api/admin" && !strings.HasPrefix(adminPath, "/api/admin/") {
		return nil, errors.New("admin path must stay below /api/admin")
	}
	ref.Path, ref.RawPath = adminPath, ""
	return c.doGet(ctx, ref.String(), SeedReasonColdSeed)
}

// DoMultiSig sends a pre-signed multi-sig request to the cloud and returns the
// parsed MultiSigActionResponse envelope.
// Multi-sig routes authenticate via body signatures, not session headers.
func (c *SessionClient) DoMultiSig(ctx context.Context, method string, reqPath string, body []byte) (*api.MultiSigActionResponse, error) {
	return c.sendMultiSig(ctx, method, reqPath, body, SeedReasonMutation)
}

// GetSessionTicket requests a short-lived JWT ticket for WebSocket auth.
func (c *SessionClient) GetSessionTicket(ctx context.Context) (string, error) {
	var resp api.TicketResponse
	if err := c.postMessage(ctx, "/api/session/ticket", &api.SessionTicketRequest{}, &resp, SeedReasonReconnect); err != nil {
		return "", errors.Wrap(err, "get session ticket")
	}
	if resp.GetTicket() == "" {
		return "", errors.New("empty ticket in response")
	}
	return resp.GetTicket(), nil
}

// SyncPush uploads a packfile to a resource-scoped block store.
// bodyHash is the pre-computed SHA-256 hash of the file contents.
// bloomFormatVersion identifies the bloom encoding (currently
// packfile.BloomFormatVersionV1).
func (c *SessionClient) SyncPush(ctx context.Context, resourceID string, packID string, blockCount int, packfilePath string, bodyHash []byte, bloomFilter []byte, bloomFormatVersion uint32) error {
	// Open the packfile and read its size.
	f, err := os.Open(packfilePath)
	if err != nil {
		return errors.Wrap(err, "open packfile")
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return errors.Wrap(err, "stat packfile")
	}

	// Push the file contents.
	return c.syncPush(ctx, resourceID, &syncPushPack{
		packID:             packID,
		blockCount:         blockCount,
		bodyHash:           bodyHash,
		bloomFilter:        bloomFilter,
		bloomFormatVersion: bloomFormatVersion,
	}, io.NewSectionReader(f, 0, stat.Size()), stat.Size())
}

// SyncPushData uploads an in-memory packfile to a resource-scoped block store.
// bodyHash is the pre-computed SHA-256 hash of the file contents.
// bloomFormatVersion identifies the bloom encoding (currently
// packfile.BloomFormatVersionV1).
func (c *SessionClient) SyncPushData(ctx context.Context, resourceID string, packID string, blockCount int, packData []byte, bodyHash []byte, bloomFilter []byte, bloomFormatVersion uint32) error {
	return c.syncPushDataWithProgress(ctx, resourceID, &syncPushPack{
		packID:             packID,
		blockCount:         blockCount,
		bodyHash:           bodyHash,
		bloomFilter:        bloomFilter,
		bloomFormatVersion: bloomFormatVersion,
	}, packData, nil)
}

// syncPushDataWithProgress uploads an in-memory pack, reporting sent bytes to
// progress when set. A replacement conflict fails without retry.
func (c *SessionClient) syncPushDataWithProgress(
	ctx context.Context,
	resourceID string,
	pack *syncPushPack,
	packData []byte,
	progress func(int64),
) error {
	var body io.Reader = bytes.NewReader(packData)
	if progress != nil {
		body = syncprogress.NewReader(body, progress)
	}
	return c.syncPush(ctx, resourceID, pack, body, int64(len(packData)))
}

// syncPush pushes a pack of size bytes read from body in three steps: the
// cloud admits the pack and returns a signed upload, the bytes go straight to
// storage, and the commit adds the pack to the catalog once the cloud has
// checked the stored size and digest. A pack the catalog already holds skips
// the upload.
func (c *SessionClient) syncPush(ctx context.Context, resourceID string, pack *syncPushPack, body io.Reader, size int64) error {
	// Require the bloom filter the catalog indexes the pack by.
	if len(pack.bloomFilter) == 0 {
		return errors.New("sync push bloom filter required")
	}
	if pack.bloomFormatVersion == 0 {
		return errors.New("sync push bloom_format_version required")
	}

	// Admit the pack, learning whether the catalog needs its bytes.
	pushPath := path.Join("/api/bstore", resourceID, "sync/push")
	req := &packfile.PushRequest{
		PackId:             pack.packID,
		BlockCount:         uint64(pack.blockCount), //nolint:gosec // block counts are positive
		BloomFilter:        pack.bloomFilter,
		BloomFormatVersion: pack.bloomFormatVersion,
		SizeBytes:          uint64(size), //nolint:gosec // sizes are positive
		Sha256:             pack.bodyHash,
		ReplacesPackIds:    pack.replacedPackIDs,
	}
	resp := &packfile.PushResponse{}
	if err := c.postMessage(ctx, pushPath, req, resp, SeedReasonMutation); err != nil {
		return errors.Wrap(err, "sync push")
	}
	upload := resp.GetUpload()
	if upload == nil {
		return nil
	}

	// Upload the bytes to storage.
	if err := c.uploadPack(ctx, upload, body, size); err != nil {
		return errors.Wrap(err, "sync push upload")
	}

	// Commit the stored pack to the catalog.
	_, err := c.doPostBinary(ctx, path.Join(pushPath, pack.packID, "commit"), nil, nil, SeedReasonMutation)
	return errors.Wrap(err, "sync push commit")
}

// uploadPack sends size bytes of body to the signed upload URL.
func (c *SessionClient) uploadPack(ctx context.Context, upload *packfile.PushUpload, body io.Reader, size int64) error {
	// Build the PUT with the exact length and headers the URL was signed for.
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, upload.GetUrl(), body)
	if err != nil {
		return err
	}
	req.ContentLength = size
	for k, v := range upload.GetHeaders() {
		req.Header.Set(k, v)
	}

	// Send the upload and report a storage refusal with its reason.
	resp, err := c.httpCli.Do(req)
	if err != nil {
		return err
	}
	defer httpclient.DrainAndCloseResponseBody(resp)
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return errors.Errorf("storage refused the upload: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}

// SyncPull reads one page of a resource-scoped block store catalog after the
// since cursor. Since zero starts from the beginning.
func (c *SessionClient) SyncPull(ctx context.Context, resourceID string, since uint64) (*packfile.PullResponse, error) {
	// Address the page after the since cursor.
	p := path.Join("/api/bstore", resourceID, "sync/pull")
	if since != 0 {
		p += "?since=" + strconv.FormatUint(since, 10)
	}

	// Fetch and decode the binary catalog page.
	resp := &packfile.PullResponse{}
	if err := c.getMessage(ctx, p, resp, SeedReasonColdSeed); err != nil {
		return nil, errors.Wrap(err, "sync pull")
	}
	return resp, nil
}

// PostOp posts one signed operation to a shared object.
func (c *SessionClient) PostOp(ctx context.Context, soID string, opData []byte) error {
	var resp api.SubmitOpResponse
	err := c.postBody(ctx, path.Join("/api/sobject", soID, "op"), opData, &resp, SeedReasonMutation)
	return errors.Wrap(err, "post op")
}

// maxOpsBatchBytes is the largest operation batch body the cloud accepts.
const maxOpsBatchBytes = 256 << 10

// PostOps posts one bounded batch of signed operations to a shared object.
func (c *SessionClient) PostOps(ctx context.Context, soID string, operations []*sobject.SOOperation) error {
	// Encode a batch within the size limits.
	if len(operations) == 0 || len(operations) > 50 {
		return errors.New("operation batch must contain 1 to 50 operations")
	}
	body, err := (&api.PostOpsRequest{Operations: operations}).MarshalVT()
	if err != nil {
		return err
	}
	if len(body) > maxOpsBatchBytes {
		return errors.New("operation batch exceeds 256 KiB")
	}

	// Post the batch.
	var resp api.SubmitOpResponse
	err = c.postBody(ctx, path.Join("/api/sobject", soID, "ops"), body, &resp, SeedReasonMutation)
	return errors.Wrap(err, "post ops")
}

// PostCheckpoint posts an owner-signed checkpoint to a shared object. The
// first checkpoint of a new object is its genesis checkpoint.
func (c *SessionClient) PostCheckpoint(ctx context.Context, soID string, checkpoint *sobject.SOCheckpoint) error {
	var resp api.SubmitCheckpointResponse
	err := c.postMessage(ctx, path.Join("/api/sobject", soID, "checkpoint"), &api.PostCheckpointRequest{Checkpoint: checkpoint}, &resp, SeedReasonMutation)
	return errors.Wrap(err, "post checkpoint")
}

// PostClientErrorReport submits a best-effort diagnostic report for a client-side failure.
func (c *SessionClient) PostClientErrorReport(
	ctx context.Context,
	errorCode string,
	component string,
	resourceType string,
	resourceID string,
	detail string,
) error {
	req := &api.ClientErrorReportRequest{
		ErrorCode:    errorCode,
		Component:    component,
		ResourceType: resourceType,
		ResourceId:   resourceID,
		Detail:       detail,
	}
	var resp api.ClientErrorReportResponse
	err := c.postMessage(ctx, "/api/account/client-error-report", req, &resp, SeedReasonMutation)
	return errors.Wrap(err, "post client error report")
}

// CreateSharedObject creates a new shared object in the cloud.
// ownerType is "account" or "organization"; ownerID is the principal id.
func (c *SessionClient) CreateSharedObject(
	ctx context.Context,
	soID string,
	displayName string,
	objectType string,
	ownerType string,
	ownerID string,
	accountPrivate bool,
) error {
	req := &api.CreateSObjectRequest{
		DisplayName:    displayName,
		ObjectType:     objectType,
		OwnerType:      ownerType,
		OwnerId:        ownerID,
		AccountPrivate: accountPrivate,
	}
	var resp api.CreateSObjectResponse
	err := c.postMessage(ctx, path.Join("/api/sobject", soID, "create"), req, &resp, SeedReasonMutation)
	return errors.Wrap(err, "create shared object")
}

// ListSharedObjects lists shared objects from the cloud.
func (c *SessionClient) ListSharedObjects(ctx context.Context) ([]byte, error) {
	data, err := c.doGet(ctx, "/api/sobject/list", SeedReasonListBootstrap)
	if err != nil {
		return nil, errors.Wrap(err, "list shared objects")
	}
	return data, nil
}

// GetSOState retrieves the current state of a shared object.
// If since > 0, the server may use it as a hint for incremental delivery.
// reason tags the fan-out origin (cold-seed, gap-recovery, or reconnect).
func (c *SessionClient) GetSOState(ctx context.Context, soID string, since uint64, reason SeedReason) ([]byte, error) {
	// Address the state, hinting the last seen sequence.
	p := path.Join("/api/sobject", soID, "state")
	if since > 0 {
		p += "?since=" + strconv.FormatUint(since, 10)
	}

	// Fetch the encoded state.
	data, err := c.doGet(ctx, p, reason)
	if err != nil {
		return nil, errors.Wrap(err, "get so state")
	}
	return data, nil
}

// GetConfigChain retrieves the config change chain and key epochs for a shared object.
func (c *SessionClient) GetConfigChain(ctx context.Context, soID string) ([]byte, error) {
	data, err := c.doGet(ctx, path.Join("/api/sobject", soID, "config-chain"), SeedReasonConfigChainVerify)
	if err != nil {
		return nil, errors.Wrap(err, "get config chain")
	}
	return data, nil
}

// PostConfig posts a signed config change to a shared object.
func (c *SessionClient) PostConfig(ctx context.Context, soID string, configData []byte) error {
	var resp api.PostConfigResponse
	err := c.postBody(ctx, path.Join("/api/sobject", soID, "config"), configData, &resp, SeedReasonMutation)
	return errors.Wrap(err, "post config")
}

// PostConfigState posts a signed config change and updated invite state.
func (c *SessionClient) PostConfigState(
	ctx context.Context,
	soID string,
	configData []byte,
	invites []*sobject.SOInvite,
	keyEpoch *sobject.SOKeyEpoch,
	recoveryEnvelopes []*sobject.SOEntityRecoveryEnvelope,
) error {
	req := &api.PostConfigStateRequest{
		ConfigChange:      configData,
		Invites:           invites,
		KeyEpoch:          keyEpoch,
		RecoveryEnvelopes: recoveryEnvelopes,
	}
	var resp api.PostConfigStateResponse
	err := c.postMessage(ctx, path.Join("/api/sobject", soID, "config-state"), req, &resp, SeedReasonMutation)
	return errors.Wrap(err, "post config state")
}

// PostControl posts signed control messages of the open group decision. The
// cloud skips messages of a closed decision and ones it holds.
func (c *SessionClient) PostControl(ctx context.Context, soID string, control []*sobject.SOControlMessage) error {
	body, err := (&api.SOControlBatch{Control: control}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal control batch")
	}
	_, err = c.doPostBinary(
		ctx,
		path.Join("/api/sobject", soID, "control"),
		body,
		nil,
		SeedReasonMutation,
	)
	return errors.Wrap(err, "post control")
}

// PostKeyEpoch posts a new key epoch (after key rotation) to the server.
func (c *SessionClient) PostKeyEpoch(
	ctx context.Context,
	soID string,
	epoch *sobject.SOKeyEpoch,
	recoveryEnvelopes []*sobject.SOEntityRecoveryEnvelope,
) error {
	req := &api.PostKeyEpochRequest{
		KeyEpoch:          epoch,
		RecoveryEnvelopes: recoveryEnvelopes,
	}
	var resp api.PostKeyEpochResponse
	err := c.postMessage(ctx, path.Join("/api/sobject", soID, "key-epoch"), req, &resp, SeedReasonMutation)
	return errors.Wrap(err, "post key epoch")
}

// EnrollMember resolves all registered session peer IDs for a target account
// on a given shared object. The SO DO queries D1 sessions for the account.
// Returns the list of session peers keyed by peer_id.
func (c *SessionClient) EnrollMember(ctx context.Context, soID, accountID string, ignoreExclusion bool) (*api.EnrollMemberResponse, error) {
	req := &api.EnrollMemberRequest{
		AccountId:       accountID,
		IgnoreExclusion: ignoreExclusion,
	}
	resp := &api.EnrollMemberResponse{}
	if err := c.postMessage(ctx, path.Join("/api/sobject", soID, "enroll-member"), req, resp, SeedReasonRejoin); err != nil {
		return nil, err
	}
	return resp, nil
}

// ResolveMemberParticipants resolves the current SO participant peer IDs for a
// target account on a given shared object.
func (c *SessionClient) ResolveMemberParticipants(ctx context.Context, soID, accountID string) (*api.ResolveMemberParticipantsResponse, error) {
	resp := &api.ResolveMemberParticipantsResponse{}
	if err := c.postMessage(ctx, path.Join("/api/sobject", soID, "member-participants"), &api.ResolveMemberParticipantsRequest{AccountId: accountID}, resp, SeedReasonRejoin); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListSORecoveryEntityKeypairs retrieves current keypairs for readable entity
// participants on a shared object.
func (c *SessionClient) ListSORecoveryEntityKeypairs(
	ctx context.Context,
	soID string,
) (*api.ListSORecoveryEntityKeypairsResponse, error) {
	resp := &api.ListSORecoveryEntityKeypairsResponse{}
	if err := c.getMessage(ctx, path.Join("/api/sobject", soID, "recovery-entity-keypairs"), resp, SeedReasonRejoin); err != nil {
		return nil, errors.Wrap(err, "list recovery entity keypairs")
	}
	return resp, nil
}

// GetSORecoveryEnvelope retrieves the current recovery envelope for the
// authenticated entity on a shared object.
func (c *SessionClient) GetSORecoveryEnvelope(
	ctx context.Context,
	soID string,
) (*sobject.SOEntityRecoveryEnvelope, error) {
	resp := &api.GetSORecoveryEnvelopeResponse{}
	if err := c.getMessage(ctx, path.Join("/api/sobject", soID, "recovery-envelope"), resp, SeedReasonRejoin); err != nil {
		return nil, errors.Wrap(err, "get recovery envelope")
	}
	if resp.GetEnvelope() == nil {
		return nil, errors.New("recovery envelope missing from response")
	}
	return resp.GetEnvelope(), nil
}

// GetSOSequencer returns the peer ID of the key the cloud signs a shared
// object's order with.
func (c *SessionClient) GetSOSequencer(ctx context.Context, soID string) (string, error) {
	// Fetch and decode the sequencer response.
	resp := &api.GetSOSequencerResponse{}
	if err := c.getMessage(ctx, path.Join("/api/sobject", soID, "sequencer"), resp, SeedReasonColdSeed); err != nil {
		return "", errors.Wrap(err, "get sequencer")
	}

	// A Cloud without a sequencer secret names no peer.
	if resp.GetPeerId() == "" {
		return "", errors.New("sequencer peer ID missing from response")
	}
	return resp.GetPeerId(), nil
}

// RegisterInviteCode registers a short invite code for the shared object.
// The code maps to the full serialized SOInviteMessage for lookup.
func (c *SessionClient) RegisterInviteCode(ctx context.Context, soID string, req *api.RegisterInviteCodeRequest) error {
	var resp api.RegisterInviteCodeResponse
	err := c.postMessage(ctx, path.Join("/api/sobject", soID, "invite-code"), req, &resp, SeedReasonMutation)
	return errors.Wrap(err, "register invite code")
}

// LookupInviteCode resolves a short invite code to the full SOInviteMessage.
func (c *SessionClient) LookupInviteCode(ctx context.Context, code string) (*api.LookupInviteCodeResponse, error) {
	resp := &api.LookupInviteCodeResponse{}
	if err := c.getMessage(ctx, "/api/sobject/lookup-code?code="+url.QueryEscape(code), resp, SeedReasonColdSeed); err != nil {
		return nil, errors.Wrap(err, "lookup invite code")
	}
	return resp, nil
}

// GetMailboxEntries returns pending mailbox entries for a shared object.
func (c *SessionClient) GetMailboxEntries(
	ctx context.Context,
	soID string,
) (*api.GetMailboxResponse, error) {
	return c.getMailboxEntries(ctx, soID, "pending")
}

// GetAcceptedMailboxEntries returns the accepted mailbox entries for a shared
// object. Each pairs an admitted peer with the account that submitted it.
func (c *SessionClient) GetAcceptedMailboxEntries(
	ctx context.Context,
	soID string,
) (*api.GetMailboxResponse, error) {
	return c.getMailboxEntries(ctx, soID, "accepted")
}

// getMailboxEntries returns the mailbox entries for a shared object with the
// given status. Only the owner may read them.
func (c *SessionClient) getMailboxEntries(
	ctx context.Context,
	soID string,
	status string,
) (*api.GetMailboxResponse, error) {
	// Fetch the entries with the requested status.
	path := "/api/sobject/" + soID + "/invite-mailbox?status=" + status
	respBody, err := c.doGet(ctx, path, SeedReasonColdSeed)
	if err != nil {
		return nil, errors.Wrap(err, "get mailbox entries")
	}

	// Decode the response.
	resp := &api.GetMailboxResponse{}
	if err := resp.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal mailbox response")
	}
	return resp, nil
}

// SubmitMailboxEntry submits a mailbox join request for a shared object.
func (c *SessionClient) SubmitMailboxEntry(
	ctx context.Context,
	soID string,
	req *api.SubmitMailboxEntryRequest,
) (*api.SubmitMailboxEntryResponse, error) {
	resp := &api.SubmitMailboxEntryResponse{}
	if err := c.postMessage(ctx, "/api/sobject/"+soID+"/invite-mailbox", req, resp, SeedReasonMutation); err != nil {
		return nil, errors.Wrap(err, "submit mailbox entry")
	}
	return resp, nil
}

// ProcessMailboxEntry processes a mailbox entry and returns the resulting status.
func (c *SessionClient) ProcessMailboxEntry(
	ctx context.Context,
	soID string,
	req *api.ProcessMailboxEntryRequest,
) (*api.ProcessMailboxEntryResponse, error) {
	resp := &api.ProcessMailboxEntryResponse{}
	if err := c.postMessage(ctx, "/api/sobject/"+soID+"/invite-mailbox/process", req, resp, SeedReasonMutation); err != nil {
		return nil, errors.Wrap(err, "process mailbox entry")
	}
	return resp, nil
}

// WithdrawMailboxEntries withdraws this peer's pending mailbox entries on soID
// and returns how many it withdrew.
func (c *SessionClient) WithdrawMailboxEntries(ctx context.Context, soID string) (uint32, error) {
	// The route identifies the peer by its signed request, so it has no body.
	resp := &api.WithdrawMailboxEntriesResponse{}
	if err := c.postBody(ctx, "/api/sobject/"+soID+"/invite-mailbox/withdraw", nil, resp, SeedReasonMutation); err != nil {
		return 0, errors.Wrap(err, "withdraw mailbox entries")
	}
	return resp.GetWithdrawn(), nil
}

// CreateCheckoutSession submits the customer's explicit monthly-offer consent.
func (c *SessionClient) CreateCheckoutSession(ctx context.Context, req *s4wave_provider_spacewave.CreateCheckoutSessionRequest) (*api.CheckoutResponse, error) {
	checkout := &api.CheckoutRequest{
		SuccessUrl:       req.GetSuccessUrl(),
		CancelUrl:        req.GetCancelUrl(),
		BillingInterval:  req.GetBillingInterval(),
		BillingAccountId: req.GetBillingAccountId(),
		Consent:          req.GetConsent(),
	}
	var resp api.CheckoutResponse
	if err := c.postMessage(ctx, "/api/billing/checkout", checkout, &resp, SeedReasonMutation); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateBillingAccount creates a new unassigned billing account owned
// (managed) by the caller. Returns the new BA's ULID.
func (c *SessionClient) CreateBillingAccount(ctx context.Context, displayName string) (string, error) {
	req := &api.CreateBillingAccountRequest{
		DisplayName: displayName,
	}
	var resp api.CreateBillingAccountResponse
	if err := c.postMessage(ctx, "/api/billing/accounts", req, &resp, SeedReasonMutation); err != nil {
		return "", err
	}
	return resp.GetBillingAccountId(), nil
}

// RenameBillingAccount updates the display_name on a BA the caller manages.
func (c *SessionClient) RenameBillingAccount(ctx context.Context, baID, displayName string) error {
	body, err := (&api.RenameBillingAccountRequest{
		BillingAccountId: baID,
		DisplayName:      displayName,
	}).MarshalVT()
	if err != nil {
		return err
	}
	if _, err := c.doPost(ctx, "/api/billing/"+baID+"/rename", "application/octet-stream", body, nil, SeedReasonMutation); err != nil {
		return err
	}
	return nil
}

// DeleteBillingAccount permanently removes a canceled BA the caller manages.
func (c *SessionClient) DeleteBillingAccount(ctx context.Context, baID string) error {
	var resp api.DeleteBillingAccountResponse
	return c.deleteMessage(ctx, "/api/billing/accounts/"+baID, &resp, SeedReasonMutation)
}

// CancelCheckoutSession cancels pending checkout attempts and expires the
// Stripe session. Returns 'completed' if the subscription activated during
// the race window.
func (c *SessionClient) CancelCheckoutSession(ctx context.Context) (*api.CheckoutResponse, error) {
	var resp api.CheckoutResponse
	if err := c.deleteMessage(ctx, "/api/billing/checkout", &resp, SeedReasonMutation); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetBillingState retrieves the billing state for a billing account.
func (c *SessionClient) GetBillingState(ctx context.Context, baID string) ([]byte, error) {
	return c.doGetBinary(ctx, "/api/billing/"+baID+"/state", SeedReasonColdSeed)
}

// GetBillingUsage retrieves usage data for a billing account.
func (c *SessionClient) GetBillingUsage(ctx context.Context, baID string) ([]byte, error) {
	return c.doGetBinary(ctx, "/api/billing/"+baID+"/usage-query", SeedReasonColdSeed)
}

// CancelSubscription cancels a billing account subscription.
func (c *SessionClient) CancelSubscription(ctx context.Context, baID string) (*api.CancelBillingResponse, error) {
	resp := &api.CancelBillingResponse{}
	if err := c.postMessage(ctx, "/api/billing/"+baID+"/cancel", &api.CancelBillingRequest{}, resp, SeedReasonMutation); err != nil {
		return nil, err
	}
	return resp, nil
}

// ReactivateSubscription reactivates a canceled billing account subscription.
func (c *SessionClient) ReactivateSubscription(ctx context.Context, baID string) (*api.ReactivateBillingResponse, error) {
	resp := &api.ReactivateBillingResponse{}
	if err := c.postMessage(ctx, "/api/billing/"+baID+"/reactivate", &api.ReactivateBillingRequest{}, resp, SeedReasonMutation); err != nil {
		return nil, err
	}
	return resp, nil
}

// SetBillingSpendingLimit persists an explicitly selected recurring budget.
func (c *SessionClient) SetBillingSpendingLimit(ctx context.Context, baID string, consent *s4wave_provider_spacewave.BillingConsent) error {
	body, err := (&s4wave_provider_spacewave.SetBillingSpendingLimitRequest{Consent: consent}).MarshalVT()
	if err != nil {
		return err
	}
	_, err = c.doPost(ctx, "/api/billing/"+baID+"/spending-limit", "application/octet-stream", body, nil, SeedReasonMutation)
	return err
}

// CreateBillingPortal creates a Stripe billing portal session and returns the URL.
func (c *SessionClient) CreateBillingPortal(ctx context.Context, baID string) (string, error) {
	var resp api.BillingPortalResponse
	if err := c.postMessage(ctx, "/api/billing/"+baID+"/portal", &api.BillingPortalRequest{}, &resp, SeedReasonMutation); err != nil {
		return "", err
	}
	return resp.GetUrl(), nil
}

// GetAccountInfo retrieves account info from the cloud.
func (c *SessionClient) GetAccountInfo(ctx context.Context) (*api.AccountInfoResponse, error) {
	var resp api.AccountInfoResponse
	if err := c.getMessage(ctx, "/api/account/info", &resp, SeedReasonColdSeed); err != nil {
		return nil, errors.Wrap(err, "get account info")
	}
	return &resp, nil
}

// GetPeerID returns the session peer ID.
func (c *SessionClient) GetPeerID() peer.ID {
	return c.peerID
}

// SelfRevoke revokes the current session using session-signed auth headers.
// No entity key or multi-sig is needed.
func (c *SessionClient) SelfRevoke(ctx context.Context) error {
	_, err := c.doDelete(ctx, "/api/session/revoke", SeedReasonMutation)
	if err != nil {
		return errors.Wrap(err, "self-revoke session")
	}
	return nil
}

// ListSessions retrieves the attached cloud auth session set for the account.
func (c *SessionClient) ListSessions(ctx context.Context) ([]*api.AccountSessionInfo, error) {
	var resp api.ListAccountSessionsResponse
	if err := c.getMessage(ctx, "/api/account/sessions", &resp, SeedReasonColdSeed); err != nil {
		return nil, errors.Wrap(err, "list sessions")
	}
	return resp.GetSessions(), nil
}

// ListKeypairs retrieves entity keypairs from the cloud.
func (c *SessionClient) ListKeypairs(ctx context.Context) ([]*session.EntityKeypair, error) {
	var resp api.ListKeypairsResponse
	if err := c.getMessage(ctx, "/api/account/keypairs", &resp, SeedReasonColdSeed); err != nil {
		return nil, errors.Wrap(err, "list keypairs")
	}
	return resp.GetKeypairs(), nil
}

// GetAccountState retrieves combined account info and keypairs from the cloud.
func (c *SessionClient) GetAccountState(ctx context.Context) (*api.AccountStateResponse, error) {
	var resp api.AccountStateResponse
	if err := c.getMessage(ctx, "/api/account/state", &resp, SeedReasonColdSeed); err != nil {
		return nil, errors.Wrap(err, "get account state")
	}
	return &resp, nil
}

// EnsureAccountSObjectBinding reserves or returns an account-owned shared object
// binding for the requested purpose.
func (c *SessionClient) EnsureAccountSObjectBinding(
	ctx context.Context,
	purpose string,
) (*api.AccountSObjectBinding, error) {
	req := &api.EnsureAccountSObjectBindingRequest{Purpose: purpose}
	return c.postAccountSObjectBinding(ctx, "ensure", req, &api.EnsureAccountSObjectBindingResponse{})
}

// FinalizeAccountSObjectBinding marks a reserved account-owned shared object
// binding ready after client-signed initialization succeeds.
func (c *SessionClient) FinalizeAccountSObjectBinding(
	ctx context.Context,
	purpose string,
	soID string,
) (*api.AccountSObjectBinding, error) {
	req := &api.FinalizeAccountSObjectBindingRequest{Purpose: purpose, SoId: soID}
	return c.postAccountSObjectBinding(ctx, "finalize", req, &api.FinalizeAccountSObjectBindingResponse{})
}

// accountSObjectBindingResponse is a response carrying an account shared
// object binding.
type accountSObjectBindingResponse interface {
	protobuf_go_lite.Message
	GetBinding() *api.AccountSObjectBinding
}

// postAccountSObjectBinding posts req to the given step of the account shared
// object binding route and returns the binding resp carries.
func (c *SessionClient) postAccountSObjectBinding(
	ctx context.Context,
	step string,
	req protobuf_go_lite.Message,
	resp accountSObjectBindingResponse,
) (*api.AccountSObjectBinding, error) {
	if err := c.postMessage(ctx, "/api/account/sobject-binding/"+step, req, resp, SeedReasonMutation); err != nil {
		return nil, errors.Wrap(err, step+" account sobject binding")
	}
	if resp.GetBinding() == nil {
		return nil, errors.New("missing account sobject binding in response")
	}
	return resp.GetBinding(), nil
}

// ListOrganizations returns the user's organizations.
func (c *SessionClient) ListOrganizations(ctx context.Context) ([]byte, error) {
	return c.doGet(ctx, "/api/org/list", SeedReasonListBootstrap)
}

// GetSOMetadata returns metadata for a shared object.
func (c *SessionClient) GetSOMetadata(ctx context.Context, soID string) ([]byte, error) {
	return c.doGet(ctx, "/api/sobject/"+soID+"/meta", SeedReasonColdSeed)
}

// UpdateSOMetadata updates metadata for a shared object. Omitted fields (zero
// values in the proto) are preserved server-side: display_name is required for
// "space" object types, public_read is an opt-in toggle that only changes when
// meta.PublicRead is true.
func (c *SessionClient) UpdateSOMetadata(ctx context.Context, soID string, meta *api.SpaceMetadataResponse) ([]byte, error) {
	body, err := meta.MarshalVT()
	if err != nil {
		return nil, err
	}
	return c.doPost(ctx, "/api/sobject/"+soID+"/update", "application/octet-stream", body, nil, SeedReasonMutation)
}

// ReinitializeSharedObject destructively rewrites a broken shared object in place.
func (c *SessionClient) ReinitializeSharedObject(ctx context.Context, soID string) error {
	var resp api.ReinitializeSObjectResponse
	err := c.postMessage(ctx, "/api/sobject/"+soID+"/reinitialize", &api.ReinitializeSObjectRequest{}, &resp, SeedReasonMutation)
	return errors.Wrap(err, "reinitialize shared object")
}

// CreateOrganization creates a new organization.
func (c *SessionClient) CreateOrganization(ctx context.Context, displayName string) ([]byte, error) {
	body, err := (&api.CreateOrgRequest{DisplayName: displayName}).MarshalVT()
	if err != nil {
		return nil, err
	}
	return c.doPost(ctx, "/api/org/create", "application/octet-stream", body, nil, SeedReasonMutation)
}

// CreateOrgInvite creates an invite for an organization.
func (c *SessionClient) CreateOrgInvite(ctx context.Context, orgID string, inviteType string, maxUses int32, expiresAt int64, email string) ([]byte, error) {
	body, err := (&api.CreateOrgInviteRequest{
		Type:      inviteType,
		MaxUses:   maxUses,
		ExpiresAt: expiresAt,
		Email:     email,
	}).MarshalVT()
	if err != nil {
		return nil, err
	}
	return c.doPost(ctx, "/api/org/"+orgID+"/invite", "application/octet-stream", body, nil, SeedReasonMutation)
}

// ResolveUsername resolves an exact username for an allowed context.
func (c *SessionClient) ResolveUsername(ctx context.Context, req *api.ResolveUsernameRequest) (*api.ResolveUsernameResponse, error) {
	var resp api.ResolveUsernameResponse
	if err := c.postMessage(ctx, "/api/account/username/resolve", req, &resp, SeedReasonMutation); err != nil {
		return nil, errors.Wrap(err, "resolve username")
	}
	return &resp, nil
}

// CreateTargetedInvitation creates a signed pending targeted invitation.
func (c *SessionClient) CreateTargetedInvitation(ctx context.Context, req *api.CreateTargetedInvitationRequest) (*api.CreateTargetedInvitationResponse, error) {
	var resp api.CreateTargetedInvitationResponse
	if err := c.postMessage(ctx, "/api/account/targeted-invitation", req, &resp, SeedReasonMutation); err != nil {
		return nil, errors.Wrap(err, "create targeted invitation")
	}
	return &resp, nil
}

// SignTargetedInvitationEnvelope signs a targeted invitation envelope with the
// current session key after clearing the signature field.
func (c *SessionClient) SignTargetedInvitationEnvelope(envelope *api.TargetedInvitationEnvelope) error {
	// Require an envelope and the session key.
	if envelope == nil {
		return errors.New("targeted invitation envelope is nil")
	}
	if c.priv == nil {
		return errors.New("no private key configured for signing")
	}

	// Sign the envelope without its signature field.
	payload, err := targetedInvitationSignaturePayload(envelope)
	if err != nil {
		return err
	}
	sig, err := c.priv.Sign(payload)
	if err != nil {
		return errors.Wrap(err, "sign targeted invitation envelope")
	}
	envelope.Signature = sig
	return nil
}

// VerifyTargetedInvitationEnvelope verifies a targeted invitation envelope
// signature against the embedded signer peer ID.
func VerifyTargetedInvitationEnvelope(envelope *api.TargetedInvitationEnvelope) error {
	// Require a signed envelope.
	if envelope == nil {
		return errors.New("targeted invitation envelope is nil")
	}
	if len(envelope.GetSignature()) == 0 {
		return errors.New("targeted invitation envelope signature is required")
	}

	// Check the signature against the signer's peer ID.
	pub, err := session.ExtractPublicKeyFromPeerID(envelope.GetSignerPeerId())
	if err != nil {
		return err
	}
	payload, err := targetedInvitationSignaturePayload(envelope)
	if err != nil {
		return err
	}
	valid, err := pub.Verify(payload, envelope.GetSignature())
	if err != nil {
		return errors.Wrap(err, "verify targeted invitation envelope")
	}
	if !valid {
		return errors.New("targeted invitation envelope signature is invalid")
	}
	return nil
}

// targetedInvitationSignaturePayload returns the signed bytes of a targeted
// invitation envelope: the signature context followed by the envelope encoded
// without its signature.
func targetedInvitationSignaturePayload(envelope *api.TargetedInvitationEnvelope) ([]byte, error) {
	// Require an envelope.
	if envelope == nil {
		return nil, errors.New("targeted invitation envelope is nil")
	}

	// Encode the envelope without its signature after the context.
	body := envelope.CloneVT()
	body.Signature = nil
	payload, err := body.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal targeted invitation envelope")
	}
	return append([]byte(targetedInvitationSignatureContext), payload...), nil
}

// ListTargetedInvitations lists the caller's targeted invitation inbox.
func (c *SessionClient) ListTargetedInvitations(ctx context.Context) (*api.ListTargetedInvitationsResponse, error) {
	var resp api.ListTargetedInvitationsResponse
	if err := c.getMessage(ctx, "/api/account/targeted-invitations", &resp, SeedReasonColdSeed); err != nil {
		return nil, errors.Wrap(err, "list targeted invitations")
	}
	return &resp, nil
}

// GetTargetedInvitation reads a single targeted invitation.
func (c *SessionClient) GetTargetedInvitation(ctx context.Context, id string) (*api.GetTargetedInvitationResponse, error) {
	var resp api.GetTargetedInvitationResponse
	if err := c.getMessage(ctx, "/api/account/targeted-invitation/"+url.PathEscape(id), &resp, SeedReasonColdSeed); err != nil {
		return nil, errors.Wrap(err, "get targeted invitation")
	}
	return &resp, nil
}

// RevokeTargetedInvitation revokes a pending targeted invitation.
func (c *SessionClient) RevokeTargetedInvitation(ctx context.Context, id string) (*api.RevokeTargetedInvitationResponse, error) {
	var resp api.RevokeTargetedInvitationResponse
	if err := c.postBody(ctx, "/api/account/targeted-invitation/"+url.PathEscape(id)+"/revoke", nil, &resp, SeedReasonMutation); err != nil {
		return nil, errors.Wrap(err, "revoke targeted invitation")
	}
	return &resp, nil
}

// ProcessTargetedInvitation applies a recipient lifecycle action.
func (c *SessionClient) ProcessTargetedInvitation(ctx context.Context, req *api.ProcessTargetedInvitationRequest) (*api.ProcessTargetedInvitationResponse, error) {
	var resp api.ProcessTargetedInvitationResponse
	if err := c.postMessage(ctx, "/api/account/targeted-invitation/"+url.PathEscape(req.GetId())+"/process", req, &resp, SeedReasonMutation); err != nil {
		return nil, errors.Wrap(err, "process targeted invitation")
	}
	return &resp, nil
}

// AcceptTargetedOrganizationInvitation fulfills a pending organization targeted
// invitation for the authenticated recipient.
func (c *SessionClient) AcceptTargetedOrganizationInvitation(ctx context.Context, orgID string, req *api.AcceptTargetedOrganizationInvitationRequest) (*api.AcceptTargetedOrganizationInvitationResponse, error) {
	var resp api.AcceptTargetedOrganizationInvitationResponse
	if err := c.postMessage(ctx, "/api/org/"+url.PathEscape(orgID)+"/targeted-invitation/fulfill", req, &resp, SeedReasonMutation); err != nil {
		return nil, errors.Wrap(err, "accept targeted organization invitation")
	}
	return &resp, nil
}

// JoinOrganization joins an organization via invite token.
func (c *SessionClient) JoinOrganization(ctx context.Context, token string) ([]byte, error) {
	body, err := (&api.JoinOrgRequest{Token: token}).MarshalVT()
	if err != nil {
		return nil, err
	}
	return c.doPost(ctx, "/api/org/join", "application/octet-stream", body, nil, SeedReasonMutation)
}

// UpdateOrganization updates an organization's display name.
func (c *SessionClient) UpdateOrganization(ctx context.Context, orgID, displayName string) (*api.UpdateOrgResponse, error) {
	var resp api.UpdateOrgResponse
	if err := c.postMessage(ctx, "/api/org/"+orgID+"/update", &api.UpdateOrgRequest{DisplayName: displayName}, &resp, SeedReasonMutation); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteOrganization deletes an organization.
func (c *SessionClient) DeleteOrganization(ctx context.Context, orgID string) (*api.OrgDeleteResponse, error) {
	var resp api.OrgDeleteResponse
	if err := c.postMessage(ctx, "/api/org/"+orgID+"/delete", &api.OrgDeleteRequest{}, &resp, SeedReasonMutation); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetOrganization retrieves organization info including members.
func (c *SessionClient) GetOrganization(ctx context.Context, orgID string) ([]byte, error) {
	return c.doGet(ctx, "/api/org/"+orgID, SeedReasonColdSeed)
}

// ListOrgInvites lists invites for an organization.
func (c *SessionClient) ListOrgInvites(ctx context.Context, orgID string) ([]byte, error) {
	return c.doGet(ctx, "/api/org/"+orgID+"/invites", SeedReasonColdSeed)
}

// RevokeOrgInvite revokes an invite by ID.
func (c *SessionClient) RevokeOrgInvite(ctx context.Context, orgID, inviteID string) (*api.CancelOrgInviteResponse, error) {
	var resp api.CancelOrgInviteResponse
	if err := c.deleteMessage(ctx, "/api/org/"+orgID+"/invite/"+inviteID, &resp, SeedReasonMutation); err != nil {
		return nil, err
	}
	return &resp, nil
}

// LeaveOrganization leaves an organization.
func (c *SessionClient) LeaveOrganization(ctx context.Context, orgID string) (*api.OrgLeaveResponse, error) {
	var resp api.OrgLeaveResponse
	if err := c.postMessage(ctx, "/api/org/"+orgID+"/leave", &api.OrgLeaveRequest{}, &resp, SeedReasonMutation); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RemoveOrgMember removes a member from an organization.
func (c *SessionClient) RemoveOrgMember(ctx context.Context, orgID, memberID string) (*api.RemoveOrgMemberResponse, error) {
	var resp api.RemoveOrgMemberResponse
	if err := c.deleteMessage(ctx, "/api/org/"+orgID+"/member/"+memberID, &resp, SeedReasonMutation); err != nil {
		return nil, err
	}
	return &resp, nil
}

// TransferResource transfers a resource to a typed principal owner.
// newOwnerType is "account" or "organization"; newOwnerID is the destination
// principal id (account ULID or org ULID).
func (c *SessionClient) TransferResource(ctx context.Context, resourceID, newOwnerType, newOwnerID string) ([]byte, error) {
	body, err := (&api.TransferResourceRequest{
		ResourceId:   resourceID,
		NewOwnerType: newOwnerType,
		NewOwnerId:   newOwnerID,
	}).MarshalVT()
	if err != nil {
		return nil, err
	}
	return c.doPost(
		ctx,
		"/api/resource/transfer",
		"application/octet-stream",
		body,
		nil,
		SeedReasonMutation,
	)
}

// ListManagedBillingAccounts lists billing accounts the caller manages
// (created_by_account_id = caller).
func (c *SessionClient) ListManagedBillingAccounts(ctx context.Context) ([]byte, error) {
	return c.doGet(ctx, "/api/billing/accounts", SeedReasonColdSeed)
}

// AssignBillingAccount binds a billing account to a principal.
// targetOwnerType is "account" or "organization"; targetOwnerID is the principal id.
func (c *SessionClient) AssignBillingAccount(ctx context.Context, billingAccountID, targetOwnerType, targetOwnerID string) ([]byte, error) {
	body, err := (&api.AssignBillingAccountRequest{
		BillingAccountId: billingAccountID,
		TargetOwnerType:  targetOwnerType,
		TargetOwnerId:    targetOwnerID,
	}).MarshalVT()
	if err != nil {
		return nil, err
	}
	return c.doPost(
		ctx,
		"/api/billing/assign",
		"application/octet-stream",
		body,
		nil,
		SeedReasonMutation,
	)
}

// DetachBillingAccount clears the billing account assignment on a principal.
func (c *SessionClient) DetachBillingAccount(ctx context.Context, targetOwnerType, targetOwnerID string) ([]byte, error) {
	body, err := (&api.DetachBillingAccountRequest{
		TargetOwnerType: targetOwnerType,
		TargetOwnerId:   targetOwnerID,
	}).MarshalVT()
	if err != nil {
		return nil, err
	}
	return c.doPost(
		ctx,
		"/api/billing/detach",
		"application/octet-stream",
		body,
		nil,
		SeedReasonMutation,
	)
}
