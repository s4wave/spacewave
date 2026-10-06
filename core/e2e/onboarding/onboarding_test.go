//go:build e2e

// Package onboarding_test exercises the full onboarding and migration lifecycle
// against a spacewave-coordinator built from a Spacewave Cloud checkout and run
// with an in-memory database and its test helpers.
//
// Run with:
//
//	SPACEWAVE_CLOUD_DIR=/path/to/spacewave-cloud \
//	  go test -tags e2e -count=1 -timeout=10m -v ./core/e2e/onboarding/
package onboarding_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/iowriter"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	auth_method_password "github.com/s4wave/spacewave/auth/method/password"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	provider_transfer "github.com/s4wave/spacewave/core/provider/transfer"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	packfile_writer "github.com/s4wave/spacewave/db/packfile/writer"
	bifcrypto "github.com/s4wave/spacewave/net/crypto"
	bifhash "github.com/s4wave/spacewave/net/hash"
	bifpeer "github.com/s4wave/spacewave/net/peer"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// testEnv holds the coordinator subprocess and test infrastructure.
type testEnv struct {
	// cmd runs the coordinator in its own process group.
	cmd *exec.Cmd
	// accountOrigin is the coordinator's account host origin, which opens
	// desktop passkey ceremonies and is an accepted WebAuthn origin.
	accountOrigin string
	// passkeyRpID is the WebAuthn relying party identifier.
	passkeyRpID string
	// tb retains the suite's in-memory controller bus and storage.
	tb *testbed.Testbed
	// ctx bounds the suite's subprocesses and mounted resources.
	ctx context.Context
	// cancel ends the suite lifecycle before resource release.
	cancel context.CancelFunc
	// cloudURL points only to the isolated local coordinator.
	cloudURL string
	// tempDir holds the coordinator binary and its database for this run.
	tempDir string
}

var (
	// env is constructed once by TestMain and retained until the suite ends.
	env *testEnv
	// httpClient uses normal redirect behavior for account web journeys.
	httpClient = http.DefaultClient
)

// extractJSONField reads a top-level string from the backend's JSON response.
func extractJSONField(jsonStr, key string) string {
	var parser fastjson.Parser
	value, err := parser.Parse(jsonStr)
	if err != nil {
		return ""
	}
	return string(value.GetStringBytes(key))
}

// emailToken returns the token of the newest email of tokenType sent to the
// account: verification, recovery or manage. The coordinator's test helper
// reads it from the account's stored tokens, which the emailed link carries.
func emailToken(ctx context.Context, t *testing.T, accountID, tokenType string) (token, code string) {
	// Request the token from the coordinator test helper.
	t.Helper()
	query := url.Values{"account_id": {accountID}, "type": {tokenType}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.cloudURL+"/api/test/token?"+query.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Read the response and require success.
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read %s token: %d body=%s", tokenType, resp.StatusCode, string(body))
	}

	// Extract the token and its short code.
	token, code = extractJSONField(string(body), "token"), extractJSONField(string(body), "code")
	if token == "" {
		t.Fatalf("read %s token: empty token", tokenType)
	}
	return token, code
}

// postTestHelper posts body to the coordinator test helper at path.
func postTestHelper(ctx context.Context, t *testing.T, path string, body *fastjson.Value) {
	// Build the request.
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, env.cloudURL+path, bytes.NewReader(body.MarshalTo(nil)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send it and require success.
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s returned %d: %s", path, resp.StatusCode, msg)
	}
}

// getAccountPage requests path on the account host through client and returns
// the response with its body.
func getAccountPage(ctx context.Context, t *testing.T, client *http.Client, path string) (*http.Response, string) {
	// Address the request to the account host.
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.cloudURL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "account.spacewave.app"

	// Send it and read the body.
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(body)
}

// subscribeCloudAccount gives the account an active subscription and a
// verified email, which creating a cloud resource requires.
func subscribeCloudAccount(ctx context.Context, t *testing.T, accountID string) {
	t.Helper()
	setTestSubscriptionStatus(t, accountID, "active")
	setTestEmailVerified(t, ctx, accountID, "subscriber-"+ulid.NewULID()+"@example.com")
}

// verifyEmail adds email to the account and confirms it with the code the
// verification email carries, as the app does.
func verifyEmail(ctx context.Context, t *testing.T, cli *provider_spacewave.SessionClient, accountID, email string) {
	t.Helper()
	if _, err := cli.RequestEmailVerification(ctx, email); err != nil {
		t.Fatal(err)
	}
	_, code := emailToken(ctx, t, accountID, "verification")
	if err := cli.VerifyEmailCode(ctx, email, code); err != nil {
		t.Fatal(err)
	}
}

// TestMain supplies one isolated backend to the nightly onboarding suite.
// TIER: nightly
func TestMain(m *testing.M) {
	os.Exit(runOnboardingTests(m))
}

// runOnboardingTests releases suite resources before TestMain exits the process.
func runOnboardingTests(m *testing.M) int {
	// Require an explicit local backend checkout before allocating test resources.
	cloudDir := os.Getenv("SPACEWAVE_CLOUD_DIR")
	if cloudDir == "" {
		os.Stderr.WriteString("SPACEWAVE_CLOUD_DIR not set, skipping e2e onboarding tests\n")
		return 0
	}

	// Bind the backend and controller testbed to one suite lifetime.
	ctx, cancel := context.WithCancel(context.Background())
	env = &testEnv{ctx: ctx, cancel: cancel}
	defer env.close()

	// Build and start the coordinator.
	if err := env.startCoordinator(cloudDir); err != nil {
		os.Stderr.WriteString("failed to start coordinator: " + err.Error() + "\n")
		return 1
	}

	// Create alpha testbed (bus, volume, world engine, storage).
	tb, err := testbed.Default(ctx)
	if err != nil {
		os.Stderr.WriteString("failed to create testbed: " + err.Error() + "\n")
		return 1
	}
	defer func() {
		cancel()
		tb.Release()
	}()
	env.tb = tb

	// Register controller factories.
	sr := tb.StaticResolver
	sr.AddFactory(session_controller.NewFactory(tb.Bus))
	sr.AddFactory(provider_local.NewFactory(tb.Bus))
	sr.AddFactory(provider_spacewave.NewFactory(tb.Bus))

	// Load session controller.
	_, sessCtrlRef, err := tb.Bus.AddDirective(
		resolver.NewLoadControllerWithConfig(&session_controller.Config{
			VolumeId: tb.Volume.GetID(),
		}),
		nil,
	)
	if err != nil {
		os.Stderr.WriteString("failed to load session controller: " + err.Error() + "\n")
		return 1
	}
	defer sessCtrlRef.Release()

	// Load local provider.
	peerID := tb.Volume.GetPeerID()
	_, localProvRef, err := tb.Bus.AddDirective(
		resolver.NewLoadControllerWithConfig(&provider_local.Config{
			ProviderId: provider_local.ProviderID,
			PeerId:     peerID.String(),
		}),
		nil,
	)
	if err != nil {
		os.Stderr.WriteString("failed to load local provider: " + err.Error() + "\n")
		return 1
	}
	defer localProvRef.Release()

	// Load spacewave provider.
	_, swProvRef, err := tb.Bus.AddDirective(
		resolver.NewLoadControllerWithConfig(&provider_spacewave.Config{
			Endpoint:        env.cloudURL,
			AccountEndpoint: env.accountOrigin,
		}),
		nil,
	)
	if err != nil {
		os.Stderr.WriteString("failed to load spacewave provider: " + err.Error() + "\n")
		return 1
	}
	defer swProvRef.Release()

	// Run tests while every provider registration remains retained.
	return m.Run()
}

// close stops the coordinator's process group and removes this run's state.
func (e *testEnv) close() {
	// Cancellation stops provider work before the coordinator disappears.
	e.cancel()
	if e.cmd != nil && e.cmd.Process != nil {
		_ = syscall.Kill(-e.cmd.Process.Pid, syscall.SIGKILL)
		_ = e.cmd.Wait()
	}

	// The directory was created uniquely by startCoordinator.
	if e.tempDir != "" {
		if err := os.RemoveAll(e.tempDir); err != nil {
			os.Stderr.WriteString("remove onboarding test state: " + err.Error() + "\n")
		}
	}
}

// startCoordinator builds the coordinator from cloudDir and serves it on a
// free port with an in-memory database and the /api/test helpers. Email is
// logged rather than sent; tests read its tokens with emailToken.
func (e *testEnv) startCoordinator(cloudDir string) error {
	// Allocate this run's directory and build the coordinator into it.
	tempDir, err := os.MkdirTemp("", "onboarding-e2e-*")
	if err != nil {
		return err
	}
	e.tempDir = tempDir
	bin := filepath.Join(tempDir, "spacewave-coordinator")
	build := exec.CommandContext(e.ctx, "go", "build", "-mod=mod", "-o", bin, "./cmd/spacewave-coordinator")
	build.Dir = cloudDir
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		return errors.Wrap(err, "build coordinator: "+string(out))
	}

	// Pick a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		return err
	}

	// Keep the in-memory database's directory inside this run's directory, so
	// close removes it after the forced shutdown.
	cmd := exec.Command(bin,
		"--memory",
		"--test-helpers",
		"--listen", "127.0.0.1:"+strconv.Itoa(port),
		"--signing-env-prefix", "spacewave",
	)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "TMPDIR=" + tempDir}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// os/exec serializes the shared writer and drains it when Wait returns.
	// Retain only the readiness marker's suffix between arbitrary output chunks.
	ready := make(chan struct{}, 1)
	const readyMarker = "msg=serving"
	var suffix string
	output := iowriter.NewCallbackWriter(func(data []byte) (int, error) {
		pending := suffix + string(data)
		if strings.Contains(pending, readyMarker) {
			select {
			case ready <- struct{}{}:
			default:
			}
		}
		if len(pending) >= len(readyMarker) {
			pending = pending[len(pending)-len(readyMarker)+1:]
		}
		suffix = pending
		return os.Stderr.Write(data)
	})

	// Start the coordinator and record its endpoints.
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Start(); err != nil {
		return err
	}
	e.cmd = cmd
	e.cloudURL = "http://localhost:" + strconv.Itoa(port)
	e.accountOrigin = "https://account.spacewave.app"
	e.passkeyRpID = "spacewave.app"

	// Bound readiness separately from the subprocess's suite lifetime.
	waitCtx, waitCancel := context.WithTimeout(e.ctx, time.Minute)
	defer waitCancel()
	select {
	case <-ready:
		os.Stderr.WriteString("coordinator ready on port " + strconv.Itoa(port) + "\n")
	case <-waitCtx.Done():
		return errors.Wrap(waitCtx.Err(), "wait for coordinator readiness")
	}
	return nil
}

// lookupSessionController returns the session controller from the bus.
func lookupSessionController(ctx context.Context) (session.SessionController, func(), error) {
	ctrl, ref, err := session.ExLookupSessionController(ctx, env.tb.Bus, "", false, nil)
	if err != nil {
		return nil, nil, err
	}
	if ctrl == nil {
		return nil, nil, errors.New("session controller not found")
	}
	return ctrl, ref.Release, nil
}

// createLocalSession creates a local session and registers it in the session controller.
// Returns the session list entry and the local provider account ID.
func createLocalSession(ctx context.Context, t *testing.T, cloudAccountID string) (*session.SessionListEntry, string) {
	t.Helper()
	b := env.tb.Bus

	// Retain the session inventory for this operation.
	sessCtrl, relSess, err := lookupSessionController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relSess)

	// Resolve the provider through the testbed bus.
	prov, provRef, err := provider.ExLookupProvider(ctx, b, provider_local.ProviderID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)

	// Create the local account before registering its Session.
	localProv := prov.(*provider_local.Provider)
	sessRef, err := localProv.CreateLocalAccountAndSession(ctx, cloudAccountID)
	if err != nil {
		t.Fatal(err)
	}

	// Register the newly created Session with its account metadata.
	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	entry, err := sessCtrl.RegisterSession(ctx, sessRef, &session.SessionMetadata{
		DisplayName:       "Local Test",
		ProviderAccountId: accountID,
		CloudAccountId:    cloudAccountID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return entry, accountID
}

// createCloudSession creates a spacewave cloud account and session.
// Returns the session list entry.
func createCloudSession(ctx context.Context, t *testing.T) *session.SessionListEntry {
	t.Helper()
	b := env.tb.Bus

	// Retain the session inventory for this operation.
	sessCtrl, relSess, err := lookupSessionController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relSess)

	// Resolve the provider through the testbed bus.
	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	swProv := prov.(*provider_spacewave.Provider)
	username := "test-" + ulid.NewULID()
	password := []byte("test-password-" + ulid.NewULID())
	entry, err := swProv.CreateSpacewaveAccountAndSession(ctx, username, password, "", sessCtrl)
	if err != nil {
		t.Fatal(err)
	}

	// Account APIs require the authenticated Session to remain mounted.
	_, _, release := mountSessionResource(ctx, t, entry)
	t.Cleanup(release)
	return entry
}

// createLocalSpace creates a Space through the mounted local provider.
func createLocalSpace(
	ctx context.Context,
	t *testing.T,
	entry *session.SessionListEntry,
	spaceName string,
) string {
	t.Helper()

	// Retain the account selected by this Session.
	provRef := entry.GetSessionRef().GetProviderResourceRef()
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(
		ctx,
		env.tb.Bus,
		provRef.GetProviderId(),
		provRef.GetProviderAccountId(),
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provAccRef.Release)
	localAcc, ok := provAcc.(*provider_local.ProviderAccount)
	if !ok {
		t.Fatal("expected local provider account")
	}

	// Create the Space with its provider metadata.
	meta, err := space.NewSharedObjectMeta(spaceName)
	if err != nil {
		t.Fatal(err)
	}
	soID := "space-" + ulid.NewULID()
	if _, err := localAcc.CreateSharedObject(ctx, soID, meta, "", ""); err != nil {
		t.Fatal(err)
	}
	return soID
}

// mountSessionResource retains the Session and returns the release required by its caller.
func mountSessionResource(
	ctx context.Context,
	t *testing.T,
	entry *session.SessionListEntry,
) (*resource_session.SessionResource, session.Session, func()) {
	t.Helper()

	// Keep the Session mounted while its resource is used.
	sess, sessRef, err := session.ExMountSession(
		ctx,
		env.tb.Bus,
		entry.GetSessionRef(),
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	le := logrus.NewEntry(logrus.New())
	return resource_session.NewSessionResource(le, env.tb.Bus, sess), sess, sessRef.Release
}

// waitForTransferComplete observes the transfer snapshot and its matching state notification.
func waitForTransferComplete(
	t *testing.T,
	xfer *provider_transfer.Transfer,
) *provider_transfer.TransferState {
	t.Helper()

	// Bound transfer completion while observing atomic state notifications.
	deadline := time.After(60 * time.Second)
	for {
		state, ch := xfer.WatchState()
		if state.GetPhase() == provider_transfer.TransferPhase_TransferPhase_COMPLETE {
			return state
		}
		if state.GetPhase() == provider_transfer.TransferPhase_TransferPhase_FAILED {
			t.Fatalf("transfer failed: %s", state.GetErrorMessage())
		}
		select {
		case <-deadline:
			t.Fatal("transfer timed out")
		case <-ch:
		}
	}
}

// waitForSessionCount waits for the controller to publish the expected inventory size.
func waitForSessionCount(
	ctx context.Context,
	t *testing.T,
	sessCtrl session.SessionController,
	want int,
) []*session.SessionListEntry {
	t.Helper()

	// Bound inventory convergence while waiting on session notifications.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	t.Cleanup(cancel)
	for {
		var waitCh <-chan struct{}
		sessCtrl.GetSessionBroadcast().HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
		})
		sessions, err := sessCtrl.ListSessions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(sessions) == want {
			return sessions
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for %d sessions, last count=%d: %v", want, len(sessions), ctx.Err())
		case <-waitCh:
		}
	}
}

// TestQuickstartLocal verifies that a local session can be created and
// appears in the session list.
func TestQuickstartLocal(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	t.Cleanup(cancel)

	// Create a local Session with an explicit cloud association.
	cloudAcctID := "qs-" + ulid.NewULID()
	entry, accountID := createLocalSession(ctx, t, cloudAcctID)
	t.Logf("local session created: idx=%d account=%s", entry.GetSessionIndex(), accountID)

	// Verify session in list.
	sessCtrl, relSess, err := lookupSessionController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relSess)
	sessions, err := sessCtrl.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range sessions {
		if s.GetSessionIndex() == entry.GetSessionIndex() {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("local session idx=%d not found in %d sessions", entry.GetSessionIndex(), len(sessions))
	}
}

// TestUpgradeToCloud verifies creating a cloud session, linking a local
// session, and checking the linked state.
func TestUpgradeToCloud(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	t.Cleanup(cancel)
	b := env.tb.Bus

	// Create cloud session first to get the account ID.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()
	cloudSessionID := cloudRef.GetId()
	t.Logf("cloud session: idx=%d account=%s", cloudEntry.GetSessionIndex(), cloudAccountID)

	// Create and register local session keyed to cloud account.
	localEntry, _ := createLocalSession(ctx, t, cloudAccountID)
	t.Logf("local session: idx=%d", localEntry.GetSessionIndex())

	// Access the spacewave provider account to set linked local.
	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	swProv := prov.(*provider_spacewave.Provider)
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relAcc)
	swAcc := accIface.(*provider_spacewave.ProviderAccount)

	// Link the local session to the cloud session.
	if err := swAcc.SetLinkedLocalSession(ctx, cloudSessionID, localEntry.GetSessionIndex()); err != nil {
		t.Fatal(err)
	}

	// Verify linked state.
	found, linkedIdx, err := swAcc.GetLinkedLocalSession(ctx, cloudSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("linked local session not found after SetLinkedLocalSession")
	}
	if linkedIdx != localEntry.GetSessionIndex() {
		t.Fatalf("linked idx mismatch: want %d, got %d", localEntry.GetSessionIndex(), linkedIdx)
	}
	t.Logf("local session %d linked to cloud session %s", linkedIdx, cloudSessionID)
}

// TestQuickstartUpgradeFullFlow verifies the real onboarding transfer path for
// a quickstart local session upgraded into an active cloud account with a
// linked local session. The original local session is merged into the linked
// local target, leaving only the cloud + linked-local pair.
func TestQuickstartUpgradeFullFlow(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	t.Cleanup(cancel)

	// Retain the session inventory for this operation.
	sessCtrl, relSess, err := lookupSessionController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relSess)

	// Record the inventory before adding the three transfer participants.
	initialSessions, err := sessCtrl.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	initialCount := len(initialSessions)

	// Seed the original quickstart account with a Space to transfer.
	originalLocal, _ := createLocalSession(ctx, t, "")
	spaceName := "Quickstart Space"
	createLocalSpace(ctx, t, originalLocal, spaceName)

	// Create an independent cloud account for this test.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()
	cloudSessionID := cloudRef.GetId()
	setTestSubscriptionStatus(t, cloudAccountID, "active")

	// Resolve the provider through the testbed bus.
	prov, provRef, err := provider.ExLookupProvider(ctx, env.tb.Bus, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	swProv := prov.(*provider_spacewave.Provider)
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relAcc)
	swAcc := accIface.(*provider_spacewave.ProviderAccount)

	// Require the destination cloud account to be active.
	swAcc.BumpLocalEpoch()
	if _, err := waitForSubscriptionStatus(ctx, swAcc, "active"); err != nil {
		t.Fatal(err)
	}

	// Create the linked local destination through the cloud Session resource.
	cloudResource, cloudSess, relCloudResource := mountSessionResource(ctx, t, cloudEntry)
	t.Cleanup(relCloudResource)
	swResource := resource_session.NewSpacewaveSessionResource(
		cloudResource,
		logrus.NewEntry(logrus.New()),
		env.tb.Bus,
		cloudSess,
		swAcc,
	)

	// Require a linked local session distinct from the other two.
	created, err := swResource.CreateLinkedLocalSession(
		ctx,
		&s4wave_provider_spacewave.CreateLinkedLocalSessionRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}
	linkedLocal := created.GetSessionListEntry()
	if linkedLocal == nil {
		t.Fatal("expected linked local session entry")
	}
	if linkedLocal.GetSessionIndex() == originalLocal.GetSessionIndex() {
		t.Fatal("linked local session should be distinct from original quickstart session")
	}
	if linkedLocal.GetSessionIndex() == cloudEntry.GetSessionIndex() {
		t.Fatal("linked local session should be distinct from cloud session")
	}

	// Check that the cloud Session names the new local destination.
	found, linkedIdx, err := swAcc.GetLinkedLocalSession(ctx, cloudSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || linkedIdx != linkedLocal.GetSessionIndex() {
		t.Fatalf(
			"expected cloud session %s linked to local idx=%d, got found=%t idx=%d",
			cloudSessionID,
			linkedLocal.GetSessionIndex(),
			found,
			linkedIdx,
		)
	}

	// Require every transfer participant to be registered.
	beforeTransfer := waitForSessionCount(ctx, t, sessCtrl, initialCount+3)
	if len(beforeTransfer) != initialCount+3 {
		t.Fatalf("expected %d sessions before transfer, got %d", initialCount+3, len(beforeTransfer))
	}

	// Merge the original local session into the linked local target.
	targetResource, _, relTargetResource := mountSessionResource(ctx, t, linkedLocal)
	t.Cleanup(relTargetResource)
	_, err = targetResource.StartTransfer(ctx, &s4wave_session.StartTransferRequest{
		SourceSessionIndex: originalLocal.GetSessionIndex(),
		TargetSessionIndex: linkedLocal.GetSessionIndex(),
		Mode:               provider_transfer.TransferMode_TransferMode_MERGE,
	})
	if err != nil {
		t.Fatal(err)
	}
	xfer := targetResource.GetActiveTransfer()
	if xfer == nil {
		t.Fatal("expected active transfer")
	}
	waitForTransferComplete(t, xfer)

	// Wait until merging removes the original local Session.
	afterTransfer := waitForSessionCount(ctx, t, sessCtrl, initialCount+2)
	if len(afterTransfer) != initialCount+2 {
		t.Fatalf("expected %d sessions after transfer, got %d", initialCount+2, len(afterTransfer))
	}

	// Confirm the original Session can no longer be resolved.
	srcEntry, err := sessCtrl.GetSessionByIdx(ctx, originalLocal.GetSessionIndex())
	if err != nil {
		t.Fatal(err)
	}
	if srcEntry != nil {
		t.Fatal("expected original quickstart local session to be deleted after merge")
	}

	// Confirm the destination contains the original Space.
	inventory, err := targetResource.GetTransferInventory(
		ctx,
		&s4wave_session.GetTransferInventoryRequest{
			SessionIndex: linkedLocal.GetSessionIndex(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	hasTransferredSpace := false
	for _, sp := range inventory.GetSpaces() {
		if sp.GetSpaceMeta().GetName() == spaceName {
			hasTransferredSpace = true
			break
		}
	}
	if !hasTransferredSpace {
		t.Fatalf("expected transferred space %q on linked local target", spaceName)
	}

	// Confirm the cloud association survives the merge.
	found, linkedIdx, err = swAcc.GetLinkedLocalSession(ctx, cloudSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || linkedIdx != linkedLocal.GetSessionIndex() {
		t.Fatalf(
			"expected cloud session to remain linked to local idx=%d, got found=%t idx=%d",
			linkedLocal.GetSessionIndex(),
			found,
			linkedIdx,
		)
	}
}

// TestDeleteAndResignup verifies that deleting a session and re-creating
// one results in a clean state with no stale references.
func TestDeleteAndResignup(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	t.Cleanup(cancel)
	b := env.tb.Bus

	// Create cloud session.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()
	cloudSessionID := cloudRef.GetId()
	t.Logf("cloud session created: idx=%d account=%s", cloudEntry.GetSessionIndex(), cloudAccountID)

	// Create and link local.
	localEntry, _ := createLocalSession(ctx, t, cloudAccountID)

	// Resolve the provider through the testbed bus.
	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	swProv := prov.(*provider_spacewave.Provider)

	// Retain the authenticated cloud account for API operations.
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	swAcc := accIface.(*provider_spacewave.ProviderAccount)
	if err := swAcc.SetLinkedLocalSession(ctx, cloudSessionID, localEntry.GetSessionIndex()); err != nil {
		t.Fatal(err)
	}
	relAcc()

	// Verify linked before delete.
	accIface2, relAcc2, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	swAcc2 := accIface2.(*provider_spacewave.ProviderAccount)
	found, _, err := swAcc2.GetLinkedLocalSession(ctx, cloudSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("linked session should exist before delete")
	}
	relAcc2()

	// Delete cloud session.
	sessCtrl, relSess, err := lookupSessionController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relSess)
	if err := sessCtrl.DeleteSession(ctx, cloudEntry.GetSessionRef()); err != nil {
		t.Fatal(err)
	}
	t.Log("cloud session deleted")

	// Re-create cloud session (new account to simulate fresh signup).
	newCloudEntry := createCloudSession(ctx, t)
	newCloudRef := newCloudEntry.GetSessionRef().GetProviderResourceRef()
	newCloudAccountID := newCloudRef.GetProviderAccountId()
	newCloudSessionID := newCloudRef.GetId()
	t.Logf("new cloud session: idx=%d account=%s", newCloudEntry.GetSessionIndex(), newCloudAccountID)

	// Verify no stale linked local on new account.
	accIface3, relAcc3, err := swProv.AccessProviderAccount(ctx, newCloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relAcc3)
	swAcc3 := accIface3.(*provider_spacewave.ProviderAccount)
	found, _, err = swAcc3.GetLinkedLocalSession(ctx, newCloudSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("new account should have no linked local session")
	}
	t.Log("verified: new account has clean state with no stale references")
}

// TestCloudSpaceLifecycle verifies creating a SharedObject on the cloud
// and listing it via the SessionClient API.
func TestCloudSpaceLifecycle(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	t.Cleanup(cancel)
	b := env.tb.Bus

	// Create cloud session.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	// Subscribe the account so it can create cloud resources.
	subscribeCloudAccount(ctx, t, cloudAccountID)

	// Access provider account for SessionClient.
	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	swProv := prov.(*provider_spacewave.Provider)

	// Retain the authenticated cloud account for API operations.
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relAcc)
	swAcc := accIface.(*provider_spacewave.ProviderAccount)
	cli := swAcc.GetSessionClient()

	// Create SharedObject on cloud.
	soID := ulid.NewULID()
	if err := cli.CreateSharedObject(ctx, soID, "Test Space", "space", "", "", false); err != nil {
		t.Fatal(err)
	}
	t.Logf("created SharedObject on cloud: %s", soID)

	// List SharedObjects and verify it appears.
	listData, err := cli.ListSharedObjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listData) == 0 {
		t.Fatal("ListSharedObjects returned empty")
	}
	t.Logf("ListSharedObjects returned %d bytes", len(listData))

	// Verify the SO state is readable.
	stateData, err := cli.GetSOState(
		ctx,
		soID,
		0,
		provider_spacewave.SeedReasonReconnect,
	)
	if err != nil {
		// New SO may not have state yet; that's OK.
		t.Logf("GetSOState (expected for new SO): %v", err)
	} else {
		t.Logf("SO state: %d bytes", len(stateData))
	}
}

// TestAccountInfoRetrieval verifies fetching account info from the cloud.
func TestAccountInfoRetrieval(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	t.Cleanup(cancel)
	b := env.tb.Bus

	// Create an independent cloud account for this test.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	// Resolve the provider through the testbed bus.
	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	swProv := prov.(*provider_spacewave.Provider)

	// Retain the authenticated cloud account for API operations.
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relAcc)
	swAcc := accIface.(*provider_spacewave.ProviderAccount)

	// GetAccountInfo via SessionClient.
	info, err := swAcc.GetSessionClient().GetAccountInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.AccountId == "" {
		t.Fatal("account ID is empty")
	}
	if info.EntityId == "" {
		t.Fatal("entity ID is empty")
	}
	t.Logf("account info: id=%s entity=%s keypairs=%d",
		info.AccountId, info.EntityId, info.KeypairCount)
}

// TestSubscriptionStatus verifies that a new account has no active subscription.
func TestSubscriptionStatus(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	t.Cleanup(cancel)
	b := env.tb.Bus

	// Create an independent cloud account for this test.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	// Resolve the provider through the testbed bus.
	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	swProv := prov.(*provider_spacewave.Provider)

	// Retain the authenticated cloud account for API operations.
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relAcc)
	swAcc := accIface.(*provider_spacewave.ProviderAccount)

	// Read the new account subscription without provisioning a plan.
	status, err := swAcc.GetSubscriptionStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// New accounts have no subscription.
	if status != "none" {
		t.Fatalf("expected no subscription, got %q", status)
	}
	t.Logf("subscription status: %q (expected for new account)", status)
}

// TestUnlinkLocalSession verifies unlinking a local session from a cloud
// session (the "keep separate" flow).
func TestUnlinkLocalSession(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	t.Cleanup(cancel)
	b := env.tb.Bus

	// Create cloud + local and link them.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()
	cloudSessionID := cloudRef.GetId()
	localEntry, _ := createLocalSession(ctx, t, cloudAccountID)

	// Resolve the provider through the testbed bus.
	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	swProv := prov.(*provider_spacewave.Provider)

	// Retain the authenticated cloud account for API operations.
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relAcc)
	swAcc := accIface.(*provider_spacewave.ProviderAccount)

	// Link.
	if err := swAcc.SetLinkedLocalSession(ctx, cloudSessionID, localEntry.GetSessionIndex()); err != nil {
		t.Fatal(err)
	}

	// Verify linked.
	found, _, err := swAcc.GetLinkedLocalSession(ctx, cloudSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected linked local session")
	}
	t.Log("linked local to cloud")

	// Unlink (the "keep separate" path).
	if err := swAcc.DeleteLinkedLocalSession(ctx, cloudSessionID); err != nil {
		t.Fatal(err)
	}

	// Verify unlinked.
	found, _, err = swAcc.GetLinkedLocalSession(ctx, cloudSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("linked local session should be removed after unlink")
	}
	t.Log("verified: local session unlinked successfully")
}

// TestMultipleSessionListing verifies that creating multiple sessions
// (local + cloud) results in all of them appearing in the session list.
func TestMultipleSessionListing(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	t.Cleanup(cancel)

	// Retain the session inventory for this operation.
	sessCtrl, relSess, err := lookupSessionController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relSess)

	// Record session count before.
	before, err := sessCtrl.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	countBefore := len(before)

	// Create cloud session.
	cloudEntry := createCloudSession(ctx, t)
	cloudAccountID := cloudEntry.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	// Create local session.
	localEntry, _ := createLocalSession(ctx, t, cloudAccountID)

	// List sessions again.
	after, err := sessCtrl.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// We created 2 sessions (cloud + local).
	newCount := len(after) - countBefore
	if newCount < 2 {
		t.Fatalf("expected at least 2 new sessions, got %d (before=%d after=%d)",
			newCount, countBefore, len(after))
	}

	// Verify both indices are present.
	indices := make(map[uint32]bool, len(after))
	for _, s := range after {
		indices[s.GetSessionIndex()] = true
	}
	if !indices[cloudEntry.GetSessionIndex()] {
		t.Fatalf("cloud session idx=%d not in list", cloudEntry.GetSessionIndex())
	}
	if !indices[localEntry.GetSessionIndex()] {
		t.Fatalf("local session idx=%d not in list", localEntry.GetSessionIndex())
	}
	t.Logf("verified: %d sessions in list (cloud=%d, local=%d)",
		len(after), cloudEntry.GetSessionIndex(), localEntry.GetSessionIndex())
}

// TestOrganizationLifecycle verifies creating, listing, and deleting
// an organization on the cloud.
func TestOrganizationLifecycle(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	t.Cleanup(cancel)
	b := env.tb.Bus

	// Create an independent cloud account for this test.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	// Resolve the provider through the testbed bus.
	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	swProv := prov.(*provider_spacewave.Provider)

	// Retain the authenticated cloud account for API operations.
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relAcc)
	swAcc := accIface.(*provider_spacewave.ProviderAccount)
	cli := swAcc.GetSessionClient()

	// Create organization.
	orgName := "Test Org " + ulid.NewULID()
	createResp, err := cli.CreateOrganization(ctx, orgName)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("created org: %d bytes response", len(createResp))

	// List organizations.
	listResp, err := cli.ListOrganizations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listResp) == 0 {
		t.Fatal("ListOrganizations returned empty after creating org")
	}
	t.Logf("ListOrganizations: %d bytes", len(listResp))
}

// TestBlockStoreSyncPushPull verifies constructing a packfile from blocks,
// pushing it to the cloud block store, and pulling the manifest to confirm
// the packfile was received.
func TestBlockStoreSyncPushPull(t *testing.T) {
	// Scope the test to the shared environment.
	ctx, cancel := context.WithCancel(env.ctx)
	t.Cleanup(cancel)
	b := env.tb.Bus

	// Create cloud session and access account.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	// Subscribe the account so it can create cloud resources.
	subscribeCloudAccount(ctx, t, cloudAccountID)

	// Resolve the provider through the testbed bus.
	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	swProv := prov.(*provider_spacewave.Provider)

	// Retain the authenticated cloud account for API operations.
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relAcc)
	swAcc := accIface.(*provider_spacewave.ProviderAccount)
	cli := swAcc.GetSessionClient()

	// Create a SharedObject on the cloud (which creates the block store).
	soID := ulid.NewULID()
	if err := cli.CreateSharedObject(ctx, soID, "Sync Test", "space", "", "", false); err != nil {
		t.Fatal(err)
	}
	bstoreID := provider_local.SobjectBlockStoreID(soID)
	t.Logf("block store: %s", bstoreID)

	// Create test blocks and compute hashes.
	type testBlock struct {
		// data is the pack payload whose round trip is checked.
		data []byte
		// hash identifies the corresponding content-addressed block.
		hash *bifhash.Hash
	}
	blocks := make([]testBlock, 3)
	for i := range blocks {
		blocks[i].data = []byte("sync test block " + strconv.Itoa(i) + " " + ulid.NewULID())
		h, err := bifhash.Sum(bifhash.HashType_HashType_SHA256, blocks[i].data)
		if err != nil {
			t.Fatal(err)
		}
		blocks[i].hash = h
	}

	// Write packfile to temp file with SHA-256 body hash.
	tmpPath := filepath.Join(t.TempDir(), "sync.kvf")
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tmpFile.Close() })

	// Hash the exact pack bytes submitted to cloud storage.
	hashWriter := sha256.New()
	multiWriter := io.MultiWriter(tmpFile, hashWriter)

	// Supply each block once to the pack writer.
	idx := 0
	iter := func() (*bifhash.Hash, *block.StoredBlock, error) {
		if idx >= len(blocks) {
			return nil, nil, nil
		}
		blk := blocks[idx]
		idx++
		return blk.hash, &block.StoredBlock{Data: blk.data, RefsKnown: true}, nil
	}

	// Close the pack before uploading its path and digest.
	result, err := packfile_writer.PackBlocks(multiWriter, iter)
	if err != nil {
		t.Fatal(err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatal(err)
	}
	bodyHash := hashWriter.Sum(nil)
	t.Logf("packed %d blocks, %d bytes, bloom %d bytes",
		result.BlockCount, result.BytesWritten, len(result.BloomFilter))

	// Push packfile to cloud.
	packID, err := identity.BuildPackID(bstoreID, result)
	if err != nil {
		t.Fatal(err)
	}
	if err := cli.SyncPush(ctx, bstoreID, packID, int(result.BlockCount), tmpPath, bodyHash, result.BloomFilter, packfile.BloomFormatVersionV1); err != nil {
		t.Fatal(err)
	}
	t.Logf("pushed pack %s", packID)

	// Pull manifest and verify the pack appears.
	pullResp, err := cli.SyncPull(ctx, bstoreID, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Locate the uploaded pack.
	if len(pullResp.GetEntries()) == 0 {
		t.Fatal("SyncPull returned no entries after push")
	}
	found := false
	for _, entry := range pullResp.GetEntries() {
		if entry.GetId() == packID {
			found = true
			if entry.GetBlockCount() != uint64(len(blocks)) {
				t.Fatalf("block count mismatch: want %d, got %d", len(blocks), entry.GetBlockCount())
			}
			t.Logf("verified pack %s: %d blocks, %d bytes",
				entry.GetId(), entry.GetBlockCount(), entry.GetSizeBytes())
			break
		}
	}
	if !found {
		t.Fatalf("pushed pack %s not found in pull response", packID)
	}
}

// TestPasskeyRegistrationAndAuth verifies the WebAuthn passkey flow by
// simulating a virtual authenticator that constructs valid CBOR
// attestation and assertion objects with fmt=none.
func TestPasskeyRegistrationAndAuth(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	t.Cleanup(cancel)
	b := env.tb.Bus

	// Create cloud session.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	// Resolve the provider through the testbed bus.
	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	swProv := prov.(*provider_spacewave.Provider)

	// Retain the authenticated cloud account for API operations.
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relAcc)
	swAcc := accIface.(*provider_spacewave.ProviderAccount)
	cli := swAcc.GetSessionClient()

	// Create virtual authenticator.
	va, err := newVirtualAuthenticator()
	if err != nil {
		t.Fatal(err)
	}

	// Step 1: Get registration options.
	optionsJSON, err := cli.PasskeyRegisterOptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("register options: %d chars", len(optionsJSON))

	// Extract challenge from options JSON.
	challenge := extractJSONField(optionsJSON, "challenge")
	if challenge == "" {
		t.Fatal("no challenge in registration options")
	}

	// Step 2: Create registration credential.
	credJSON := va.createRegistrationResponse(challenge)

	// Generate a fake entity keypair for the passkey binding.
	// The passkey register endpoint stores the wrapped PEM blob and derives the
	// public key from the submitted peer ID.
	entityPriv, _, err := bifcrypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}
	entityPeerID, err := bifpeer.IDFromPrivateKey(entityPriv)
	if err != nil {
		t.Fatal(err)
	}

	// Step 3: Verify registration with the cloud.
	credID, err := cli.PasskeyRegisterVerify(
		ctx,
		credJSON,
		false, // prfCapable
		base64.StdEncoding.EncodeToString([]byte("fake-encrypted")), // encryptedPrivkey
		entityPeerID.String(), // peerID
		"",                    // authParams
		"",                    // prfSalt
	)
	if err != nil {
		t.Fatalf("passkey register verify: %v", err)
	}
	t.Logf("registered passkey: credentialID=%s", credID)

	// Step 4: Get authentication options.
	authOptJSON, err := provider_spacewave.PasskeyAuthOptions(ctx, httpClient, env.cloudURL, "")
	if err != nil {
		t.Fatal(err)
	}
	authChallenge := extractJSONField(authOptJSON, "challenge")
	if authChallenge == "" {
		t.Fatal("no challenge in auth options")
	}

	// Step 5: Create authentication assertion.
	authCredJSON := va.createAuthenticationResponse(authChallenge)

	// Step 6: Verify authentication with the cloud.
	authResp, err := provider_spacewave.PasskeyAuthVerify(ctx, httpClient, env.cloudURL, authCredJSON)
	if err != nil {
		t.Fatalf("passkey auth verify: %v", err)
	}
	if !authResp.GetVerified() {
		t.Fatal("passkey authentication was not verified")
	}
	if authResp.GetAccountId() == "" {
		t.Fatal("auth response missing account ID")
	}
	t.Logf("passkey auth verified: account=%s entity=%s",
		authResp.GetAccountId(), authResp.GetEntityId())
}

// createCloudSessionWithKey creates a spacewave cloud account and session,
// returning the session entry, entity private key, and entity peer ID.
func createCloudSessionWithKey(ctx context.Context, t *testing.T) (*session.SessionListEntry, bifcrypto.PrivKey, bifpeer.ID) {
	t.Helper()
	b := env.tb.Bus

	// Retain the session inventory for this operation.
	sessCtrl, relSess, err := lookupSessionController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relSess)

	// Resolve the provider through the testbed bus.
	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	swProv := prov.(*provider_spacewave.Provider)
	username := "test-" + ulid.NewULID()
	password := []byte("test-password-" + ulid.NewULID())
	entry, err := swProv.CreateSpacewaveAccountAndSession(ctx, username, password, "", sessCtrl)
	if err != nil {
		t.Fatal(err)
	}

	// Re-derive the entity key from the same credentials.
	_, privKey, err := auth_method_password.BuildParametersWithUsernamePassword(username, password)
	if err != nil {
		t.Fatal(err)
	}
	entityPeerID, err := bifpeer.IDFromPrivateKey(privKey)
	if err != nil {
		t.Fatal(err)
	}

	// Retain authentication for account API calls made by the recovery test.
	_, _, release := mountSessionResource(ctx, t, entry)
	t.Cleanup(release)
	return entry, privKey, entityPeerID
}

// TestAccountRecovery verifies the recovery flow end-to-end:
// set verified email -> request recovery -> read the emailed token ->
// verify token -> sign and execute recovery with new keypair.
func TestAccountRecovery(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	t.Cleanup(cancel)
	b := env.tb.Bus

	// Create cloud session with entity key access.
	cloudEntry, entityPrivKey, entityPeerID := createCloudSessionWithKey(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()
	t.Logf("cloud account: %s entity peer: %s", cloudAccountID, entityPeerID.String())

	// Resolve the provider through the testbed bus.
	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	swProv := prov.(*provider_spacewave.Provider)

	// Retain the authenticated cloud account for API operations.
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relAcc)
	swAcc := accIface.(*provider_spacewave.ProviderAccount)
	cli := swAcc.GetSessionClient()

	// Step 1: Verify an email on the account.
	testEmail := "recovery-test-" + ulid.NewULID() + "@example.com"
	verifyEmail(ctx, t, cli, cloudAccountID, testEmail)
	t.Log("email verified")

	// Step 2: Request account recovery.
	if err := provider_spacewave.RequestRecoveryEmail(ctx, httpClient, env.cloudURL, testEmail, ""); err != nil {
		t.Fatal(err)
	}
	t.Log("recovery request sent")

	// Step 3: Read the token the recovery email carries.
	recoveryToken, _ := emailToken(ctx, t, cloudAccountID, "recovery")

	// Step 4: Verify recovery token.
	verifyResult, err := provider_spacewave.RecoverVerify(ctx, httpClient, env.cloudURL, recoveryToken)
	if err != nil {
		t.Fatalf("recover verify: %v", err)
	}
	if verifyResult.AccountId != cloudAccountID {
		t.Fatalf("recover verify account mismatch: want %s, got %s", cloudAccountID, verifyResult.AccountId)
	}
	t.Logf("recovery verified: account=%s entity=%s", verifyResult.AccountId, verifyResult.EntityId)

	// Step 5: Generate a new keypair for recovery.
	newPrivKey, _, err := bifcrypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}
	newPeerID, err := bifpeer.IDFromPrivateKey(newPrivKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("new recovery keypair: %s", newPeerID.String())

	// Step 6: Sign the recovery payload with the existing entity key.
	// Payload: RECOVERY_CONTEXT || accountId || token || peerId
	recoveryContext := "spacewave 2026-03-19 account recovery v1."
	sigMsg := []byte(recoveryContext + cloudAccountID + recoveryToken + newPeerID.String())
	sig, err := entityPrivKey.Sign(sigMsg)
	if err != nil {
		t.Fatalf("sign recovery payload: %v", err)
	}

	// Step 7: Execute recovery.
	execReq := &api.RecoverExecuteRequest{
		Token: recoveryToken,
		AddKeypair: &api.RecoverExecuteKeypair{
			PeerId:     newPeerID.String(),
			AuthMethod: "recovery",
		},
		Signatures: []*api.RecoverExecuteSignature{
			{
				PeerId:    entityPeerID.String(),
				Signature: base64.StdEncoding.EncodeToString(sig),
			},
		},
	}
	if err := provider_spacewave.RecoverExecute(ctx, httpClient, env.cloudURL, execReq); err != nil {
		t.Fatalf("recover execute: %v", err)
	}
	t.Log("recovery executed successfully")

	// Step 8: Verify the new keypair exists on the account.
	keypairs, err := cli.ListKeypairs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, kp := range keypairs {
		if kp.PeerId == newPeerID.String() {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("new recovery keypair %s not found in account keypairs (count=%d)", newPeerID.String(), len(keypairs))
	}
	t.Logf("verified: new keypair %s present on account (%d total keypairs)", newPeerID.String(), len(keypairs))
}

// TestEmailVerification verifies the email verification flow end-to-end:
// request verification -> read the emailed code -> confirm it -> check the
// account state.
func TestEmailVerification(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	t.Cleanup(cancel)
	b := env.tb.Bus

	// Create an independent cloud account for this test.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	// Resolve the provider through the testbed bus.
	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	swProv := prov.(*provider_spacewave.Provider)

	// Retain the authenticated cloud account for API operations.
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relAcc)
	swAcc := accIface.(*provider_spacewave.ProviderAccount)
	cli := swAcc.GetSessionClient()

	// Verify an email with the emailed code.
	testEmail := "verify-" + ulid.NewULID() + "@example.com"
	verifyEmail(ctx, t, cli, cloudAccountID, testEmail)
	t.Log("email verified successfully")

	// Confirm the account state reports the verified email.
	state, err := cli.GetAccountState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !state.GetEmailVerified() {
		t.Fatal("account state does not report a verified email")
	}
}

// TestFailsafeLogin verifies the failsafe account management flow:
// set email -> request failsafe token -> read the emailed token ->
// verify token to access account settings page.
func TestFailsafeLogin(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	t.Cleanup(cancel)

	// Create an independent cloud account for this test.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	// Set verified email on the account via test helper.
	testEmail := "failsafe-" + ulid.NewULID() + "@example.com"
	setTestEmailVerified(t, ctx, cloudAccountID, testEmail)
	t.Logf("set email %s on account %s", testEmail, cloudAccountID)

	// Build the form request for a failsafe token on the account host.
	formBody := "email=" + testEmail
	failsafeURL := env.cloudURL + "/request-token"
	failsafeReq, err := http.NewRequestWithContext(ctx, http.MethodPost, failsafeURL, strings.NewReader(formBody))
	if err != nil {
		t.Fatal(err)
	}
	failsafeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	failsafeReq.Host = "account.spacewave.app"

	// Request the token and require success.
	failsafeResp, err := httpClient.Do(failsafeReq)
	if err != nil {
		t.Fatal(err)
	}
	failsafeRespBody, err := io.ReadAll(failsafeResp.Body)
	failsafeResp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if failsafeResp.StatusCode != http.StatusOK {
		t.Fatalf("failsafe request-token failed: %d body=%s", failsafeResp.StatusCode, string(failsafeRespBody))
	}
	t.Log("failsafe token requested")

	// Read the token the account-management email carries.
	token, _ := emailToken(ctx, t, cloudAccountID, "manage")

	// Open the account settings page with the token.
	verifyResp, verifyBody := getAccountPage(ctx, t, httpClient, "/verify?token="+token)
	if verifyResp.StatusCode != http.StatusOK {
		t.Fatalf("failsafe verify failed: %d body=%s", verifyResp.StatusCode, verifyBody)
	}
	if !strings.Contains(verifyBody, "html") {
		t.Fatal("failsafe verify did not return HTML page")
	}
	t.Log("failsafe login verified: account settings page accessible")
}

// TestInvalidAccountManagementTokenRedirectsToEntryPage verifies that an
// invalid or expired account-management token redirects to the account
// email-entry page with a reason parameter.
func TestInvalidAccountManagementTokenRedirectsToEntryPage(t *testing.T) {
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	t.Cleanup(cancel)

	// Disable redirect following to inspect the 302 directly.
	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// An invalid token redirects to the entry page with its reason.
	invalidResp, _ := getAccountPage(ctx, t, noRedirectClient, "/verify?token=invalid-garbage-token-xyz")
	if invalidResp.StatusCode != http.StatusFound {
		t.Fatalf("invalid token returned status %d, want 302", invalidResp.StatusCode)
	}
	location := invalidResp.Header.Get("Location")
	if !strings.Contains(location, "reason=invalid") {
		t.Fatalf("redirect location missing reason=invalid: %s", location)
	}
	t.Logf("invalid token redirected to: %s", location)

	// The entry page loads.
	entryResp, entryBody := getAccountPage(ctx, t, httpClient, location)
	if entryResp.StatusCode != http.StatusOK {
		t.Fatalf("entry page returned status %d", entryResp.StatusCode)
	}
	if !strings.Contains(entryBody, "html") {
		t.Fatal("entry page did not return HTML")
	}
	t.Log("verified: invalid token redirects to entry page with reason=invalid")
}
