package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/util/blockenc"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	"github.com/sirupsen/logrus"
)

// TestTryRecoverMissingSharedObjectPeerSkipsWhenEnrolled checks that a
// verified cache granting the local peer in the current key epoch ends the
// rejoin sweep before any HTTP call.
func TestTryRecoverMissingSharedObjectPeerSkipsWhenEnrolled(t *testing.T) {
	// A local peer holds a grant in the verified current key epoch.
	const (
		soID      = "so-enrolled"
		accountID = "test-account"
	)
	_, pid := generateTestKeypair(t)
	priv, _ := generateTestKeypair(t)

	// Fail the test on any HTTP call through the account's Session client.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call from rejoin gate: %s", r.URL.Path)
	}))
	defer srv.Close()
	acc := NewTestProviderAccount(t, srv.URL)
	acc.sessionClient = NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	// Host the object over a verified cache that already grants the local peer.
	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		acc.sessionClient,
		soID,
		accountID,
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return acc.sessionClient }),
		priv,
		pid,
		acc.sfs,
		&api.VerifiedSOStateCache{
			VerifiedConfigChainHash:  []byte("verified-head"),
			VerifiedConfigChainSeqno: 4,
			KeyEpochs: []*sobject.SOKeyEpoch{{
				Epoch: 2,
				Grants: []*sobject.SOGrant{{
					PeerId: pid.String(),
				}},
			}},
		},
		nil,
		nil,
	)
	host.soHost.SetContext(context.Background())
	so := &SharedObject{
		tkr:      &sobjectTracker{a: acc, id: soID},
		host:     host,
		privKey:  priv,
		localPid: pid,
	}
	ref := sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)

	// The rejoin sweep returns before any request.
	if err := so.tkr.tryRecoverMissingSharedObjectPeer(
		context.Background(),
		ref,
		so,
		acc.sessionClient,
	); err != nil {
		t.Fatalf("tryRecoverMissingSharedObjectPeer: %v", err)
	}
}

// TestTryRecoverMissingSharedObjectPeerRefreshesCachedEpochGrants checks that
// the rejoin sweep replaces a stale local grant with the verified cached one.
func TestTryRecoverMissingSharedObjectPeerRefreshesCachedEpochGrants(t *testing.T) {
	// Derive the current and a stale transform config for the local peer.
	const (
		soID      = "so-enrolled-stale-grants"
		accountID = "test-account"
	)
	priv, pid := generateTestKeypair(t)
	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      []byte("0123456789abcdef0123456789abcdef"),
		},
	})
	if err != nil {
		t.Fatalf("build transform config: %v", err)
	}
	staleTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      []byte("abcdef0123456789abcdef0123456789"),
		},
	})
	if err != nil {
		t.Fatalf("build stale transform config: %v", err)
	}

	// Encrypt a stale grant and a valid grant to the local peer.
	pub, err := pid.ExtractPublicKey()
	if err != nil {
		t.Fatalf("extract public key: %v", err)
	}
	staleGrant, err := sobject.EncryptSOGrant(
		priv,
		pub,
		soID,
		&sobject.SOGrantInner{TransformConf: staleTransformConf},
	)
	if err != nil {
		t.Fatalf("encrypt stale grant: %v", err)
	}
	validGrant, err := sobject.EncryptSOGrant(
		priv,
		pub,
		soID,
		&sobject.SOGrantInner{TransformConf: transformConf},
	)
	if err != nil {
		t.Fatalf("encrypt valid grant: %v", err)
	}

	// Fail the test on any HTTP call through the account's Session client.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call from rejoin gate: %s", r.URL.Path)
	}))
	defer srv.Close()
	acc := NewTestProviderAccount(t, srv.URL)
	acc.sessionClient = NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	// Cache the valid grant in the verified epoch; local state holds the stale one.
	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		acc.sessionClient,
		soID,
		accountID,
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return acc.sessionClient }),
		priv,
		pid,
		acc.sfs,
		&api.VerifiedSOStateCache{
			VerifiedConfigChainHash:  []byte("verified-head"),
			VerifiedConfigChainSeqno: 4,
			KeyEpochs: []*sobject.SOKeyEpoch{{
				Epoch:  2,
				Grants: []*sobject.SOGrant{validGrant},
			}},
		},
		nil,
		nil,
	)
	host.soHost.SetContext(context.Background())
	host.stateCtr.SetValue(&sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{{
				PeerId:   pid.String(),
				Role:     sobject.SOParticipantRole_SOParticipantRole_OWNER,
				EntityId: accountID,
			}},
		},
		KeyEpochs: []*sobject.SOKeyEpoch{{
			Epoch:  1,
			Grants: []*sobject.SOGrant{staleGrant},
		}},
	})
	so := &SharedObject{
		tkr:      &sobjectTracker{a: acc, id: soID},
		host:     host,
		privKey:  priv,
		localPid: pid,
	}
	ref := sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)

	// The rejoin sweep refreshes the local grant from the verified cache.
	if err := so.tkr.tryRecoverMissingSharedObjectPeer(
		context.Background(),
		ref,
		so,
		acc.sessionClient,
	); err != nil {
		t.Fatalf("tryRecoverMissingSharedObjectPeer: %v", err)
	}
	grant := host.stateCtr.GetValue().CurrentKeyEpoch().FindGrant(pid.String())
	if grant == nil {
		t.Fatal("expected a local grant in the current key epoch")
	}
	grantInner, err := grant.DecryptInnerData(priv, soID)
	if err != nil {
		t.Fatalf("decrypt refreshed grant: %v", err)
	}
	if err := grantInner.Validate(); err != nil {
		t.Fatalf("expected refreshed grant with transform config: %v", err)
	}
	if !grantInner.GetTransformConf().EqualVT(transformConf) {
		t.Fatal("expected state grant to refresh from cached current epoch")
	}
}

// TestTryRecoverMissingSharedObjectPeerAllowsReadOnlyLifecycle checks that a
// read-only grace account still attempts self-enrollment.
func TestTryRecoverMissingSharedObjectPeerAllowsReadOnlyLifecycle(t *testing.T) {
	const (
		soID      = "so-readonly"
		accountID = "test-account"
	)

	priv, pid := generateTestKeypair(t)
	var requested bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sobject/"+soID+"/state" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		requested = true
		http.Error(w, "stop after read-only gate", http.StatusTeapot)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.state.info = &api.AccountStateResponse{
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_CANCELED,
		LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_CANCELED_GRACE_READONLY,
	}
	acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY
	acc.sessionClient = NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		acc.sessionClient,
		soID,
		accountID,
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return acc.sessionClient }),
		priv,
		pid,
		acc.sfs,
		nil,
		nil,
		nil,
	)
	host.soHost.SetContext(context.Background())
	so := &SharedObject{
		tkr:      &sobjectTracker{a: acc, id: soID},
		host:     host,
		privKey:  priv,
		localPid: pid,
	}
	ref := sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)

	if err := so.tkr.tryRecoverMissingSharedObjectPeer(
		context.Background(),
		ref,
		so,
		acc.sessionClient,
	); err == nil {
		t.Fatal("expected initial state pull to fail after read-only gate")
	}
	if !requested {
		t.Fatal("expected read-only lifecycle to attempt self-enrollment")
	}
}

// TestTryRecoverMissingSharedObjectPeerSkipsPendingDeleteLifecycle checks that
// an account pending deletion makes no rejoin request.
func TestTryRecoverMissingSharedObjectPeerSkipsPendingDeleteLifecycle(t *testing.T) {
	const (
		soID      = "so-pending-delete"
		accountID = "test-account"
	)

	priv, pid := generateTestKeypair(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected rejoin request during pending delete lifecycle: %s", r.URL.Path)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.state.info = &api.AccountStateResponse{
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_CANCELED,
		LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_PENDING_DELETE_READONLY,
	}
	acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY
	acc.sessionClient = NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		acc.sessionClient,
		soID,
		accountID,
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return acc.sessionClient }),
		priv,
		pid,
		acc.sfs,
		nil,
		nil,
		nil,
	)
	host.soHost.SetContext(context.Background())
	so := &SharedObject{
		tkr:      &sobjectTracker{a: acc, id: soID},
		host:     host,
		privKey:  priv,
		localPid: pid,
	}
	ref := sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)

	if err := so.tkr.tryRecoverMissingSharedObjectPeer(
		context.Background(),
		ref,
		so,
		acc.sessionClient,
	); err != nil {
		t.Fatalf("tryRecoverMissingSharedObjectPeer: %v", err)
	}
}
