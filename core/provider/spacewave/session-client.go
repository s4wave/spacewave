package provider_spacewave

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/syncprogress"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/httpclient"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

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
	uploaded, err := c.syncPushUpload(ctx, resourceID, pack, body, size)
	if err != nil || !uploaded {
		return err
	}
	return c.SyncPushCommit(ctx, resourceID, pack.packID)
}

// SyncPushUpload runs the first two steps of a push of an in-memory pack: the
// admission and the upload. It reports whether the pack awaits SyncPushCommit;
// a pack the catalog already holds does not. A caller that uploads several
// packs at once commits them in the order it chooses, before the upload's
// admission expires.
func (c *SessionClient) SyncPushUpload(ctx context.Context, resourceID, packID string, blockCount int, packData, bodyHash, bloomFilter []byte, bloomFormatVersion uint32) (bool, error) {
	return c.syncPushUpload(ctx, resourceID, &syncPushPack{
		packID:             packID,
		blockCount:         blockCount,
		bodyHash:           bodyHash,
		bloomFilter:        bloomFilter,
		bloomFormatVersion: bloomFormatVersion,
	}, bytes.NewReader(packData), int64(len(packData)))
}

// SyncPushCommit adds an uploaded pack to the catalog once the cloud has
// checked the stored size and digest. Committing a pack the catalog holds
// succeeds.
func (c *SessionClient) SyncPushCommit(ctx context.Context, resourceID, packID string) error {
	commitPath := path.Join("/api/bstore", resourceID, "sync/push", packID, "commit")
	_, err := c.doPostBinary(ctx, commitPath, nil, nil, SeedReasonMutation)
	return errors.Wrap(err, "sync push commit")
}

// syncPushUpload admits a pack and uploads its bytes, and reports whether the
// pack awaits a commit.
func (c *SessionClient) syncPushUpload(ctx context.Context, resourceID string, pack *syncPushPack, body io.Reader, size int64) (bool, error) {
	// Require the bloom filter the catalog indexes the pack by.
	if len(pack.bloomFilter) == 0 {
		return false, errors.New("sync push bloom filter required")
	}
	if pack.bloomFormatVersion == 0 {
		return false, errors.New("sync push bloom_format_version required")
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
		return false, errors.Wrap(err, "sync push")
	}
	upload := resp.GetUpload()
	if upload == nil {
		return false, nil
	}

	// Upload the bytes to storage.
	if err := c.uploadPack(ctx, upload, body, size); err != nil {
		return false, errors.Wrap(err, "sync push upload")
	}
	return true, nil
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

// RedeemInvite redeems a platform invite code for the session's account.
func (c *SessionClient) RedeemInvite(ctx context.Context, code string) (*s4wave_provider_spacewave.RedeemInviteResponse, error) {
	req := &s4wave_provider_spacewave.RedeemInviteRequest{Code: code}
	var resp s4wave_provider_spacewave.RedeemInviteResponse
	if err := c.postMessage(ctx, "/api/invite/redeem", req, &resp, SeedReasonMutation); err != nil {
		return nil, err
	}
	return &resp, nil
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
	// GetBinding returns the binding the response carries.
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
