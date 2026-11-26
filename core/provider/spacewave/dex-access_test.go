package provider_spacewave

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestCloudDEXRequestAuthorization reads current roles through a mounted cloud SharedObject.
func TestCloudDEXRequestAuthorization(t *testing.T) {
	// Keep the request alive independently from the cloud SharedObject source.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	account, host, _, requester, absent := newCloudDEXAccessTestAccount(t)

	// A current readable role authorizes the production DEX request interface.
	access, err := account.authorizeDEXRequest(ctx, &dex_solicit.Config{
		ProtocolContext: []byte(host.soID),
	}, requester)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(access.Release)
	if err := access.Check(); err != nil {
		t.Fatalf("current reader role was denied: %v", err)
	}

	// A peer absent from the current config cannot read through the same gate.
	refused, err := account.authorizeDEXRequest(ctx, &dex_solicit.Config{
		ProtocolContext: []byte(host.soID),
	}, absent)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(refused.Release)
	if err := refused.Check(); err == nil {
		t.Fatal("requester without a current role was authorized")
	}

	// Revoking the current reader updates the held authority source immediately.
	revoked := make(chan error, 1)
	go func() { revoked <- access.WaitRevoked(ctx) }()
	state := host.stateCtr.GetValue().CloneVT()
	state.Config.Participants = state.Config.Participants[:1]
	host.stateCtr.SetValue(state)
	select {
	case err := <-revoked:
		if err == nil {
			t.Fatal("revoked requester kept its held DEX access")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("held DEX access did not observe role revocation")
	}
	if err := access.Check(); err == nil {
		t.Fatal("role revocation did not stop a further read")
	}
}

// TestCloudDEXRequestAccessStopsWithSource rejects cached roles when the
// mounted cloud SharedObject source ends before the request does.
func TestCloudDEXRequestAccessStopsWithSource(t *testing.T) {
	// Keep the request alive independently from its mounted source.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	account, host, cancelSource, requester, _ := newCloudDEXAccessTestAccount(t)

	// Acquire access while the cloud SharedObject reports a readable role.
	access, err := account.authorizeDEXRequest(ctx, &dex_solicit.Config{
		ProtocolContext: []byte(host.soID),
	}, requester)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(access.Release)
	if err := access.Check(); err != nil {
		t.Fatal(err)
	}

	// Source cancellation must revoke the cached role before the request ends.
	revoked := make(chan error, 1)
	go func() { revoked <- access.WaitRevoked(ctx) }()
	cancelSource()
	select {
	case err := <-revoked:
		if err == nil {
			t.Fatal("cloud source ended without rejecting the held request")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("held DEX access outlived its cloud SharedObject source")
	}
	if err := access.Check(); err == nil {
		t.Fatal("cached cloud role still authorized after its source ended")
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("source cancellation ended the independent DEX request: %v", err)
	}
}

// newCloudDEXAccessTestAccount mounts the package's real cloud SharedObject host without network access.
func newCloudDEXAccessTestAccount(
	t *testing.T,
) (*ProviderAccount, *cloudSOHost, context.CancelFunc, peer.ID, peer.ID) {
	// Create the test account and requester identities.
	t.Helper()
	const soID = "so-cloud-dex-access"
	account := NewTestProviderAccount(t, "http://example.invalid")
	ownerPriv, owner := generateTestKeypair(t)
	_, requester := generateTestKeypair(t)
	_, absent := generateTestKeypair(t)
	account.sessionClient = NewSessionClient(http.DefaultClient, "http://example.invalid", DefaultSigningEnvPrefix, ownerPriv, owner.String())

	// Construct a cloud host whose lifetime follows the mounted source.
	sourceCtx, cancelSource := context.WithCancel(t.Context())
	t.Cleanup(cancelSource)
	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		account.sessionClient,
		soID,
		account.accountID,
		newTestWSTracker(account.le, func() *SessionClient { return account.sessionClient }),
		ownerPriv,
		owner,
		account.sfs,
		nil,
		cloudSOHostHooks{},
		nil,
	)
	host.soHost.SetContext(sourceCtx)

	// Publish the current owner and requester roles through the real state watch.
	host.stateCtr.SetValue(&sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{{
				PeerId: owner.String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_OWNER,
			}, {
				PeerId: requester.String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_READER,
			}},
		},
	})

	// Seed the normal tracker with the actual cloud host for MountSharedObject.
	trackerRef, tracker, _ := account.sobjects.AddKeyRef(soID)
	t.Cleanup(trackerRef.Release)
	tracker.ref.SetResult(account.buildSharedObjectRef(soID), nil)
	tracker.sobjectProm.SetResult(&SharedObject{
		tkr:      tracker,
		ctx:      sourceCtx,
		host:     host,
		privKey:  ownerPriv,
		localPid: owner,
	}, nil)
	return account, host, cancelSource, requester, absent
}
