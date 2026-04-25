package provider_spacewave

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	alpha_nethttp "github.com/s4wave/spacewave/core/nethttp"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// maxResponseBodySize is the maximum HTTP response body size (10 MiB).
const maxResponseBodySize = 10 * 1024 * 1024

const (
	packReadTicketHeader   = "X-Sw-Pack-Read-Ticket"
	packReadTicketReuseTTL = 25 * time.Second
)

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

// cloudError is a structured error from the Spacewave cloud API.
type cloudError struct {
	// StatusCode is the HTTP status code.
	StatusCode int
	// Code is the error code from the cloud.
	Code string
	// Message is the human-readable error message.
	Message string
	// Retryable indicates whether the client should retry.
	Retryable bool
	// RetryAfterSeconds is the suggested retry delay.
	RetryAfterSeconds uint32
}

// Error returns a string representation of the cloud error.
func (e *cloudError) Error() string {
	msg := strconv.Itoa(e.StatusCode) + " " + e.Code + ": " + e.Message
	if e.RetryAfterSeconds > 0 {
		msg += " [retry_after=" + strconv.FormatUint(uint64(e.RetryAfterSeconds), 10) + "]"
	}
	return msg
}

// unauthCodes are error codes indicating the session key is stale but the
// account still exists. These are recoverable via reauthentication.
var unauthCodes = map[string]bool{
	"unknown_session":   true,
	"invalid_signature": true,
	"unknown_keypair":   true,
}

// refreshableWriteTicketCodes are error codes indicating a write ticket is
// stale, expired, or otherwise refreshable without full session
// reauthentication. These are handled by write-ticket refresh-and-retry paths,
// not by the account deletion or session reauthentication flows.
var refreshableWriteTicketCodes = map[string]bool{
	"invalid_write_ticket":               true,
	"expired_write_ticket":               true,
	"stale_write_ticket":                 true,
	"stale_session_account_write_ticket": true,
	"stale_resource_write_ticket":        true,
}

// deletedCodes are error codes indicating the account itself is gone.
// These are permanent and trigger the account deletion cascade.
var deletedCodes = map[string]bool{
	"account_not_found": true,
	"invalid_peer_id":   true,
	"unknown_entity":    true,
}

// blockedCodes are error codes indicating a resource is blocked (e.g. DMCA
// takedown). These are permanent until manually retried by the user.
var blockedCodes = map[string]bool{
	"dmca_blocked": true,
}

// permanentCodes is the union of unauthCodes, deletedCodes, and blockedCodes.
var permanentCodes = func() map[string]bool {
	m := make(map[string]bool, len(unauthCodes)+len(deletedCodes)+len(blockedCodes))
	for k := range unauthCodes {
		m[k] = true
	}
	for k := range deletedCodes {
		m[k] = true
	}
	for k := range blockedCodes {
		m[k] = true
	}
	return m
}()

// parseCloudError parses a cloud API error response body into a cloudError.
func parseCloudError(statusCode int, body []byte) *cloudError {
	ce := &cloudError{StatusCode: statusCode}
	var resp api.ErrorResponse
	if err := resp.UnmarshalJSON(body); err == nil {
		ce.Code = resp.GetCode()
		ce.Message = resp.GetMessage()
		ce.Retryable = resp.GetRetryable()
		ce.RetryAfterSeconds = resp.GetRetryAfterSeconds()
	}
	if permanentCodes[ce.Code] {
		ce.Retryable = false
	}
	return ce
}

// parseCloudResponseError parses a cloud API error response and retry hints.
func parseCloudResponseError(resp *http.Response, body []byte) *cloudError {
	ce := parseCloudError(resp.StatusCode, body)
	headerDelay := parseRetryAfterHeader(resp.Header.Get("Retry-After"), time.Now())
	if headerDelay <= 0 {
		return ce
	}
	headerSeconds := uint32(headerDelay / time.Second)
	if headerDelay%time.Second != 0 {
		headerSeconds++
	}
	if headerSeconds > ce.RetryAfterSeconds {
		ce.RetryAfterSeconds = headerSeconds
	}
	return ce
}

// parseRetryAfterHeader parses a Retry-After header as delay seconds or date.
func parseRetryAfterHeader(header string, now time.Time) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	seconds, err := strconv.ParseUint(header, 10, 32)
	if err == nil {
		return time.Duration(seconds) * time.Second
	}
	at, err := http.ParseTime(header)
	if err != nil {
		return 0
	}
	if !at.After(now) {
		return 0
	}
	return at.Sub(now)
}

// isNonRetryableCloudError checks if an error is a non-retryable cloud error.
func isNonRetryableCloudError(err error) bool {
	var ce *cloudError
	if errors.As(err, &ce) {
		return !ce.Retryable
	}
	return false
}

// isUnauthCloudError checks if an error indicates a stale session key
// (recoverable via reauthentication) as opposed to a deleted account.
func isUnauthCloudError(err error) bool {
	var ce *cloudError
	if errors.As(err, &ce) {
		return unauthCodes[ce.Code]
	}
	return false
}

// isRefreshableWriteTicketCloudError checks if an error indicates a
// write-ticket-specific refresh path should run instead of full
// reauthentication.
func isRefreshableWriteTicketCloudError(err error) bool {
	var ce *cloudError
	if errors.As(err, &ce) {
		return refreshableWriteTicketCodes[ce.Code]
	}
	return false
}

// isBlockedCloudError checks if an error indicates a resource is blocked
// (e.g. DMCA takedown). These errors are permanent until the user manually
// retries after the block is lifted.
func isBlockedCloudError(err error) bool {
	var ce *cloudError
	if errors.As(err, &ce) {
		return blockedCodes[ce.Code]
	}
	return false
}

// isCloudAccessGatedError checks if a cloud error depends on account/resource
// access state and should wait for an invalidation instead of retrying.
func isCloudAccessGatedError(err error) bool {
	var ce *cloudError
	if !errors.As(err, &ce) {
		return false
	}
	switch ce.Code {
	case "account_not_found",
		"account_read_only",
		"dmca_blocked",
		"insufficient_role",
		"rbac_denied",
		"resource_not_found",
		"subscription_readonly",
		"subscription_required":
		return true
	}
	return false
}

// isDirtySyncGatedCloudError checks if dirty sync should idle for access state.
func isDirtySyncGatedCloudError(err error) bool {
	return isCloudAccessGatedError(err)
}

// IsCloudErrorStatus returns true when err is a cloud error with the given HTTP status.
func IsCloudErrorStatus(err error, statusCode int) bool {
	var ce *cloudError
	return errors.As(err, &ce) && ce.StatusCode == statusCode
}

// WriteTicketProofPayloadFields contains the canonical fields signed for a
// write-ticket-authenticated hot write request.
type WriteTicketProofPayloadFields struct {
	Ticket        string
	Method        string
	Path          string
	TimestampMs   int64
	ContentLength int64
	BodyHashHex   string
	SignedHeaders map[string]string
}

// marshalWriteTicketProofPayload serializes a write-ticket proof payload to
// deterministic proto binary bytes for detached signing.
func marshalWriteTicketProofPayload(
	fields WriteTicketProofPayloadFields,
) ([]byte, error) {
	keys := make([]string, 0, len(fields.SignedHeaders))
	for k := range fields.SignedHeaders {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var hdrs strings.Builder
	for i, k := range keys {
		if i > 0 {
			hdrs.WriteByte(',')
		}
		hdrs.WriteString(k)
		hdrs.WriteByte('=')
		hdrs.WriteString(fields.SignedHeaders[k])
	}

	payload := &api.WriteTicketProofPayload{
		Ticket:        fields.Ticket,
		Method:        fields.Method,
		Path:          fields.Path,
		TimestampMs:   fields.TimestampMs,
		ContentLength: fields.ContentLength,
		BodyHashHex:   fields.BodyHashHex,
		SignedHeaders: hdrs.String(),
	}
	return payload.MarshalVT()
}

// buildWriteTicketProof signs a serialized proof payload and returns the
// detached envelope.
func buildWriteTicketProof(
	payload []byte,
	priv crypto.PrivKey,
) (*api.WriteTicketProof, error) {
	if priv == nil {
		return nil, errors.New("no private key configured for write ticket proof")
	}
	sig, err := priv.Sign(payload)
	if err != nil {
		return nil, errors.Wrap(err, "sign write ticket proof")
	}
	return &api.WriteTicketProof{
		Payload:   payload,
		Signature: sig,
	}, nil
}

// marshalSObjectWriteTicketProofPayload builds the canonical proof payload for
// shared-object hot writes. The signed metadata binds the request body hash
// directly to the presented ticket.
func marshalSObjectWriteTicketProofPayload(
	ticket string,
	method string,
	reqPath string,
	contentType string,
	body []byte,
	timestampMs int64,
) ([]byte, error) {
	h := sha256.Sum256(body)
	return marshalWriteTicketProofPayload(WriteTicketProofPayloadFields{
		Ticket:        ticket,
		Method:        method,
		Path:          reqPath,
		TimestampMs:   timestampMs,
		ContentLength: int64(len(body)),
		BodyHashHex:   hex.EncodeToString(h[:]),
		SignedHeaders: map[string]string{
			"content-type": contentType,
		},
	})
}

// marshalSyncPushWriteTicketProofPayload builds the canonical proof payload for
// sync/push hot writes. The signed metadata binds the precomputed body hash and
// the critical push headers without requiring the caller to re-hash the body.
func marshalSyncPushWriteTicketProofPayload(
	ticket string,
	method string,
	reqPath string,
	contentType string,
	contentLength int64,
	bodyHash []byte,
	packID string,
	blockCount int,
	bloomFilter []byte,
	timestampMs int64,
) ([]byte, error) {
	signedHeaders := map[string]string{
		"content-type":  contentType,
		"x-pack-id":     packID,
		"x-block-count": strconv.Itoa(blockCount),
	}
	if len(bloomFilter) != 0 {
		signedHeaders["x-bloom-filter"] = base64.StdEncoding.EncodeToString(
			bloomFilter,
		)
	}
	return marshalWriteTicketProofPayload(WriteTicketProofPayloadFields{
		Ticket:        ticket,
		Method:        method,
		Path:          reqPath,
		TimestampMs:   timestampMs,
		ContentLength: contentLength,
		BodyHashHex:   hex.EncodeToString(bodyHash),
		SignedHeaders: signedHeaders,
	})
}

// signedHeaders is the list of headers that are signed when present on a request.
var signedHeaders = []string{
	"content-type",
	"x-block-count",
	"x-bloom-filter",
	"x-pack-id",
}

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
}

// signRequest signs an HTTP request with the Ed25519 private key.
func (c *SignedHTTPClient) signRequest(req *http.Request, body []byte) error {
	h := sha256.Sum256(body)
	return c.signRequestPrecomputed(req, h[:], int64(len(body)))
}

// signRequestPrecomputed signs an HTTP request using a pre-computed body hash and content length.
func (c *SignedHTTPClient) signRequestPrecomputed(req *http.Request, bodyHash []byte, contentLength int64) error {
	if c.priv == nil {
		return errors.New("no private key configured for signing")
	}

	// Collect signed headers (only those present on the request).
	keys := make([]string, 0, len(signedHeaders))
	for _, name := range signedHeaders {
		if req.Header.Get(name) != "" {
			keys = append(keys, name)
		}
	}
	sort.Strings(keys)
	var hdrs strings.Builder
	for i, k := range keys {
		if i > 0 {
			hdrs.WriteByte(',')
		}
		hdrs.WriteString(k)
		hdrs.WriteByte('=')
		hdrs.WriteString(req.Header.Get(k))
	}

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

	// Sign with Ed25519.
	sig, err := c.priv.Sign(payloadBytes)
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

// Do signs and executes an HTTP request.
func (c *SignedHTTPClient) Do(req *http.Request) (*http.Response, error) {
	// Read body for signing if present.
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, errors.Wrap(err, "read request body")
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
	}

	if err := c.signRequest(req, body); err != nil {
		return nil, err
	}

	return c.httpCli.Do(req)
}

// doPost signs and executes a POST request, returning the response body.
// reason tags the request with X-Alpha-Seed-Reason when non-empty.
func (c *SignedHTTPClient) doPost(ctx context.Context, path string, contentType string, body []byte, headers map[string]string, reason SeedReason) ([]byte, error) {
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
	if reason != "" {
		req.Header.Set(SeedReasonHeader, string(reason))
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseCloudResponseError(resp, respBody)
	}
	return respBody, nil
}

// doPostJSON signs and executes a POST request with JSON content type, returning the response body.
// reason tags the request with X-Alpha-Seed-Reason when non-empty.
func (c *SignedHTTPClient) doPostJSON(ctx context.Context, path string, body []byte, reason SeedReason) ([]byte, error) {
	reqURL, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return nil, errors.Wrap(err, "build URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if reason != "" {
		req.Header.Set(SeedReasonHeader, string(reason))
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseCloudResponseError(resp, respBody)
	}
	return respBody, nil
}

// doPostBinary signs and executes a POST request with protobuf binary content
// type, returning the response body. The body should be the marshaled proto
// (Proto.MarshalVT()) and the response body is the raw proto-binary bytes for
// the caller to UnmarshalVT into the typed response message.
//
// headers carries any extra request headers (e.g., X-Turnstile-Token).
// reason tags the request with X-Alpha-Seed-Reason when non-empty.
func (c *SignedHTTPClient) doPostBinary(ctx context.Context, path string, body []byte, headers map[string]string, reason SeedReason) ([]byte, error) {
	reqURL, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return nil, errors.Wrap(err, "build URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Accept", "application/octet-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if reason != "" {
		req.Header.Set(SeedReasonHeader, string(reason))
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseCloudResponseError(resp, respBody)
	}
	return respBody, nil
}

// doPostStream signs and executes a streaming POST request.
// The bodyHash and contentLength are used for signing without reading the body into memory.
// reason tags the request with X-Alpha-Seed-Reason when non-empty.
func (c *SignedHTTPClient) doPostStream(ctx context.Context, path string, contentType string, body io.Reader, contentLength int64, bodyHash []byte, headers map[string]string, reason SeedReason) ([]byte, error) {
	reqURL, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return nil, errors.Wrap(err, "build URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, body)
	if err != nil {
		return nil, err
	}
	req.ContentLength = contentLength
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if reason != "" {
		req.Header.Set(SeedReasonHeader, string(reason))
	}

	if err := c.signRequestPrecomputed(req, bodyHash, contentLength); err != nil {
		return nil, err
	}

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseCloudResponseError(resp, respBody)
	}
	return respBody, nil
}

// doDelete signs and executes a DELETE request, returning the response body.
// reason tags the request with X-Alpha-Seed-Reason when non-empty.
func (c *SignedHTTPClient) doDelete(ctx context.Context, path string, reason SeedReason) ([]byte, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, errors.Wrap(err, "parse base URL")
	}
	ref, err := url.Parse(path)
	if err != nil {
		return nil, errors.Wrap(err, "parse path")
	}
	reqURL := base.ResolveReference(ref).String()

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	if reason != "" {
		req.Header.Set(SeedReasonHeader, string(reason))
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseCloudResponseError(resp, respBody)
	}
	return respBody, nil
}

// doGet signs and executes a GET request, returning the response body.
// reason tags the request with X-Alpha-Seed-Reason when non-empty.
func (c *SignedHTTPClient) doGet(ctx context.Context, path string, reason SeedReason) ([]byte, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, errors.Wrap(err, "parse base URL")
	}
	ref, err := url.Parse(path)
	if err != nil {
		return nil, errors.Wrap(err, "parse path")
	}
	reqURL := base.ResolveReference(ref).String()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		req.Header.Set(SeedReasonHeader, string(reason))
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseCloudResponseError(resp, respBody)
	}
	return respBody, nil
}

// doGetBinary signs and executes a GET request, advertising protobuf binary on
// the response. Returns the response body for the caller to UnmarshalVT into
// the typed response message.
//
// reason tags the request with X-Alpha-Seed-Reason when non-empty.
func (c *SignedHTTPClient) doGetBinary(ctx context.Context, path string, reason SeedReason) ([]byte, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, errors.Wrap(err, "parse base URL")
	}
	ref, err := url.Parse(path)
	if err != nil {
		return nil, errors.Wrap(err, "parse path")
	}
	reqURL := base.ResolveReference(ref).String()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	if reason != "" {
		req.Header.Set(SeedReasonHeader, string(reason))
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseCloudResponseError(resp, respBody)
	}
	return respBody, nil
}

// MultiSigContext is the signing context for multi-sig actions.
const MultiSigContext = "spacewave 2026-03-19 multi-sig action v2."

// BuildMultiSigPayload constructs the signing payload for a multi-sig action.
// The signing payload is: MultiSigContext || Timestamp.toBinary(signedAt) ||
// envelope. Must produce identical bytes as the TS server.
func BuildMultiSigPayload(signedAt *timestamppb.Timestamp, envelope []byte) []byte {
	ctx := []byte(MultiSigContext)
	ts, _ := signedAt.MarshalVT()
	payload := make([]byte, 0, len(ctx)+len(ts)+len(envelope))
	payload = append(payload, ctx...)
	payload = append(payload, ts...)
	payload = append(payload, envelope...)
	return payload
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

// initPrivKey initializes the private key from the peer if not already set.
func (c *EntityClient) initPrivKey(ctx context.Context, p peer.Peer) error {
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
	if c.priv == nil {
		return "", errors.New("no private key configured")
	}

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

	headers := make(map[string]string)
	if turnstileToken != "" {
		headers["X-Turnstile-Token"] = turnstileToken
	}
	if deviceTypeValue != "" {
		headers["X-Device-Type"] = deviceTypeValue
	}

	respBody, err := c.doPostBinary(ctx, "/api/account/register", body, headers, SeedReasonMutation)
	if err != nil {
		return "", errors.Wrap(err, "register account")
	}

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
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal session request")
	}

	headers := make(map[string]string)
	if turnstileToken != "" {
		headers["X-Turnstile-Token"] = turnstileToken
	}
	if deviceTypeValue != "" {
		headers["X-Device-Type"] = deviceTypeValue
	}

	respBody, err := c.doPostBinary(ctx, "/api/account/session/register", body, headers, SeedReasonMutation)
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

	var resp api.RegisterSessionResponse
	if err := resp.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal session response")
	}
	return &resp, nil
}

// signMultiSig signs envelope bytes with each entity key using the multi-sig
// signing context.
func (c *EntityClient) signMultiSig(
	envelope []byte,
	keys []crypto.PrivKey,
	peerIDs []string,
) ([]*api.EntitySignature, error) {
	if len(keys) != len(peerIDs) {
		return nil, errors.New("keys and peerIDs length mismatch")
	}
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
	msReq := &api.MultiSigRequest{Envelope: envBytes, Signatures: sigs}
	body, err := msReq.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal multi-sig request")
	}
	reqURL, err := url.JoinPath(c.baseURL, reqPath)
	if err != nil {
		return nil, errors.Wrap(err, "build URL")
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "create request")
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "multi-sig request")
	}
	defer alpha_nethttp.DrainAndCloseResponseBody(resp)
	respBody, readErr := readResponseBody(resp)
	if resp.StatusCode != http.StatusOK {
		if readErr != nil {
			return nil, readErr
		}
		return nil, parseCloudResponseError(resp, respBody)
	}
	if readErr != nil {
		return nil, readErr
	}
	out := &api.MultiSigActionResponse{}
	if len(respBody) != 0 {
		if err := out.UnmarshalVT(respBody); err != nil {
			return nil, errors.Wrap(err, "unmarshal multi-sig response")
		}
	}
	return out, nil
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
	result := resp.GetAccountDelete()
	if result == nil {
		return nil, errors.New("multi-sig response missing account delete result")
	}
	return result, nil
}

// SessionClient uses the session keypair for authenticated API calls.
type SessionClient struct {
	*SignedHTTPClient

	// executeWriteTicketAudience resolves and retries refreshable write tickets
	// for hot write routes when configured by ProviderAccount.
	executeWriteTicketAudience func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error

	// packReadTicketMtx guards packReadTickets
	packReadTicketMtx sync.Mutex
	// packReadTickets caches short-lived private pack read tickets by resource
	packReadTickets map[string]*packReadTicketState
}

type packReadTicketState struct {
	// ticket is the cached header value
	ticket string
	// exp is the local reuse deadline
	exp time.Time
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

func (c *SessionClient) getPackReadTicket(resourceID string) (string, bool) {
	c.packReadTicketMtx.Lock()
	defer c.packReadTicketMtx.Unlock()
	if c.packReadTickets == nil {
		return "", false
	}
	state, ok := c.packReadTickets[resourceID]
	if !ok {
		return "", false
	}
	if time.Now().After(state.exp) {
		delete(c.packReadTickets, resourceID)
		return "", false
	}
	return state.ticket, true
}

func (c *SessionClient) setPackReadTicket(resourceID string, ticket string) {
	if ticket == "" {
		return
	}
	c.packReadTicketMtx.Lock()
	if c.packReadTickets == nil {
		c.packReadTickets = make(map[string]*packReadTicketState)
	}
	c.packReadTickets[resourceID] = &packReadTicketState{
		ticket: ticket,
		exp:    time.Now().Add(packReadTicketReuseTTL),
	}
	c.packReadTicketMtx.Unlock()
}

func (c *SessionClient) signPackReadRequest(req *http.Request, resourceID string) error {
	if ticket, ok := c.getPackReadTicket(resourceID); ok {
		req.Header.Set(packReadTicketHeader, ticket)
		return nil
	}
	return c.signRequest(req, nil)
}

func (c *SessionClient) observePackReadResponse(resourceID string, resp *http.Response) {
	if resp == nil {
		return
	}
	c.setPackReadTicket(resourceID, resp.Header.Get(packReadTicketHeader))
}

// DoMultiSig sends a pre-signed multi-sig request to the cloud and returns the
// parsed MultiSigActionResponse envelope.
// Multi-sig routes authenticate via body signatures, not session headers.
func (c *SessionClient) DoMultiSig(ctx context.Context, method string, reqPath string, body []byte) (*api.MultiSigActionResponse, error) {
	reqURL, err := url.JoinPath(c.baseURL, reqPath)
	if err != nil {
		return nil, errors.Wrap(err, "build URL")
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "create request")
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set(SeedReasonHeader, string(SeedReasonMutation))
	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "multi-sig request")
	}
	defer alpha_nethttp.DrainAndCloseResponseBody(resp)
	respBody, readErr := readResponseBody(resp)
	if resp.StatusCode != http.StatusOK {
		if readErr != nil {
			return nil, readErr
		}
		return nil, parseCloudResponseError(resp, respBody)
	}
	if readErr != nil {
		return nil, readErr
	}
	out := &api.MultiSigActionResponse{}
	if len(respBody) != 0 {
		if err := out.UnmarshalVT(respBody); err != nil {
			return nil, errors.Wrap(err, "unmarshal multi-sig response")
		}
	}
	return out, nil
}

// GetSessionTicket requests a short-lived JWT ticket for WebSocket auth.
func (c *SessionClient) GetSessionTicket(ctx context.Context) (string, error) {
	body, err := (&api.SessionTicketRequest{}).MarshalVT()
	if err != nil {
		return "", errors.Wrap(err, "marshal session ticket request")
	}
	data, err := c.doPostBinary(ctx, "/api/session/ticket", body, nil, SeedReasonReconnect)
	if err != nil {
		return "", errors.Wrap(err, "get session ticket")
	}
	var resp api.TicketResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return "", errors.Wrap(err, "unmarshal ticket response")
	}
	if resp.GetTicket() == "" {
		return "", errors.New("empty ticket in response")
	}
	return resp.GetTicket(), nil
}

// GetWriteTicketBundle requests the bundled write tickets for a resource.
func (c *SessionClient) GetWriteTicketBundle(
	ctx context.Context,
	resourceID string,
) (*api.WriteTicketBundleResponse, error) {
	data, err := c.doPostBinary(
		ctx,
		path.Join("/api/session/write-tickets", resourceID),
		nil,
		nil,
		SeedReasonMutation,
	)
	if err != nil {
		return nil, errors.Wrap(err, "get write ticket bundle")
	}

	var resp api.WriteTicketBundleResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal write ticket bundle")
	}
	return &resp, nil
}

// GetWriteTicket requests a fresh ticket for one write-ticket audience.
func (c *SessionClient) GetWriteTicket(
	ctx context.Context,
	resourceID string,
	audience string,
) (string, error) {
	data, err := c.doPostBinary(
		ctx,
		path.Join("/api/session/write-ticket", resourceID, audience),
		nil,
		nil,
		SeedReasonMutation,
	)
	if err != nil {
		return "", errors.Wrap(err, "get write ticket")
	}

	var resp api.TicketResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return "", errors.Wrap(err, "unmarshal write ticket response")
	}
	if resp.GetTicket() == "" {
		return "", errors.New("empty ticket in response")
	}
	return resp.GetTicket(), nil
}

// EnableDirectWriteTickets configures the session client to fetch one
// write-ticket JWT per protected mutation directly from the cloud API.
//
// Standalone CLIs use this when they have a session keypair but do not have a
// ProviderAccount available to own cached write-ticket bundles.
func (c *SessionClient) EnableDirectWriteTickets() {
	if c == nil {
		return
	}
	c.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		if strings.TrimSpace(resourceID) == "" {
			return errors.New("missing resource id")
		}
		if fn == nil {
			return errors.New("missing write ticket callback")
		}
		if err := validateWriteTicketAudience(audience); err != nil {
			return err
		}

		ticket, err := c.GetWriteTicket(ctx, resourceID, string(audience))
		if err != nil {
			return err
		}
		err = fn(ticket)
		if !isRefreshableWriteTicketCloudError(err) {
			return err
		}

		ticket, err = c.GetWriteTicket(ctx, resourceID, string(audience))
		if err != nil {
			return err
		}
		return fn(ticket)
	}
}

func (c *SessionClient) postSObjectWriteWithTicket(
	ctx context.Context,
	soID string,
	action string,
	body []byte,
	ticket string,
) ([]byte, error) {
	if ticket == "" {
		return nil, errors.New("missing write ticket")
	}

	reqPath := path.Join("/api/sobject", soID, action)
	reqURL, err := url.JoinPath(c.baseURL, reqPath)
	if err != nil {
		return nil, errors.Wrap(err, "build URL")
	}

	payload, err := marshalSObjectWriteTicketProofPayload(
		ticket,
		http.MethodPost,
		reqPath,
		"application/octet-stream",
		body,
		time.Now().UnixMilli(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "marshal write ticket proof payload")
	}
	proof, err := buildWriteTicketProof(payload, c.priv)
	if err != nil {
		return nil, errors.Wrap(err, "build write ticket proof")
	}
	proofData, err := proof.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal write ticket proof")
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		reqURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, errors.Wrap(err, "create request")
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("X-Write-Ticket", ticket)
	req.Header.Set(
		"X-Write-Proof",
		base64.StdEncoding.EncodeToString(proofData),
	)
	req.Header.Set(SeedReasonHeader, string(SeedReasonMutation))

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "post sobject write")
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseCloudResponseError(resp, respBody)
	}
	return respBody, nil
}

// executeRequiredWriteTicketAudience executes a write-ticket-authenticated
// mutation and fails locally when the ticket executor is unavailable.
func (c *SessionClient) executeRequiredWriteTicketAudience(
	ctx context.Context,
	resourceID string,
	audience writeTicketAudience,
	fn func(ticket string) error,
) error {
	if c.executeWriteTicketAudience == nil {
		return errors.Errorf("missing write-ticket executor for %s", audience)
	}
	return c.executeWriteTicketAudience(ctx, resourceID, audience, fn)
}

func (c *SessionClient) postSyncPushWithTicket(
	ctx context.Context,
	resourceID string,
	packID string,
	blockCount int,
	body io.Reader,
	contentLength int64,
	bodyHash []byte,
	bloomFilter []byte,
	bloomFormatVersion uint32,
	ticket string,
) ([]byte, error) {
	if ticket == "" {
		return nil, errors.New("missing write ticket")
	}
	if len(bloomFilter) == 0 {
		return nil, errors.New("sync push bloom filter required")
	}
	if bloomFormatVersion == 0 {
		return nil, errors.New("sync push bloom_format_version required")
	}

	reqPath := path.Join("/api/bstore", resourceID, "sync/push")
	reqURL, err := url.JoinPath(c.baseURL, reqPath)
	if err != nil {
		return nil, errors.Wrap(err, "build URL")
	}

	payload, err := marshalSyncPushWriteTicketProofPayload(
		ticket,
		http.MethodPost,
		reqPath,
		"application/octet-stream",
		contentLength,
		bodyHash,
		packID,
		blockCount,
		bloomFilter,
		time.Now().UnixMilli(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "marshal write ticket proof payload")
	}
	proof, err := buildWriteTicketProof(payload, c.priv)
	if err != nil {
		return nil, errors.Wrap(err, "build write ticket proof")
	}
	proofData, err := proof.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal write ticket proof")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, body)
	if err != nil {
		return nil, errors.Wrap(err, "create request")
	}
	req.ContentLength = contentLength
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Sw-Hash", hex.EncodeToString(bodyHash))
	req.Header.Set("X-Pack-ID", packID)
	req.Header.Set("X-Block-Count", strconv.Itoa(blockCount))
	req.Header.Set(
		"X-Bloom-Filter",
		base64.StdEncoding.EncodeToString(bloomFilter),
	)
	req.Header.Set(
		"X-Bloom-Format-Version",
		strconv.FormatUint(uint64(bloomFormatVersion), 10),
	)
	req.Header.Set("X-Write-Ticket", ticket)
	req.Header.Set(
		"X-Write-Proof",
		base64.StdEncoding.EncodeToString(proofData),
	)
	req.Header.Set(SeedReasonHeader, string(SeedReasonMutation))

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "post sync push")
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseCloudResponseError(resp, respBody)
	}
	return respBody, nil
}

// SyncPush uploads a packfile to a resource-scoped block store.
// bodyHash is the pre-computed SHA-256 hash of the file contents.
// bloomFormatVersion identifies the bloom encoding (currently
// packfile.BloomFormatVersionV1).
func (c *SessionClient) SyncPush(ctx context.Context, resourceID string, packID string, blockCount int, packfilePath string, bodyHash []byte, bloomFilter []byte, bloomFormatVersion uint32) error {
	f, err := os.Open(packfilePath)
	if err != nil {
		return errors.Wrap(err, "open packfile")
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return errors.Wrap(err, "stat packfile")
	}

	var respData []byte
	err = c.executeRequiredWriteTicketAudience(
		ctx,
		resourceID,
		writeTicketAudienceBstoreSyncPush,
		func(ticket string) error {
			var postErr error
			respData, postErr = c.postSyncPushWithTicket(
				ctx,
				resourceID,
				packID,
				blockCount,
				f,
				stat.Size(),
				bodyHash,
				bloomFilter,
				bloomFormatVersion,
				ticket,
			)
			return postErr
		},
	)
	if err != nil {
		return errors.Wrap(err, "sync push")
	}

	if len(respData) > 0 {
		resp := &packfile.PushResponse{}
		if err := resp.UnmarshalVT(respData); err != nil {
			return errors.Wrap(err, "unmarshal push response")
		}
	}
	return nil
}

// SyncPushData uploads an in-memory packfile to a resource-scoped block store.
// bodyHash is the pre-computed SHA-256 hash of the file contents.
// bloomFormatVersion identifies the bloom encoding (currently
// packfile.BloomFormatVersionV1).
func (c *SessionClient) SyncPushData(ctx context.Context, resourceID string, packID string, blockCount int, packData []byte, bodyHash []byte, bloomFilter []byte, bloomFormatVersion uint32) error {
	return c.syncPushDataWithProgress(ctx, resourceID, packID, blockCount, packData, bodyHash, bloomFilter, bloomFormatVersion, nil)
}

func (c *SessionClient) syncPushDataWithProgress(
	ctx context.Context,
	resourceID string,
	packID string,
	blockCount int,
	packData []byte,
	bodyHash []byte,
	bloomFilter []byte,
	bloomFormatVersion uint32,
	progress func(int64),
) error {
	var respData []byte
	var err error
	err = c.executeRequiredWriteTicketAudience(
		ctx,
		resourceID,
		writeTicketAudienceBstoreSyncPush,
		func(ticket string) error {
			var postErr error
			var body io.Reader = bytes.NewReader(packData)
			if progress != nil {
				body = newSyncPushProgressReader(body, progress)
			}
			respData, postErr = c.postSyncPushWithTicket(
				ctx,
				resourceID,
				packID,
				blockCount,
				body,
				int64(len(packData)),
				bodyHash,
				bloomFilter,
				bloomFormatVersion,
				ticket,
			)
			return postErr
		},
	)
	if err != nil {
		return errors.Wrap(err, "sync push")
	}

	if len(respData) > 0 {
		resp := &packfile.PushResponse{}
		if err := resp.UnmarshalVT(respData); err != nil {
			return errors.Wrap(err, "unmarshal push response")
		}
	}
	return nil
}

// SyncPull retrieves packfile entries from a resource-scoped block store since the given ID.
func (c *SessionClient) SyncPull(ctx context.Context, resourceID string, since string) ([]byte, error) {
	p := path.Join("/api/bstore", resourceID, "sync/pull")
	if since != "" {
		p += "?since=" + url.QueryEscape(since)
	}

	data, err := c.doGet(ctx, p, SeedReasonColdSeed)
	if err != nil {
		return nil, errors.Wrap(err, "sync pull")
	}
	return data, nil
}

// PostOp posts an operation to a shared object.
func (c *SessionClient) PostOp(ctx context.Context, soID string, opData []byte) error {
	err := c.executeRequiredWriteTicketAudience(
		ctx,
		soID,
		writeTicketAudienceSOOp,
		func(ticket string) error {
			data, err := c.postSObjectWriteWithTicket(ctx, soID, "op", opData, ticket)
			if err != nil {
				return err
			}
			var resp api.SubmitOpResponse
			if err := resp.UnmarshalVT(data); err != nil {
				return errors.Wrap(err, "unmarshal submit op response")
			}
			return nil
		},
	)
	return errors.Wrap(err, "post op")
}

// PostRoot posts a root state update to a shared object.
func (c *SessionClient) PostRoot(
	ctx context.Context,
	soID string,
	root *sobject.SORoot,
	rejectedOps []*sobject.SOOperationRejection,
) error {
	body, err := marshalPostRootRequest(root, rejectedOps)
	if err != nil {
		return errors.Wrap(err, "marshal post root request")
	}
	err = c.executeRequiredWriteTicketAudience(
		ctx,
		soID,
		writeTicketAudienceSORoot,
		func(ticket string) error {
			data, err := c.postSObjectWriteWithTicket(ctx, soID, "root", body, ticket)
			if err != nil {
				return err
			}
			var resp api.SubmitRootResponse
			if err := resp.UnmarshalVT(data); err != nil {
				return errors.Wrap(err, "unmarshal submit root response")
			}
			return nil
		},
	)
	return errors.Wrap(err, "post root")
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
	body, err := (&api.ClientErrorReportRequest{
		ErrorCode:    errorCode,
		Component:    component,
		ResourceType: resourceType,
		ResourceId:   resourceID,
		Detail:       detail,
	}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal client error report request")
	}
	data, err := c.doPostBinary(
		ctx,
		"/api/account/client-error-report",
		body,
		nil,
		SeedReasonMutation,
	)
	if err != nil {
		return errors.Wrap(err, "post client error report")
	}
	var resp api.ClientErrorReportResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "unmarshal client error report response")
	}
	return nil
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
	body, err := (&api.CreateSObjectRequest{
		DisplayName:    displayName,
		ObjectType:     objectType,
		OwnerType:      ownerType,
		OwnerId:        ownerID,
		AccountPrivate: accountPrivate,
	}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal create request")
	}
	data, err := c.doPostBinary(ctx, path.Join("/api/sobject", soID, "create"), body, nil, SeedReasonMutation)
	if err != nil {
		return errors.Wrap(err, "create shared object")
	}
	var resp api.CreateSObjectResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "unmarshal create shared object response")
	}
	return nil
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
	p := path.Join("/api/sobject", soID, "state")
	if since > 0 {
		p += "?since=" + strconv.FormatUint(since, 10)
	}
	data, err := c.doGet(ctx, p, reason)
	if err != nil {
		return nil, errors.Wrap(err, "get so state")
	}
	return data, nil
}

// PostInitState posts the initial root state for a newly created shared object.
func (c *SessionClient) PostInitState(ctx context.Context, soID string, rootData []byte) error {
	root := &sobject.SORoot{}
	if err := root.UnmarshalVT(rootData); err != nil {
		return errors.Wrap(err, "unmarshal root")
	}
	body, err := marshalPostRootRequest(root, nil)
	if err != nil {
		return errors.Wrap(err, "marshal post root request")
	}
	err = c.executeRequiredWriteTicketAudience(
		ctx,
		soID,
		writeTicketAudienceSORoot,
		func(ticket string) error {
			data, err := c.postSObjectWriteWithTicket(ctx, soID, "root", body, ticket)
			if err != nil {
				return err
			}
			var resp api.SubmitRootResponse
			if err := resp.UnmarshalVT(data); err != nil {
				return errors.Wrap(err, "unmarshal submit root response")
			}
			return nil
		},
	)
	return errors.Wrap(err, "post init state")
}

func marshalPostRootRequest(
	root *sobject.SORoot,
	rejectedOps []*sobject.SOOperationRejection,
) ([]byte, error) {
	return (&api.PostRootRequest{
		Root:        root,
		RejectedOps: rejectedOps,
	}).MarshalVT()
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
	data, err := c.doPostBinary(ctx, path.Join("/api/sobject", soID, "config"), configData, nil, SeedReasonMutation)
	if err != nil {
		return errors.Wrap(err, "post config")
	}
	var resp api.PostConfigResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "unmarshal post config response")
	}
	return nil
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
	body, err := req.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal config state request")
	}
	data, err := c.doPostBinary(
		ctx,
		path.Join("/api/sobject", soID, "config-state"),
		body,
		nil,
		SeedReasonMutation,
	)
	if err != nil {
		return errors.Wrap(err, "post config state")
	}
	var resp api.PostConfigStateResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "unmarshal post config state response")
	}
	return nil
}

// PostKeyEpoch posts a new key epoch (after key rotation) to the server.
func (c *SessionClient) PostKeyEpoch(
	ctx context.Context,
	soID string,
	epoch *sobject.SOKeyEpoch,
	recoveryEnvelopes []*sobject.SOEntityRecoveryEnvelope,
) error {
	body, err := (&api.PostKeyEpochRequest{
		KeyEpoch:          epoch,
		RecoveryEnvelopes: recoveryEnvelopes,
	}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal key epoch request")
	}
	data, err := c.doPostBinary(ctx, path.Join("/api/sobject", soID, "key-epoch"), body, nil, SeedReasonMutation)
	if err != nil {
		return errors.Wrap(err, "post key epoch")
	}
	var resp api.PostKeyEpochResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "unmarshal post key epoch response")
	}
	return nil
}

// EnrollMember resolves all registered session peer IDs for a target account
// on a given shared object. The SO DO queries D1 sessions for the account.
// Returns the list of session peers keyed by peer_id.
func (c *SessionClient) EnrollMember(ctx context.Context, soID, accountID string, ignoreExclusion bool) (*api.EnrollMemberResponse, error) {
	body, err := (&api.EnrollMemberRequest{
		AccountId:       accountID,
		IgnoreExclusion: ignoreExclusion,
	}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal enroll member request")
	}
	data, err := c.doPost(ctx, path.Join("/api/sobject", soID, "enroll-member"), "application/octet-stream", body, nil, SeedReasonRejoin)
	if err != nil {
		return nil, err
	}
	resp := &api.EnrollMemberResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal enroll member response")
	}
	return resp, nil
}

// ResolveMemberParticipants resolves the current SO participant peer IDs for a
// target account on a given shared object.
func (c *SessionClient) ResolveMemberParticipants(ctx context.Context, soID, accountID string) (*api.ResolveMemberParticipantsResponse, error) {
	body, err := (&api.ResolveMemberParticipantsRequest{AccountId: accountID}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal resolve member participants request")
	}
	data, err := c.doPost(ctx, path.Join("/api/sobject", soID, "member-participants"), "application/octet-stream", body, nil, SeedReasonRejoin)
	if err != nil {
		return nil, err
	}
	resp := &api.ResolveMemberParticipantsResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal resolve member participants response")
	}
	return resp, nil
}

// ListSORecoveryEntityKeypairs retrieves current keypairs for readable entity
// participants on a shared object.
func (c *SessionClient) ListSORecoveryEntityKeypairs(
	ctx context.Context,
	soID string,
) (*api.ListSORecoveryEntityKeypairsResponse, error) {
	data, err := c.doGet(ctx, path.Join("/api/sobject", soID, "recovery-entity-keypairs"), SeedReasonRejoin)
	if err != nil {
		return nil, errors.Wrap(err, "list recovery entity keypairs")
	}
	resp := &api.ListSORecoveryEntityKeypairsResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal recovery entity keypairs response")
	}
	return resp, nil
}

// GetSORecoveryEnvelope retrieves the current recovery envelope for the
// authenticated entity on a shared object.
func (c *SessionClient) GetSORecoveryEnvelope(
	ctx context.Context,
	soID string,
) (*sobject.SOEntityRecoveryEnvelope, error) {
	data, err := c.doGet(ctx, path.Join("/api/sobject", soID, "recovery-envelope"), SeedReasonRejoin)
	if err != nil {
		return nil, errors.Wrap(err, "get recovery envelope")
	}
	resp := &api.GetSORecoveryEnvelopeResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal recovery envelope response")
	}
	if resp.GetEnvelope() == nil {
		return nil, errors.New("recovery envelope missing from response")
	}
	return resp.GetEnvelope(), nil
}

// RegisterInviteCode registers a short invite code on the cloud SO DO.
// The code maps to the full serialized SOInviteMessage for lookup.
func (c *SessionClient) RegisterInviteCode(ctx context.Context, soID string, req *api.RegisterInviteCodeRequest) error {
	body, err := req.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal register invite code request")
	}
	data, err := c.doPost(ctx, path.Join("/api/sobject", soID, "invite-code"), "application/octet-stream", body, nil, SeedReasonMutation)
	if err != nil {
		return errors.Wrap(err, "register invite code")
	}
	var resp api.RegisterInviteCodeResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "unmarshal register invite code response")
	}
	return nil
}

// RegisterInviteBeacon registers an owner-side invite beacon for mailbox joins.
func (c *SessionClient) RegisterInviteBeacon(
	ctx context.Context,
	soID string,
	inviteID string,
	tokenHashHex string,
	expiresAtMs int64,
) error {
	body, err := (&api.InviteBeaconRequest{
		InviteId:  inviteID,
		TokenHash: tokenHashHex,
		ExpiresAt: expiresAtMs,
	}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal invite beacon request")
	}
	data, err := c.doPostBinary(
		ctx,
		path.Join("/api/sobject", soID, "invite-beacon"),
		body,
		nil,
		SeedReasonMutation,
	)
	if err != nil {
		return errors.Wrap(err, "register invite beacon")
	}
	var resp api.InviteBeaconResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "unmarshal invite beacon response")
	}
	return nil
}

// LookupInviteCode resolves a short invite code to the full SOInviteMessage.
func (c *SessionClient) LookupInviteCode(ctx context.Context, code string) (*api.LookupInviteCodeResponse, error) {
	respBody, err := c.doGet(ctx, "/api/sobject/lookup-code?code="+url.QueryEscape(code), SeedReasonColdSeed)
	if err != nil {
		return nil, errors.Wrap(err, "lookup invite code")
	}
	resp := &api.LookupInviteCodeResponse{}
	if err := resp.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal lookup invite code response")
	}
	return resp, nil
}

// GetMailboxEntries returns pending mailbox entries for a shared object.
func (c *SessionClient) GetMailboxEntries(ctx context.Context, soID string) (*api.GetMailboxResponse, error) {
	respBody, err := c.doGet(ctx, "/api/sobject/"+soID+"/invite-mailbox?status=pending", SeedReasonColdSeed)
	if err != nil {
		return nil, errors.Wrap(err, "get mailbox entries")
	}
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
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal submit mailbox request")
	}
	respBody, err := c.doPost(ctx, "/api/sobject/"+soID+"/invite-mailbox", "application/octet-stream", body, nil, SeedReasonMutation)
	if err != nil {
		return nil, errors.Wrap(err, "submit mailbox entry")
	}
	resp := &api.SubmitMailboxEntryResponse{}
	if err := resp.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal submit mailbox response")
	}
	return resp, nil
}

// ProcessMailboxEntry processes a mailbox entry and returns the resulting status.
func (c *SessionClient) ProcessMailboxEntry(
	ctx context.Context,
	soID string,
	req *api.ProcessMailboxEntryRequest,
) (*api.ProcessMailboxEntryResponse, error) {
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal process mailbox request")
	}
	respBody, err := c.doPost(ctx, "/api/sobject/"+soID+"/invite-mailbox/process", "application/octet-stream", body, nil, SeedReasonMutation)
	if err != nil {
		return nil, errors.Wrap(err, "process mailbox entry")
	}
	resp := &api.ProcessMailboxEntryResponse{}
	if err := resp.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal process mailbox response")
	}
	return resp, nil
}

// CreateCheckoutSession creates or resumes a Stripe Checkout Session.
// Returns the checkout URL, a WebSocket ticket, and the attempt status.
// billingAccountID selects a specific BA; empty falls back to the caller's
// single managed BA (legacy default).
func (c *SessionClient) CreateCheckoutSession(
	ctx context.Context,
	successURL string,
	cancelURL string,
	interval s4wave_provider_spacewave.BillingInterval,
	billingAccountID string,
) (*api.CheckoutResponse, error) {
	body, err := (&api.CheckoutRequest{
		SuccessUrl:       successURL,
		CancelUrl:        cancelURL,
		BillingInterval:  interval,
		BillingAccountId: billingAccountID,
	}).MarshalVT()
	if err != nil {
		return nil, err
	}
	data, err := c.doPost(ctx, "/api/billing/checkout", "application/octet-stream", body, nil, SeedReasonMutation)
	if err != nil {
		return nil, err
	}
	var resp api.CheckoutResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateBillingAccount creates a new unassigned billing account owned
// (managed) by the caller. Returns the new BA's ULID.
func (c *SessionClient) CreateBillingAccount(ctx context.Context, displayName string) (string, error) {
	body, err := (&api.CreateBillingAccountRequest{
		DisplayName: displayName,
	}).MarshalVT()
	if err != nil {
		return "", err
	}
	data, err := c.doPost(ctx, "/api/billing/accounts", "application/octet-stream", body, nil, SeedReasonMutation)
	if err != nil {
		return "", err
	}
	var resp api.CreateBillingAccountResponse
	if err := resp.UnmarshalVT(data); err != nil {
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
	data, err := c.doDelete(ctx, "/api/billing/accounts/"+baID, SeedReasonMutation)
	if err != nil {
		return err
	}
	var resp api.DeleteBillingAccountResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return err
	}
	return nil
}

// CancelCheckoutSession cancels pending checkout attempts and expires the
// Stripe session. Returns 'completed' if the subscription activated during
// the race window.
func (c *SessionClient) CancelCheckoutSession(ctx context.Context) (*api.CheckoutResponse, error) {
	data, err := c.doDelete(ctx, "/api/billing/checkout", SeedReasonMutation)
	if err != nil {
		return nil, err
	}
	var resp api.CheckoutResponse
	if err := resp.UnmarshalVT(data); err != nil {
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
	body, err := (&api.CancelBillingRequest{}).MarshalVT()
	if err != nil {
		return nil, err
	}
	data, err := c.doPostBinary(ctx, "/api/billing/"+baID+"/cancel", body, nil, SeedReasonMutation)
	if err != nil {
		return nil, err
	}
	resp := &api.CancelBillingResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal cancel billing response")
	}
	return resp, nil
}

// ReactivateSubscription reactivates a canceled billing account subscription.
func (c *SessionClient) ReactivateSubscription(ctx context.Context, baID string) (*api.ReactivateBillingResponse, error) {
	body, err := (&api.ReactivateBillingRequest{}).MarshalVT()
	if err != nil {
		return nil, err
	}
	data, err := c.doPostBinary(ctx, "/api/billing/"+baID+"/reactivate", body, nil, SeedReasonMutation)
	if err != nil {
		return nil, err
	}
	resp := &api.ReactivateBillingResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal reactivate billing response")
	}
	return resp, nil
}

// SwitchBillingInterval switches the billing interval for a subscription.
func (c *SessionClient) SwitchBillingInterval(
	ctx context.Context,
	baID string,
	interval s4wave_provider_spacewave.BillingInterval,
) ([]byte, error) {
	body, err := (&api.SwitchIntervalRequest{BillingInterval: interval}).MarshalVT()
	if err != nil {
		return nil, err
	}
	return c.doPost(
		ctx,
		"/api/billing/"+baID+"/switch-interval",
		"application/octet-stream",
		body,
		nil,
		SeedReasonMutation,
	)
}

// CreateBillingPortal creates a Stripe billing portal session and returns the URL.
func (c *SessionClient) CreateBillingPortal(ctx context.Context, baID string) (string, error) {
	body, err := (&api.BillingPortalRequest{}).MarshalVT()
	if err != nil {
		return "", err
	}
	data, err := c.doPostBinary(ctx, "/api/billing/"+baID+"/portal", body, nil, SeedReasonMutation)
	if err != nil {
		return "", err
	}
	var resp api.BillingPortalResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return "", errors.Wrap(err, "unmarshal portal response")
	}
	return resp.GetUrl(), nil
}

// GetAccountInfo retrieves account info from the cloud.
func (c *SessionClient) GetAccountInfo(ctx context.Context) (*api.AccountInfoResponse, error) {
	data, err := c.doGetBinary(ctx, "/api/account/info", SeedReasonColdSeed)
	if err != nil {
		return nil, errors.Wrap(err, "get account info")
	}
	var resp api.AccountInfoResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal account info")
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
	data, err := c.doGetBinary(ctx, "/api/account/sessions", SeedReasonColdSeed)
	if err != nil {
		return nil, errors.Wrap(err, "list sessions")
	}
	var resp api.ListAccountSessionsResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal sessions response")
	}
	return resp.GetSessions(), nil
}

// ListKeypairs retrieves entity keypairs from the cloud.
func (c *SessionClient) ListKeypairs(ctx context.Context) ([]*session.EntityKeypair, error) {
	data, err := c.doGetBinary(ctx, "/api/account/keypairs", SeedReasonColdSeed)
	if err != nil {
		return nil, errors.Wrap(err, "list keypairs")
	}
	var resp api.ListKeypairsResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal keypair list")
	}
	return resp.GetKeypairs(), nil
}

// GetAccountState retrieves combined account info and keypairs from the cloud.
func (c *SessionClient) GetAccountState(ctx context.Context) (*api.AccountStateResponse, error) {
	data, err := c.doGetBinary(ctx, "/api/account/state", SeedReasonColdSeed)
	if err != nil {
		return nil, errors.Wrap(err, "get account state")
	}
	var resp api.AccountStateResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal account state")
	}
	return &resp, nil
}

// EnsureAccountSObjectBinding reserves or returns an account-owned shared object
// binding for the requested purpose.
func (c *SessionClient) EnsureAccountSObjectBinding(
	ctx context.Context,
	purpose string,
) (*api.AccountSObjectBinding, error) {
	body, err := (&api.EnsureAccountSObjectBindingRequest{
		Purpose: purpose,
	}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal ensure account sobject binding request")
	}
	data, err := c.doPost(
		ctx,
		"/api/account/sobject-binding/ensure",
		"application/octet-stream",
		body,
		nil,
		SeedReasonMutation,
	)
	if err != nil {
		return nil, errors.Wrap(err, "ensure account sobject binding")
	}
	var resp api.EnsureAccountSObjectBindingResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal ensure account sobject binding response")
	}
	if resp.GetBinding() == nil {
		return nil, errors.New("missing account sobject binding in response")
	}
	return resp.GetBinding(), nil
}

// FinalizeAccountSObjectBinding marks a reserved account-owned shared object
// binding ready after client-signed initialization succeeds.
func (c *SessionClient) FinalizeAccountSObjectBinding(
	ctx context.Context,
	purpose string,
	soID string,
) (*api.AccountSObjectBinding, error) {
	body, err := (&api.FinalizeAccountSObjectBindingRequest{
		Purpose: purpose,
		SoId:    soID,
	}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal finalize account sobject binding request")
	}
	data, err := c.doPost(
		ctx,
		"/api/account/sobject-binding/finalize",
		"application/octet-stream",
		body,
		nil,
		SeedReasonMutation,
	)
	if err != nil {
		return nil, errors.Wrap(err, "finalize account sobject binding")
	}
	var resp api.FinalizeAccountSObjectBindingResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal finalize account sobject binding response")
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
	body, err := (&api.ReinitializeSObjectRequest{}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal reinitialize shared object request")
	}
	data, err := c.doPostBinary(ctx, "/api/sobject/"+soID+"/reinitialize", body, nil, SeedReasonMutation)
	if err != nil {
		return errors.Wrap(err, "reinitialize shared object")
	}
	var resp api.ReinitializeSObjectResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "unmarshal reinitialize shared object response")
	}
	return nil
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
	body, err := (&api.UpdateOrgRequest{DisplayName: displayName}).MarshalVT()
	if err != nil {
		return nil, err
	}
	data, err := c.doPostBinary(ctx, "/api/org/"+orgID+"/update", body, nil, SeedReasonMutation)
	if err != nil {
		return nil, err
	}
	var resp api.UpdateOrgResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal update organization response")
	}
	return &resp, nil
}

// DeleteOrganization deletes an organization.
func (c *SessionClient) DeleteOrganization(ctx context.Context, orgID string) (*api.OrgDeleteResponse, error) {
	body, err := (&api.OrgDeleteRequest{}).MarshalVT()
	if err != nil {
		return nil, err
	}
	data, err := c.doPostBinary(ctx, "/api/org/"+orgID+"/delete", body, nil, SeedReasonMutation)
	if err != nil {
		return nil, err
	}
	var resp api.OrgDeleteResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal delete organization response")
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
	data, err := c.doDelete(ctx, "/api/org/"+orgID+"/invite/"+inviteID, SeedReasonMutation)
	if err != nil {
		return nil, err
	}
	var resp api.CancelOrgInviteResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal cancel org invite response")
	}
	return &resp, nil
}

// LeaveOrganization leaves an organization.
func (c *SessionClient) LeaveOrganization(ctx context.Context, orgID string) (*api.OrgLeaveResponse, error) {
	body, err := (&api.OrgLeaveRequest{}).MarshalVT()
	if err != nil {
		return nil, err
	}
	data, err := c.doPostBinary(ctx, "/api/org/"+orgID+"/leave", body, nil, SeedReasonMutation)
	if err != nil {
		return nil, err
	}
	var resp api.OrgLeaveResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal leave organization response")
	}
	return &resp, nil
}

// RemoveOrgMember removes a member from an organization.
func (c *SessionClient) RemoveOrgMember(ctx context.Context, orgID, memberID string) (*api.RemoveOrgMemberResponse, error) {
	data, err := c.doDelete(ctx, "/api/org/"+orgID+"/member/"+memberID, SeedReasonMutation)
	if err != nil {
		return nil, err
	}
	var resp api.RemoveOrgMemberResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal remove org member response")
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
