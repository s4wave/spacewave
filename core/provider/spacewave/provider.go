package provider_spacewave

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/refcount"
	"github.com/aperturerobotics/util/scrub"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	auth_method_password "github.com/s4wave/spacewave/auth/method/password"
	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Provider implements the spacewave cloud provider.
type Provider struct {
	// le is the logger
	le *logrus.Entry
	// b is the bus
	b bus.Bus
	// conf is the provider configuration
	conf *Config
	// endpoint is the resolved cloud API endpoint URL.
	endpoint string
	// signingEnvPfx is the request-signing environment prefix.
	signingEnvPfx string
	// info is the provider info
	info *provider.ProviderInfo
	// sfs is the step factory set for block transforms
	sfs *block_transform.StepFactorySet
	// peer is the peer instance
	peer peer.Peer
	// handler is the provider handler
	handler provider.ProviderHandler
	// httpCli is the HTTP client for API calls
	httpCli *http.Client
	// cacheSeedBuf records every tagged HTTP call for the cache-seed
	// inspector. Always-on; only the debug RPC exposing it is gated behind
	// a build tag.
	cacheSeedBuf *CacheSeedBuffer
	// accountRc is the keyed refcount for accounts.
	accountRc *keyed.KeyedRefCount[string, *providerAccountTracker]
	// cloudCfgRc lazily fetches and caches pre-auth cloud config.
	cloudCfgRc *refcount.RefCount[*api.AuthConfigResponse]
	// storesMtx guards entityKeyStores and soListCtrs.
	storesMtx sync.Mutex
	// entityKeyStores stores unlocked entity keys keyed by account ID.
	entityKeyStores map[string]*EntityKeyStore
	// soListCtrs caches shared object list containers per account ID.
	// Lives at Provider level so it survives ProviderAccount recreation.
	soListCtrs map[string]*ccontainer.CContainer[*sobject.SharedObjectList]
}

// providerBackoff is the default backoff for provider services.
var providerBackoff = &backoff.Backoff{
	BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
	Exponential: &backoff.Exponential{
		InitialInterval: 500,
		MaxInterval:     1800,
		Multiplier:      1.4,
	},
}

// ProdPublicBaseURL is the production browser-facing app URL.
const ProdPublicBaseURL = "https://spacewave.app"

// getSpacewaveProviderFeatures returns the slice of provider features.
func getSpacewaveProviderFeatures() []provider.ProviderFeature {
	return []provider.ProviderFeature{
		provider.ProviderFeature_ProviderFeature_SESSION,
		provider.ProviderFeature_ProviderFeature_SHARED_OBJECT,
		provider.ProviderFeature_ProviderFeature_SHARED_OBJECT_RECOVERY,
		provider.ProviderFeature_ProviderFeature_BLOCK_STORE,
	}
}

// NewProviderInfo constructs the provider info.
func NewProviderInfo(providerID string) *provider.ProviderInfo {
	return &provider.ProviderInfo{
		ProviderId:       providerID,
		ProviderFeatures: getSpacewaveProviderFeatures(),
	}
}

// NewProvider constructs a new Provider.
func NewProvider(
	le *logrus.Entry,
	b bus.Bus,
	conf *Config,
	info *provider.ProviderInfo,
	peer peer.Peer,
	handler provider.ProviderHandler,
) *Provider {
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_s2.NewStepFactory())
	sfs.AddStepFactory(transform_blockenc.NewStepFactory())

	cacheSeedBuf := NewCacheSeedBuffer(DefaultCacheSeedBufferCapacity)
	p := &Provider{
		le:            le,
		b:             b,
		conf:          conf,
		endpoint:      initEndpoint(conf.GetEndpoint()),
		signingEnvPfx: normalizeSigningEnvPrefix(initSigningEnvPrefix(conf.GetSigningEnvPrefix())),
		info:          info,
		peer:          peer,
		handler:       handler,
		sfs:           sfs,
		httpCli: &http.Client{
			Transport: NewCacheSeedRecordingTransport(nil, cacheSeedBuf),
		},
		cacheSeedBuf: cacheSeedBuf,
	}
	p.accountRc = keyed.NewKeyedRefCountWithLogger(
		p.buildProviderAccountTracker,
		le,
		keyed.WithRetry[string, *providerAccountTracker](providerBackoff),
	)
	p.cloudCfgRc = refcount.NewRefCount(nil, true, nil, nil, p.resolveCloudConfig)
	return p
}

// GetEntityKeyStore returns the per-account entity key store.
func (p *Provider) GetEntityKeyStore(accountID string) *EntityKeyStore {
	p.storesMtx.Lock()
	if p.entityKeyStores == nil {
		p.entityKeyStores = make(map[string]*EntityKeyStore)
	}
	store, ok := p.entityKeyStores[accountID]
	if !ok {
		store = NewEntityKeyStore()
		p.entityKeyStores[accountID] = store
	}
	p.storesMtx.Unlock()
	return store
}

// RetainEntityKeyBootstrap unlocks an entity key for bootstrap work.
func (p *Provider) RetainEntityKeyBootstrap(accountID string, priv crypto.PrivKey, pid peer.ID) *EntityKeyStoreRef {
	store := p.GetEntityKeyStore(accountID)
	store.Unlock(pid, priv)
	ref := store.Retain()
	if p.accountRc == nil {
		return ref
	}
	if tracker, ok := p.accountRc.GetKey(accountID); ok {
		if acc := tracker.accCtr.GetValue(); acc != nil {
			acc.ReplaceEntityClient(NewEntityClientDirect(
				p.httpCli,
				p.endpoint,
				p.signingEnvPfx,
				priv,
				pid,
			))
			acc.bumpSelfRejoinSweepGeneration()
		}
	}
	return ref
}

// getSOListCtr returns the persistent shared object list container for an account.
// The container lives at the Provider level to survive ProviderAccount recreation.
func (p *Provider) getSOListCtr(accountID string) *ccontainer.CContainer[*sobject.SharedObjectList] {
	p.storesMtx.Lock()
	if p.soListCtrs == nil {
		p.soListCtrs = make(map[string]*ccontainer.CContainer[*sobject.SharedObjectList])
	}
	ctr, ok := p.soListCtrs[accountID]
	if !ok {
		ctr = ccontainer.NewCContainer[*sobject.SharedObjectList](nil)
		p.soListCtrs[accountID] = ctr
	}
	p.storesMtx.Unlock()
	return ctr
}

// GetProviderInfo returns the basic provider information.
func (p *Provider) GetProviderInfo() *provider.ProviderInfo {
	return p.info.CloneVT()
}

// GetHTTPClient returns the HTTP client for API calls.
func (p *Provider) GetHTTPClient() *http.Client {
	return p.httpCli
}

// GetCacheSeedBuffer returns the provider's cache-seed request buffer.
// Used by the dev-mode cache-seed inspector RPC.
func (p *Provider) GetCacheSeedBuffer() *CacheSeedBuffer {
	return p.cacheSeedBuf
}

// GetEndpoint returns the cloud API endpoint URL.
func (p *Provider) GetEndpoint() string {
	return p.endpoint
}

// GetAccountEndpoint returns the browser-facing account endpoint URL.
func (p *Provider) GetAccountEndpoint() string {
	return initAccountBaseURL(p.conf.GetAccountEndpoint())
}

// GetSigningEnvPrefix returns the cloud request-signing environment prefix.
func (p *Provider) GetSigningEnvPrefix() string {
	return p.signingEnvPfx
}

// GetPublicBaseURL returns the browser-facing public app base URL.
func (p *Provider) GetPublicBaseURL() string {
	if pu := initPublicBaseURL(p.conf.GetPublicBaseUrl()); pu != "" {
		return pu
	}
	return ProdPublicBaseURL
}

// AccessProviderAccount accesses a provider account.
func (p *Provider) AccessProviderAccount(ctx context.Context, accountID string, released func()) (provider.ProviderAccount, func(), error) {
	ref, providerAccTkr, _ := p.accountRc.AddKeyRef(accountID)
	providerAcc, err := providerAccTkr.accCtr.WaitValue(ctx, nil)
	if err != nil {
		ref.Release()
		return nil, nil, err
	}
	return providerAcc, ref.Release, nil
}

// CreateSpacewaveAccountAndSession derives an entity keypair from username and password,
// registers the account with the cloud, and mounts a session.
//
// TODO: use DeriveEntityKeypair directive via the bus instead of calling
// BuildParametersWithUsernamePassword directly (requires auth controllers on the bus).
func (p *Provider) CreateSpacewaveAccountAndSession(
	ctx context.Context,
	username string,
	password []byte,
	turnstileToken string,
	sessionCtrl session.SessionController,
) (*session.SessionListEntry, error) {
	params, privKey, err := auth_method_password.BuildParametersWithUsernamePassword(username, password)
	if err != nil {
		return nil, errors.Wrap(err, "derive entity keypair")
	}

	authParams, err := params.MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshal auth params")
	}

	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, errors.Wrap(err, "derive peer id")
	}

	// Build entity client with derived key.
	entityCli := NewEntityClientDirect(
		p.httpCli,
		p.endpoint,
		p.signingEnvPfx,
		privKey,
		peerID,
	)

	// Register account with cloud.
	accountID, err := entityCli.RegisterAccount(ctx, username, auth_method_password.MethodID, authParams, turnstileToken)
	if err != nil {
		return nil, errors.Wrap(err, "register account")
	}

	p.le.WithField("account-id", accountID).Debug("registered spacewave account")

	// Store entity private key so the account tracker can use it.
	bootstrapRef := p.RetainEntityKeyBootstrap(accountID, privKey, peerID)
	defer bootstrapRef.Release()

	// Check for an existing session with this account before creating a new one.
	sessions, listErr := sessionCtrl.ListSessions(ctx)
	if listErr == nil {
		for _, entry := range sessions {
			ref := entry.GetSessionRef().GetProviderResourceRef()
			if ref.GetProviderAccountId() == accountID {
				return entry, nil
			}
		}
	}

	// Start the account tracker.
	provAcc, relProvAcc, err := p.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		return nil, errors.Wrap(err, "access provider account")
	}
	defer relProvAcc()

	// Get the session provider feature.
	sessProv, err := session.GetSessionProviderAccountFeature(ctx, provAcc)
	if err != nil {
		return nil, errors.Wrap(err, "get session provider")
	}

	// Build session ref and mount the session.
	sessRef := &session.SessionRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                ulid.NewULID(),
			ProviderAccountId: accountID,
			ProviderId:        p.info.GetProviderId(),
		},
	}
	sess, relSess, err := sessProv.MountSession(ctx, sessRef, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount session")
	}
	defer relSess()
	_ = sess

	// Register session with initial metadata.
	meta := &session.SessionMetadata{
		DisplayName:         username,
		ProviderDisplayName: "Cloud",
		ProviderAccountId:   accountID,
		ProviderId:          "spacewave",
		CreatedAt:           time.Now().UnixMilli(),
	}
	listEntry, err := sessionCtrl.RegisterSession(ctx, sessRef, meta)
	if err != nil {
		return nil, errors.Wrap(err, "register session")
	}

	return listEntry, nil
}

// LoginOrCreateAccount derives an entity keypair from username and password,
// attempts to register a new account, and falls back to session login if the
// account already exists.
//
// Returns the session list entry, whether this is a new account, and any error.
// If the password is wrong for an existing account, the error will contain
// "unknown_keypair" from the cloud.
func (p *Provider) LoginOrCreateAccount(
	ctx context.Context,
	username string,
	password []byte,
	sessionCtrl session.SessionController,
) (*session.SessionListEntry, bool, error) {
	params, privKey, err := auth_method_password.BuildParametersWithUsernamePassword(username, password)
	if err != nil {
		return nil, false, errors.Wrap(err, "derive entity keypair")
	}

	authParams, err := params.MarshalBlock()
	if err != nil {
		return nil, false, errors.Wrap(err, "marshal auth params")
	}

	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, false, errors.Wrap(err, "derive peer id")
	}

	entityCli := NewEntityClientDirect(
		p.httpCli,
		p.endpoint,
		p.signingEnvPfx,
		privKey,
		peerID,
	)

	// Try to register a new account.
	accountID, err := entityCli.RegisterAccount(ctx, username, auth_method_password.MethodID, authParams, "")
	if err != nil {
		// Check if the account already exists.
		var ce *cloudError
		if !errors.As(err, &ce) || ce.Code != "username_taken" {
			return nil, false, errors.Wrap(err, "register account")
		}

		// Account exists: try to register a session with the derived entity key.
		// This verifies the password is correct for this username.
		sessEntry, sessErr := p.LoginExistingAccount(ctx, entityCli, privKey, peerID, username, "", sessionCtrl)
		if sessErr != nil {
			return nil, false, sessErr
		}
		return sessEntry, false, nil
	}

	p.le.WithField("account-id", accountID).Debug("registered spacewave account")

	// New account: mount session.
	sessEntry, err := p.mountNewSession(ctx, accountID, username, privKey, peerID, sessionCtrl)
	if err != nil {
		return nil, false, err
	}
	return sessEntry, true, nil
}

// LoginExistingAccount verifies the entity key is valid for the account and
// mounts a new session. Returns ErrUnknownEntity if the account exists but
// the credentials are wrong, or ErrUnknownKeypair if no account exists.
func (p *Provider) LoginExistingAccount(
	ctx context.Context,
	entityCli *EntityClient,
	privKey crypto.PrivKey,
	peerID peer.ID,
	entityID string,
	turnstileToken string,
	sessionCtrl session.SessionController,
) (*session.SessionListEntry, error) {
	// Register a probe session to verify the entity key and get the accountID.
	// The session tracker will register its own real session separately.
	resp, err := entityCli.RegisterSessionDirectWithResponse(ctx, peerID.String(), "probe", entityID, turnstileToken)
	if err != nil {
		return nil, errors.Wrap(err, "login")
	}

	accountID := resp.GetAccountId()
	if accountID == "" {
		return nil, errors.New("server response missing account_id")
	}

	// Use entity ID from the cloud response if the caller didn't provide one
	// (e.g. PEM login, passkey, SSO flows).
	if entityID == "" {
		entityID = resp.GetEntityId()
	}

	// Check for an existing session with this account before creating a new one.
	sessions, err := sessionCtrl.ListSessions(ctx)
	if err == nil {
		for _, entry := range sessions {
			ref := entry.GetSessionRef().GetProviderResourceRef()
			if ref.GetProviderAccountId() == accountID {
				// Reuse existing session and cache the entity key so
				// the provider account tracker can use it.
				bootstrapRef := p.RetainEntityKeyBootstrap(accountID, privKey, peerID)
				defer bootstrapRef.Release()
				// Update display name if it was missing.
				if entityID != "" {
					meta, _ := sessionCtrl.GetSessionMetadata(ctx, entry.GetSessionIndex())
					if meta != nil && meta.DisplayName == "" {
						meta.DisplayName = entityID
						_ = sessionCtrl.UpdateSessionMetadata(ctx, entry.GetSessionRef(), meta)
					}
				}
				return entry, nil
			}
		}
	}

	return p.mountNewSession(ctx, accountID, entityID, privKey, peerID, sessionCtrl)
}

// mountNewSession stores the entity key, starts the account tracker, and mounts
// a new session for a freshly-created account.
func (p *Provider) mountNewSession(
	ctx context.Context,
	accountID string,
	entityID string,
	privKey crypto.PrivKey,
	peerID peer.ID,
	sessionCtrl session.SessionController,
) (*session.SessionListEntry, error) {
	bootstrapRef := p.RetainEntityKeyBootstrap(accountID, privKey, peerID)
	defer bootstrapRef.Release()

	provAcc, relProvAcc, err := p.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		return nil, errors.Wrap(err, "access provider account")
	}
	defer relProvAcc()

	sessProv, err := session.GetSessionProviderAccountFeature(ctx, provAcc)
	if err != nil {
		return nil, errors.Wrap(err, "get session provider")
	}

	sessRef := &session.SessionRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                ulid.NewULID(),
			ProviderAccountId: accountID,
			ProviderId:        p.info.GetProviderId(),
		},
	}
	sess, relSess, err := sessProv.MountSession(ctx, sessRef, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount session")
	}
	defer relSess()
	_ = sess

	meta := &session.SessionMetadata{
		DisplayName:         entityID,
		ProviderDisplayName: "Cloud",
		ProviderAccountId:   accountID,
		ProviderId:          "spacewave",
		CreatedAt:           time.Now().UnixMilli(),
	}
	listEntry, err := sessionCtrl.RegisterSession(ctx, sessRef, meta)
	if err != nil {
		return nil, errors.Wrap(err, "register session")
	}

	return listEntry, nil
}

// Execute executes the provider.
func (p *Provider) Execute(ctx context.Context) error {
	p.accountRc.SetContext(ctx, true)
	_ = p.cloudCfgRc.SetContext(ctx)
	return nil
}

func scrubPrivKey(priv crypto.PrivKey) {
	if priv == nil {
		return
	}

	type ed25519StdKey interface {
		GetStdKey() ed25519.PrivateKey
	}
	if k, ok := priv.(ed25519StdKey); ok {
		scrub.Scrub(k.GetStdKey())
	}

	// NOTE: other key types are not yet supported
}

// _ is a type assertion
var _ provider.Provider = (*Provider)(nil)
