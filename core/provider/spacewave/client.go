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
	"slices"
	"strconv"
	"strings"
	"time"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	"github.com/s4wave/spacewave/core/provider/spacewave/entitykeystore"
	"github.com/s4wave/spacewave/net/httpclient"
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

// IsCloudErrorStatus returns true when err is a cloud error with the given HTTP
// status.
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

// signedHeaders is the list of headers that are signed when present on a
// request.
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
	// httpCli is the underlying HTTP client.
	httpCli *http.Client
	// baseURL is the API base URL.
	baseURL string
	// envPfx is the environment prefix for signed payloads.
	envPfx string
	// signer holds the revocable signing capability for this client.
	signer *SessionSigner
}

// signRequest signs an HTTP request with the Ed25519 private key.
func (c *SignedHTTPClient) signRequest(req *http.Request, body []byte) error {
	h := sha256.Sum256(body)
	return c.signRequestPrecomputed(req, h[:], int64(len(body)))
}

// signRequestPrecomputed signs an HTTP request using a pre-computed body hash
// and content length.
func (c *SignedHTTPClient) signRequestPrecomputed(req *http.Request, bodyHash []byte, contentLength int64) error {
	// Refuse to sign without a key or an external signer.
	if !c.signer.CanSign() {
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
	// Proto binary serialization is deterministic: both Go and TS produce
	// identical bytes.
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
	sig, err := c.signer.Sign(req.Context(), payloadBytes)
	if err != nil {
		return errors.Wrap(err, "sign payload")
	}

	// Set auth headers.
	req.Header.Set("X-Peer-ID", c.signer.GetPeerID().String())
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
// specifies, up to rateLimitRetries times while that delay is within
// rateLimitMaxWait. Otherwise, or when the request context ends during the
// delay, the 429 response is returned to the caller.
func (c *SignedHTTPClient) Do(req *http.Request) (response *http.Response, resultErr error) {
	// Cancel signed requests on revocation, retaining the binding through body reads.
	ctx, release := c.signer.bindRequestContext(req.Context())
	if release != nil {
		req = req.WithContext(ctx)
		defer func() {
			if response == nil || response.Body == nil {
				release()
				return
			}
			response.Body = &credentialResponseBody{ReadCloser: response.Body, release: release}
		}()
	}

	// Read the body once so each attempt can sign and send it.
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, errors.Wrap(err, "read request body")
		}
	}

	// Retry only the short rate-limit responses accepted by the client policy.
	for attempt := 0; ; attempt++ {
		// Sign and send this attempt. GetBody lets the transport resend an
		// idempotent request when the server closes a reused connection.
		if body != nil {
			req.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(body)), nil
			}
			req.Body, _ = req.GetBody()
		}
		if err := c.signRequest(req, body); err != nil {
			return nil, err
		}
		resp, err := c.httpCli.Do(req) //nolint:gosec // This outbound HTTP client intentionally accepts caller-selected requests.
		if err != nil || resp.StatusCode != http.StatusTooManyRequests || attempt == rateLimitRetries {
			return resp, err
		}

		// Return the response unless it specifies a short rate limit delay.
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
		//gostyle:timer deadline
		case <-time.After(delay):
		}
	}
}

// rateLimitDelay returns the delay a retryable rate_limited response specifies,
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
	ce := clouderror.ParseResponse(resp, body)
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
		return nil, clouderror.ParseResponse(resp, respBody)
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
		return nil, clouderror.ParseResponse(resp, respBody)
	}

	// Decode the action response. An empty body carries no result.
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

// DefaultSigningEnvPrefix is the default request-signing environment prefix.
const DefaultSigningEnvPrefix = "spacewave"

// normalizeSigningEnvPrefix returns signingEnvPfx, or the default prefix when
// it is empty.
func normalizeSigningEnvPrefix(signingEnvPfx string) string {
	if signingEnvPfx == "" {
		return DefaultSigningEnvPrefix
	}
	return signingEnvPfx
}
