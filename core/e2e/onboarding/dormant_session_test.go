//go:build e2e

package onboarding_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	auth_method_password "github.com/s4wave/spacewave/auth/method/password"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
)

// onboardingStatusWatchStream captures WatchOnboardingStatus responses.
type onboardingStatusWatchStream struct {
	srpc.Stream
	ctx  context.Context
	msgs chan *s4wave_provider_spacewave.WatchOnboardingStatusResponse
}

// newOnboardingStatusWatchStream builds a new watch stream recorder.
func newOnboardingStatusWatchStream(
	ctx context.Context,
) *onboardingStatusWatchStream {
	return &onboardingStatusWatchStream{
		ctx:  ctx,
		msgs: make(chan *s4wave_provider_spacewave.WatchOnboardingStatusResponse, 16),
	}
}

// Context returns the stream context.
func (m *onboardingStatusWatchStream) Context() context.Context {
	return m.ctx
}

// Send records a streamed response.
func (m *onboardingStatusWatchStream) Send(
	resp *s4wave_provider_spacewave.WatchOnboardingStatusResponse,
) error {
	select {
	case m.msgs <- resp:
		return nil
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}

// SendAndClose records a final streamed response.
func (m *onboardingStatusWatchStream) SendAndClose(
	resp *s4wave_provider_spacewave.WatchOnboardingStatusResponse,
) error {
	return m.Send(resp)
}

// MsgRecv is unused for this mock stream.
func (m *onboardingStatusWatchStream) MsgRecv(msg srpc.Message) error {
	return nil
}

// MsgSend is unused for this mock stream.
func (m *onboardingStatusWatchStream) MsgSend(msg srpc.Message) error {
	return nil
}

// CloseSend is unused for this mock stream.
func (m *onboardingStatusWatchStream) CloseSend() error {
	return nil
}

// Close is unused for this mock stream.
func (m *onboardingStatusWatchStream) Close() error {
	return nil
}

// setPlatformRole makes roleID the account's only platform lifecycle role.
func setPlatformRole(t *testing.T, accountID, roleID string) {
	// Replace the platform role through the coordinator test helper.
	t.Helper()
	var a fastjson.Arena
	body := a.NewObject()
	body.Set("account_id", a.NewString(accountID))
	body.Set("role_id", a.NewString(roleID))
	body.Set("scope", a.NewString("platform"))
	postTestHelper(context.Background(), t, "/api/test/set-rbac", body)
}

// setTestSubscriptionStatus sets the account's subscription status, which
// selects its platform role.
func setTestSubscriptionStatus(t *testing.T, accountID, status string) {
	// Set the status through the coordinator test helper.
	t.Helper()
	var a fastjson.Arena
	body := a.NewObject()
	body.Set("account_id", a.NewString(accountID))
	body.Set("subscription_status", a.NewString(status))
	postTestHelper(context.Background(), t, "/api/test/set-subscription", body)
}

// loginDormantCloudSession registers an account whose platform role lacks
// Session.create, logs in to it, and links a local session to the dormant
// cloud session.
func loginDormantCloudSession(
	ctx context.Context,
	t *testing.T,
) (*session.SessionListEntry, *provider_spacewave.ProviderAccount) {
	// Look up the session controller and the spacewave provider.
	t.Helper()
	sessCtrl, relSess, err := lookupSessionController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer relSess()
	prov, provRef, err := provider.ExLookupProvider(
		ctx,
		env.tb.Bus,
		"spacewave",
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()
	swProv := prov.(*provider_spacewave.Provider)

	// Derive the password auth parameters and entity key for a new username.
	username := "dormant-" + ulid.NewULID()
	password := []byte("test-password-" + ulid.NewULID())
	params, privKey, err := auth_method_password.BuildParametersWithUsernamePassword(
		username,
		password,
	)
	if err != nil {
		t.Fatal(err)
	}
	authParams, err := params.MarshalBlock()
	if err != nil {
		t.Fatal(err)
	}
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		t.Fatal(err)
	}

	// Register the account with the coordinator.
	entityCli := provider_spacewave.NewEntityClientDirect(
		httpClient,
		env.cloudURL,
		provider_spacewave.DefaultSigningEnvPrefix,
		privKey,
		peerID,
	)
	accountID, err := entityCli.RegisterAccount(
		ctx,
		username,
		auth_method_password.MethodID,
		authParams,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}

	// Replace the default platform role with one that lacks Session.create so
	// the real session ticket path returns rbac_denied and the tracker enters
	// DORMANT.
	setPlatformRole(t, accountID, "disputed_locked")

	// Log in to the account, which adds its cloud session.
	entry, err := swProv.LoginExistingAccount(
		ctx,
		entityCli,
		privKey,
		peerID,
		username,
		"",
		sessCtrl,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Link a new local session to the cloud session.
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relAcc)
	swAcc := accIface.(*provider_spacewave.ProviderAccount)
	localEntry, _ := createLocalSession(ctx, t, accountID)
	cloudSessionID := entry.GetSessionRef().GetProviderResourceRef().GetId()
	if err := swAcc.SetLinkedLocalSession(
		ctx,
		cloudSessionID,
		localEntry.GetSessionIndex(),
	); err != nil {
		t.Fatal(err)
	}

	// The account reads the link back.
	found, linkedIdx, err := swAcc.GetLinkedLocalSession(ctx, cloudSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || linkedIdx != localEntry.GetSessionIndex() {
		t.Fatalf(
			"expected linked local session %d, got found=%t idx=%d",
			localEntry.GetSessionIndex(),
			found,
			linkedIdx,
		)
	}

	return entry, swAcc
}

// waitForOnboardingStatus waits for an onboarding status with the account
// status and subscription flag.
func waitForOnboardingStatus(
	t *testing.T,
	msgs <-chan *s4wave_provider_spacewave.WatchOnboardingStatusResponse,
	status provider.ProviderAccountStatus,
	hasSubscription bool,
) *s4wave_provider_spacewave.WatchOnboardingStatusResponse {
	// Read statuses until one matches or the timeout passes.
	t.Helper()
	timeout := time.NewTimer(20 * time.Second)
	defer timeout.Stop()
	var last *s4wave_provider_spacewave.WatchOnboardingStatusResponse
	for {
		select {
		case resp := <-msgs:
			last = resp
			if resp.GetAccountStatus() == status && resp.GetHasSubscription() == hasSubscription {
				return resp
			}
		case <-timeout.C:
			if last == nil {
				t.Fatalf("timed out waiting for %s onboarding status", status)
			}
			t.Fatalf(
				"timed out waiting for %s with hasSubscription=%t: got %s with hasSubscription=%t",
				status,
				hasSubscription,
				last.GetAccountStatus(),
				last.GetHasSubscription(),
			)
		}
	}
}

// TestDormantCloudSessionInactiveState verifies a cloud session emits DORMANT
// through WatchOnboardingStatus once the real session ticket flow starts
// returning rbac_denied. This is the stream the routing layer uses to decide
// whether a linked local session exists while SessionContainer gates on the
// same DORMANT account status for the overlay.
func TestDormantCloudSessionInactiveState(t *testing.T) {
	// Log in to a dormant cloud session.
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()
	cloudEntry, swAcc := loginDormantCloudSession(ctx, t)

	// The account state reports no subscription.
	snapshot := swAcc.AccountStateSnapshot()
	if snapshot == nil {
		var err error
		snapshot, err = swAcc.GetAccountState(ctx)
		if err != nil {
			t.Fatal(err)
		}
	}
	if snapshot == nil {
		t.Fatal("expected account state snapshot after dormant login")
	}
	if snapshot.GetSubscriptionStatus().NormalizedString() != "none" {
		t.Fatalf(
			"expected subscription status none, got %q",
			snapshot.GetSubscriptionStatus().NormalizedString(),
		)
	}

	// Mount the cloud session and build its Spacewave resource.
	sess, sessRef, err := session.ExMountSession(
		ctx,
		env.tb.Bus,
		cloudEntry.GetSessionRef(),
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sessRef.Release()
	le := logrus.NewEntry(logrus.StandardLogger())
	parent := resource_session.NewSessionResource(
		le,
		env.tb.Bus,
		sess,
	)
	resource := resource_session.NewSpacewaveSessionResource(
		parent,
		le,
		env.tb.Bus,
		sess,
		swAcc,
	)

	// Watch the onboarding status.
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	strm := newOnboardingStatusWatchStream(watchCtx)
	watchErr := make(chan error, 1)
	go func() {
		watchErr <- resource.WatchOnboardingStatus(
			&s4wave_provider_spacewave.WatchOnboardingStatusRequest{},
			strm,
		)
	}()

	// The session reports DORMANT with its linked local session.
	resp := waitForOnboardingStatus(t, strm.msgs, provider.ProviderAccountStatus_ProviderAccountStatus_DORMANT, false)
	if !resp.GetHasLinkedLocal() {
		t.Fatal("expected dormant account to report linked local session")
	}
	if resp.GetLinkedLocalSessionIndex() == 0 {
		t.Fatal("expected dormant account to report linked local session index")
	}

	// Stop the watch.
	watchCancel()
	if err := <-watchErr; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

// TestDormantSessionWakesOnResubscribe verifies the dormant tracker wakes once
// subscription access is restored and the local account broadcast is nudged via
// the existing epoch invalidation path.
func TestDormantSessionWakesOnResubscribe(t *testing.T) {
	// Log in to a dormant cloud session.
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()
	cloudEntry, swAcc := loginDormantCloudSession(ctx, t)

	// Mount the cloud session and build its Spacewave resource.
	sess, sessRef, err := session.ExMountSession(
		ctx,
		env.tb.Bus,
		cloudEntry.GetSessionRef(),
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sessRef.Release()
	le := logrus.NewEntry(logrus.StandardLogger())
	parent := resource_session.NewSessionResource(
		le,
		env.tb.Bus,
		sess,
	)
	resource := resource_session.NewSpacewaveSessionResource(
		parent,
		le,
		env.tb.Bus,
		sess,
		swAcc,
	)

	// Watch the onboarding status.
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	strm := newOnboardingStatusWatchStream(watchCtx)
	watchErr := make(chan error, 1)
	go func() {
		watchErr <- resource.WatchOnboardingStatus(
			&s4wave_provider_spacewave.WatchOnboardingStatusRequest{},
			strm,
		)
	}()

	// The session starts DORMANT.
	waitForOnboardingStatus(t, strm.msgs, provider.ProviderAccountStatus_ProviderAccountStatus_DORMANT, false)

	// Activate the subscription and nudge the account to refetch its state.
	setTestSubscriptionStatus(t, swAcc.GetAccountID(), "active")
	swAcc.BumpLocalEpoch()

	// The session wakes to READY with its linked local session.
	readyResp := waitForOnboardingStatus(t, strm.msgs, provider.ProviderAccountStatus_ProviderAccountStatus_READY, true)
	if readyResp.GetLinkedLocalSessionIndex() == 0 {
		t.Fatal("expected linked local session metadata after reactivation")
	}

	// Stop the watch.
	watchCancel()
	if err := <-watchErr; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

// _ is a type assertion
var _ s4wave_session.SRPCSpacewaveSessionResourceService_WatchOnboardingStatusStream = (*onboardingStatusWatchStream)(nil)
