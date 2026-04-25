//go:build e2e

package onboarding_test

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

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

func forcePlatformRole(t *testing.T, accountID, roleID string) {
	t.Helper()
	if env.cloudDir == "" || env.tempDir == "" {
		t.Fatal("wrangler environment missing cloudDir or tempDir")
	}

	sql := "UPDATE rbac_role_bindings SET role_id = '" + roleID + "'" +
		" WHERE subject_id = '" + accountID + "' AND scope = 'platform';"
	cmd := exec.Command(
		"npx",
		"wrangler",
		"d1",
		"execute",
		"spacewave",
		"--local",
		"--persist-to",
		env.tempDir,
		"--command",
		sql,
	)
	cmd.Dir = env.cloudDir
	cmd.Env = append(os.Environ(), "NODE_ENV=test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("force platform role: %v: %s", err, string(out))
	}
}

func setTestSubscriptionStatus(t *testing.T, accountID, status string) {
	t.Helper()

	body := `{"account_id":"` + accountID + `","subscription_status":"` + status + `"}`
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		env.cloudURL+"/api/test/set-subscription",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set subscription status returned %d", resp.StatusCode)
	}
}

func loginDormantCloudSession(
	ctx context.Context,
	t *testing.T,
) (*session.SessionListEntry, *provider_spacewave.ProviderAccount) {
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
	forcePlatformRole(t, accountID, "owner")

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

func waitForDormantOnboardingStatus(
	t *testing.T,
	msgs <-chan *s4wave_provider_spacewave.WatchOnboardingStatusResponse,
) *s4wave_provider_spacewave.WatchOnboardingStatusResponse {
	t.Helper()

	timeout := time.NewTimer(20 * time.Second)
	defer timeout.Stop()

	var last *s4wave_provider_spacewave.WatchOnboardingStatusResponse
	for {
		select {
		case resp := <-msgs:
			last = resp
			if resp.GetAccountStatus() ==
				provider.ProviderAccountStatus_ProviderAccountStatus_DORMANT {
				return resp
			}
		case <-timeout.C:
			if last == nil {
				t.Fatal("timed out waiting for WatchOnboardingStatus response")
			}
			t.Fatalf(
				"timed out waiting for DORMANT status: hasSubscription=%t accountStatus=%s",
				last.GetHasSubscription(),
				strconv.Itoa(int(last.GetAccountStatus())),
			)
		}
	}
}

func waitForReadyOnboardingStatus(
	t *testing.T,
	msgs <-chan *s4wave_provider_spacewave.WatchOnboardingStatusResponse,
) *s4wave_provider_spacewave.WatchOnboardingStatusResponse {
	t.Helper()

	timeout := time.NewTimer(20 * time.Second)
	defer timeout.Stop()

	var last *s4wave_provider_spacewave.WatchOnboardingStatusResponse
	for {
		select {
		case resp := <-msgs:
			last = resp
			if resp.GetAccountStatus() ==
				provider.ProviderAccountStatus_ProviderAccountStatus_READY &&
				resp.GetHasSubscription() {
				return resp
			}
		case <-timeout.C:
			if last == nil {
				t.Fatal("timed out waiting for READY onboarding status")
			}
			t.Fatalf(
				"timed out waiting for READY status: hasSubscription=%t accountStatus=%s",
				last.GetHasSubscription(),
				strconv.Itoa(int(last.GetAccountStatus())),
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
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	cloudEntry, swAcc := loginDormantCloudSession(ctx, t)

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

	resp := waitForDormantOnboardingStatus(t, strm.msgs)
	if resp.GetHasSubscription() {
		t.Fatal("expected dormant account to report hasSubscription=false")
	}
	if !resp.GetHasLinkedLocal() {
		t.Fatal("expected dormant account to report linked local session")
	}
	if resp.GetLinkedLocalSessionIndex() == 0 {
		t.Fatal("expected dormant account to report linked local session index")
	}

	watchCancel()
	if err := <-watchErr; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

// TestDormantSessionWakesOnResubscribe verifies the dormant tracker wakes once
// subscription access is restored and the local account broadcast is nudged via
// the existing epoch invalidation path.
func TestDormantSessionWakesOnResubscribe(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	cloudEntry, swAcc := loginDormantCloudSession(ctx, t)

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

	dormantResp := waitForDormantOnboardingStatus(t, strm.msgs)
	if dormantResp.GetHasSubscription() {
		t.Fatal("expected dormant setup to start without a subscription")
	}

	setTestSubscriptionStatus(t, swAcc.GetAccountID(), "active")
	swAcc.BumpLocalEpoch()

	readyResp := waitForReadyOnboardingStatus(t, strm.msgs)
	if readyResp.GetLinkedLocalSessionIndex() == 0 {
		t.Fatal("expected linked local session metadata after reactivation")
	}

	watchCancel()
	if err := <-watchErr; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

// _ is a type assertion
var _ s4wave_session.SRPCSpacewaveSessionResourceService_WatchOnboardingStatusStream = (*onboardingStatusWatchStream)(nil)
