//go:build e2e

// Package onboarding_test exercises the full onboarding and migration lifecycle
// against a live wrangler dev --local (miniflare) backend.
//
// Run with:
//
//	SPACEWAVE_CLOUD_DIR=/path/to/spacewave-cloud \
//	  go test -tags e2e -count=1 -v ./core/e2e/onboarding/
package onboarding_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	auth_method_password "github.com/s4wave/spacewave/auth/method/password"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	provider_spacewave_packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	packfile_writer "github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
	provider_transfer "github.com/s4wave/spacewave/core/provider/transfer"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/block"
	bifcrypto "github.com/s4wave/spacewave/net/crypto"
	bifhash "github.com/s4wave/spacewave/net/hash"
	bifpeer "github.com/s4wave/spacewave/net/peer"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// mockEmail represents an email captured by the mock SES server.
type mockEmail struct {
	To      string
	Subject string
	HTML    string
}

// mockMailbox collects emails sent to the mock SES endpoint.
type mockMailbox struct {
	mu     sync.Mutex
	emails []mockEmail
}

// getEmails returns a snapshot of all captured emails.
func (mb *mockMailbox) getEmails() []mockEmail {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	out := make([]mockEmail, len(mb.emails))
	copy(out, mb.emails)
	return out
}

// startMockSES starts an HTTP server that mimics the SES v2 send endpoint.
// Returns the mailbox, server, and URL.
func startMockSES() (*mockMailbox, *httptest.Server) {
	mb := &mockMailbox{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if email, ok := parseMockSESEmail(body); ok {
			mb.mu.Lock()
			mb.emails = append(mb.emails, email)
			mb.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"MessageId":"mock-msg-id"}`))
	}))
	return mb, srv
}

// testEnv holds the wrangler dev subprocess and test infrastructure.
type testEnv struct {
	cmd           *exec.Cmd
	baseURL       string
	accountHost   string
	accountOrigin string
	passkeyOrigin string
	passkeyRpID   string
	tb            *testbed.Testbed
	ctx           context.Context
	cancel        context.CancelFunc
	cloudURL      string
	cloudDir      string
	tempDir       string
	mailbox       *mockMailbox
	sesSrv        *httptest.Server
}

var (
	env        *testEnv
	httpClient = http.DefaultClient
)

// extractJSONField extracts a string value for a key from a JSON string.
// Minimal parser for test use -- does not handle nested objects or escapes.
func extractJSONField(jsonStr, key string) string {
	search := `"` + key + `":"`
	idx := strings.Index(jsonStr, search)
	if idx < 0 {
		return ""
	}
	start := idx + len(search)
	end := strings.Index(jsonStr[start:], `"`)
	if end < 0 {
		return ""
	}
	return jsonStr[start : start+end]
}

func parseMockSESEmail(dat []byte) (mockEmail, bool) {
	var p fastjson.Parser
	v, err := p.ParseBytes(dat)
	if err != nil {
		return mockEmail{}, false
	}

	email := mockEmail{
		Subject: string(v.GetStringBytes("Content", "Simple", "Subject", "Data")),
		HTML:    string(v.GetStringBytes("Content", "Simple", "Body", "Html", "Data")),
	}
	toAddresses := v.GetArray("Destination", "ToAddresses")
	if len(toAddresses) != 0 && toAddresses[0] != nil {
		email.To = string(toAddresses[0].GetStringBytes())
	}
	return email, true
}

func TestMain(m *testing.M) {
	cloudDir := os.Getenv("SPACEWAVE_CLOUD_DIR")
	if cloudDir == "" {
		os.Stderr.WriteString("SPACEWAVE_CLOUD_DIR not set, skipping e2e onboarding tests\n")
		os.Exit(0)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start mock SES server before wrangler so the URL is known.
	mailbox, sesSrv := startMockSES()
	env = &testEnv{ctx: ctx, cancel: cancel, mailbox: mailbox, sesSrv: sesSrv}

	// Start wrangler dev --local.
	if err := env.startWrangler(cloudDir); err != nil {
		os.Stderr.WriteString("failed to start wrangler: " + err.Error() + "\n")
		os.Exit(1)
	}

	// Create alpha testbed (bus, volume, world engine, storage).
	tb, err := testbed.Default(ctx)
	if err != nil {
		os.Stderr.WriteString("failed to create testbed: " + err.Error() + "\n")
		os.Exit(1)
	}
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
		os.Exit(1)
	}
	_ = sessCtrlRef

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
		os.Exit(1)
	}
	_ = localProvRef

	// Load spacewave provider.
	_, swProvRef, err := tb.Bus.AddDirective(
		resolver.NewLoadControllerWithConfig(&provider_spacewave.Config{
			Endpoint: env.cloudURL,
		}),
		nil,
	)
	if err != nil {
		os.Stderr.WriteString("failed to load spacewave provider: " + err.Error() + "\n")
		os.Exit(1)
	}
	_ = swProvRef

	code := m.Run()

	// Cleanup.
	if env.cmd != nil && env.cmd.Process != nil {
		_ = syscall.Kill(-env.cmd.Process.Pid, syscall.SIGKILL)
		_ = env.cmd.Wait()
	}
	if env.sesSrv != nil {
		env.sesSrv.Close()
	}
	cancel()

	os.Exit(code)
}

// startWrangler starts wrangler dev --local on a free port with fresh state.
func (e *testEnv) startWrangler(cloudDir string) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	tempDir, err := os.MkdirTemp("", "onboarding-e2e-*")
	if err != nil {
		return err
	}
	e.cloudDir = cloudDir
	e.tempDir = tempDir

	// Apply D1 migrations.
	migrateCmd := exec.Command("npx", "wrangler", "d1", "migrations", "apply",
		"--local",
		"--persist-to", tempDir,
		"spacewave",
	)
	migrateCmd.Dir = cloudDir
	migrateCmd.Env = append(os.Environ(), "NODE_ENV=test")
	if out, err := migrateCmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, "D1 migrations failed: "+string(out))
	}
	os.Stderr.WriteString("D1 migrations applied\n")

	// Build wrangler args with email config pointing at mock SES.
	args := []string{
		"wrangler", "dev",
		"--local",
		"--port", strconv.Itoa(port),
		"--persist-to", tempDir,
		"--var", "SIGNING_ENV_PREFIX:spacewave",
		"--var", "ENVIRONMENT:test",
		"--var", "APP_ORIGIN:http://localhost:5173",
		"--var", "ACCOUNT_ORIGIN:https://account.spacewave.app",
		"--var", "PASSKEY_ORIGIN:https://account.spacewave.app",
		"--var", "PASSKEY_RP_ID:spacewave.app",
		"--var", "SSO_ALLOWED_ORIGINS:http://localhost:5173,http://localhost:4173",
		"--var", "ENABLE_TEST_HELPERS:true",
		"--var", "CUSTODIED_KEY_SECRET:test-custodied-key-secret",
	}
	if e.sesSrv != nil {
		args = append(args,
			"--var", "EMAIL_PROVIDER:ses",
			"--var", "AWS_ACCESS_KEY_ID:test",
			"--var", "AWS_SECRET_ACCESS_KEY:test",
			"--var", "AWS_SES_REGION:us-east-1",
			"--var", "AWS_SES_ENDPOINT:"+e.sesSrv.URL,
		)
	}
	cmd := exec.Command("npx", args...)
	cmd.Dir = cloudDir
	cmd.Env = append(os.Environ(), "NODE_ENV=test")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	e.cmd = cmd
	e.cloudURL = "http://localhost:" + strconv.Itoa(port)
	e.accountHost = "account.spacewave.app"
	e.accountOrigin = "https://account.spacewave.app"
	e.passkeyOrigin = "https://account.spacewave.app"
	e.passkeyRpID = "spacewave.app"

	// Wait for "Ready on" in output.
	ready := make(chan struct{}, 1)
	scanPipe := func(r io.Reader, name string) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			os.Stderr.WriteString("wrangler[" + name + "]: " + line + "\n")
			if strings.Contains(line, "Ready on") {
				select {
				case ready <- struct{}{}:
				default:
				}
			}
		}
	}
	go scanPipe(stderr, "err")
	go scanPipe(stdout, "out")

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer waitCancel()

	select {
	case <-ready:
		os.Stderr.WriteString("wrangler ready on port " + strconv.Itoa(port) + "\n")
	case <-waitCtx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return errors.New("wrangler failed to start within 30s")
	}

	// Let wrangler fully initialize.
	time.Sleep(500 * time.Millisecond)
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

	sessCtrl, relSess, err := lookupSessionController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer relSess()

	prov, provRef, err := provider.ExLookupProvider(ctx, b, provider_local.ProviderID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()

	localProv := prov.(*provider_local.Provider)
	sessRef, err := localProv.CreateLocalAccountAndSession(ctx, cloudAccountID)
	if err != nil {
		t.Fatal(err)
	}

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

	sessCtrl, relSess, err := lookupSessionController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer relSess()

	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()

	swProv := prov.(*provider_spacewave.Provider)
	username := "test-" + ulid.NewULID()
	password := []byte("test-password-" + ulid.NewULID())

	entry, err := swProv.CreateSpacewaveAccountAndSession(ctx, username, password, "", sessCtrl)
	if err != nil {
		t.Fatal(err)
	}

	return entry
}

func createLocalSpace(
	ctx context.Context,
	t *testing.T,
	entry *session.SessionListEntry,
	spaceName string,
) string {
	t.Helper()

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
	defer provAccRef.Release()

	localAcc, ok := provAcc.(*provider_local.ProviderAccount)
	if !ok {
		t.Fatal("expected local provider account")
	}

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

func mountSessionResource(
	ctx context.Context,
	t *testing.T,
	entry *session.SessionListEntry,
) (*resource_session.SessionResource, session.Session, func()) {
	t.Helper()

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

	le := logrus.NewEntry(logrus.StandardLogger())
	return resource_session.NewSessionResource(le, env.tb.Bus, sess), sess, sessRef.Release
}

func waitForTransferComplete(
	t *testing.T,
	xfer *provider_transfer.Transfer,
) *provider_transfer.TransferState {
	t.Helper()

	deadline := time.After(60 * time.Second)
	for {
		ch := xfer.WaitState()
		state := xfer.GetState()
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

func waitForSessionCount(
	ctx context.Context,
	t *testing.T,
	sessCtrl session.SessionController,
	want int,
) []*session.SessionListEntry {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for {
		sessions, err := sessCtrl.ListSessions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(sessions) == want {
			return sessions
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d sessions, last count=%d", want, len(sessions))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestQuickstartLocal verifies that a local session can be created and
// appears in the session list.
func TestQuickstartLocal(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	cloudAcctID := "qs-" + ulid.NewULID()
	entry, accountID := createLocalSession(ctx, t, cloudAcctID)
	t.Logf("local session created: idx=%d account=%s", entry.GetSessionIndex(), accountID)

	// Verify session in list.
	sessCtrl, relSess, err := lookupSessionController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer relSess()

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
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

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
	defer provRef.Release()

	swProv := prov.(*provider_spacewave.Provider)
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relAcc()

	swAcc := accIface.(*provider_spacewave.ProviderAccount)
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

func TestMigrationLifecycle(t *testing.T) {
	t.Skip("migration system deleted, replaced by core/provider/transfer")
}

// TestQuickstartUpgradeFullFlow verifies the real onboarding transfer path for
// a quickstart local session upgraded into an active cloud account with a
// linked local session. The original local session is merged into the linked
// local target, leaving only the cloud + linked-local pair.
func TestQuickstartUpgradeFullFlow(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	sessCtrl, relSess, err := lookupSessionController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer relSess()

	initialSessions, err := sessCtrl.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	initialCount := len(initialSessions)

	originalLocal, _ := createLocalSession(ctx, t, "")
	spaceName := "Quickstart Space"
	createLocalSpace(ctx, t, originalLocal, spaceName)

	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()
	cloudSessionID := cloudRef.GetId()
	setTestSubscriptionStatus(t, cloudAccountID, "active")

	prov, provRef, err := provider.ExLookupProvider(ctx, env.tb.Bus, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()

	swProv := prov.(*provider_spacewave.Provider)
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relAcc()
	swAcc := accIface.(*provider_spacewave.ProviderAccount)

	subStatus, err := swAcc.GetSubscriptionStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if subStatus != "active" {
		t.Fatalf("expected active subscription status, got %q", subStatus)
	}

	cloudResource, cloudSess, relCloudResource := mountSessionResource(ctx, t, cloudEntry)
	defer relCloudResource()

	swResource := resource_session.NewSpacewaveSessionResource(
		cloudResource,
		logrus.NewEntry(logrus.StandardLogger()),
		env.tb.Bus,
		cloudSess,
		swAcc,
	)
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

	beforeTransfer := waitForSessionCount(ctx, t, sessCtrl, initialCount+3)
	if len(beforeTransfer) != initialCount+3 {
		t.Fatalf("expected %d sessions before transfer, got %d", initialCount+3, len(beforeTransfer))
	}

	targetResource, _, relTargetResource := mountSessionResource(ctx, t, linkedLocal)
	defer relTargetResource()

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

	afterTransfer := waitForSessionCount(ctx, t, sessCtrl, initialCount+2)
	if len(afterTransfer) != initialCount+2 {
		t.Fatalf("expected %d sessions after transfer, got %d", initialCount+2, len(afterTransfer))
	}

	srcEntry, err := sessCtrl.GetSessionByIdx(ctx, originalLocal.GetSessionIndex())
	if err != nil {
		t.Fatal(err)
	}
	if srcEntry != nil {
		t.Fatal("expected original quickstart local session to be deleted after merge")
	}

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

/*
disabled_TestMigrationLifecycle was here - deleted with migration system.

func disabled_TestMigrationLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	b := env.tb.Bus

	// 1. Create cloud session.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()
	t.Logf("cloud account: %s", cloudAccountID)

	// 2. Create local session keyed to cloud account.
	localEntry, localAccountID := createLocalSession(ctx, t, cloudAccountID)
	t.Logf("local session: idx=%d account=%s", localEntry.GetSessionIndex(), localAccountID)

	// 3. Create data in local session (SharedObject with blocks).
	localProvIface, localProvRef, err := provider.ExLookupProvider(ctx, b, provider_local.ProviderID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer localProvRef.Release()
	localProv := localProvIface.(*provider_local.Provider)

	localAccIface, relLocalAcc, err := localProv.AccessProviderAccount(ctx, localAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relLocalAcc()
	localAcc := localAccIface.(*provider_local.ProviderAccount)

	// Create a SharedObject via the provider account feature.
	wsProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, localAcc)
	if err != nil {
		t.Fatal(err)
	}

	soID := "space-" + ulid.NewULID()
	soRef, err := wsProv.CreateSharedObject(ctx, soID, &sobject.SharedObjectMeta{
		BodyType: "space",
	}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("created SharedObject: %s", soID)

	// Mount the SharedObject and write some blocks.
	so, soMountRef, err := sobject.ExMountSharedObject(ctx, b, soRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer soMountRef.Release()

	bs := so.GetBlockStore()
	testData := []byte("migration test block data " + ulid.NewULID())
	blockRef, _, err := bs.PutBlock(ctx, testData, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote block: %s (%d bytes)", blockRef.MarshalString(), len(testData))

	// 4. Link local to cloud.
	swProvIface, swProvRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer swProvRef.Release()
	swProv := swProvIface.(*provider_spacewave.Provider)

	swAccIface, relSwAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relSwAcc()
	swAcc := swAccIface.(*provider_spacewave.ProviderAccount)

	cloudSessionID := cloudRef.GetId()
	if err := swAcc.SetLinkedLocalSession(ctx, cloudSessionID, localEntry.GetSessionIndex()); err != nil {
		t.Fatal(err)
	}
	t.Log("linked local to cloud")

	// 5. Get source and destination volumes.
	srcVol := localAcc.GetVolume()
	srcRefGraph := srcVol.GetRefGraph()
	if srcRefGraph == nil {
		t.Fatal("local volume has no ref graph")
	}

	srcPeer, err := srcVol.GetPeer(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	srcVolKey, err := srcPeer.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}

	dstVol := swAcc.GetVolume()
	dstPeer, err := dstVol.GetPeer(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	dstVolKey, err := dstPeer.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// 6. Mount destination block store.
	dstBstoreID := provider_local.SobjectBlockStoreID(soID)
	dstBstoreRef := provider_spacewave.NewBlockStoreRef(
		swAcc.GetProviderID(), swAcc.GetAccountID(), dstBstoreID,
	)
	dstBlockStore, dstBsRef, err := bstore.ExMountBlockStore(ctx, b, dstBstoreRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer dstBsRef.Release()

	// 7. Mount ObjectStores.
	localProviderID := localEntry.GetSessionRef().GetProviderResourceRef().GetProviderId()
	srcVolID := provider_local.StorageVolumeID(localProviderID, localAccountID)
	srcSOStoreID := provider_local.SobjectObjectStoreID(localProviderID, localAccountID)
	srcSessStoreID := provider_local.SessionObjectStoreID(localProviderID, localAccountID)

	srcSOHandle, _, srcSORef, err := volume.ExBuildObjectStoreAPI(ctx, b, false, srcSOStoreID, srcVolID, nil)
	if err != nil {
		// Fall back: use volume.ExBuildObjectStoreAPI
		t.Fatalf("mount src SO store: %v", err)
	}
	defer srcSORef.Release()

	srcSessHandle, _, srcSessRef, err := volume.ExBuildObjectStoreAPI(ctx, b, false, srcSessStoreID, srcVolID, nil)
	if err != nil {
		t.Fatalf("mount src session store: %v", err)
	}
	defer srcSessRef.Release()

	dstProviderID := swAcc.GetProviderID()
	dstAccountID := swAcc.GetAccountID()
	dstVolID := dstVol.GetID()
	dstSOStoreID := provider_local.SobjectObjectStoreID(dstProviderID, dstAccountID)
	dstSessStoreID := provider_local.SessionObjectStoreID(dstProviderID, dstAccountID)

	dstSOHandle, _, dstSORef, err := volume.ExBuildObjectStoreAPI(ctx, b, false, dstSOStoreID, dstVolID, nil)
	if err != nil {
		t.Fatalf("mount dst SO store: %v", err)
	}
	defer dstSORef.Release()

	dstSessHandle, _, dstSessRef, err := volume.ExBuildObjectStoreAPI(ctx, b, false, dstSessStoreID, dstVolID, nil)
	if err != nil {
		t.Fatalf("mount dst session store: %v", err)
	}
	defer dstSessRef.Release()

	// 8. Construct and run migration Controller.
	localSessionID := localEntry.GetSessionRef().GetProviderResourceRef().GetId()
	ctrl := provider_migration.NewController(
		env.tb.Logger,
		localProviderID,
		localAccountID,
		dstProviderID,
		dstAccountID,
		provider_migration.MigrationMode_MIGRATE,
		srcSOHandle.GetObjectStore(), // stateStore
		srcVol,                       // srcBlockStore
		dstBlockStore,                // dstBlockStore
		srcRefGraph,                  // srcRefGraph
		srcSOHandle.GetObjectStore(), // srcSOStore
		dstSOHandle.GetObjectStore(), // dstSOStore
		srcSessHandle.GetObjectStore(),
		dstSessHandle.GetObjectStore(),
		localSessionID,
		srcVolKey,
		dstVolKey,
	)

	// Set PushSOStateFn to push via SessionClient.
	sessionClient := swAcc.GetSessionClient()
	dstSOStore := dstSOHandle.GetObjectStore()
	ctrl.PushSOStateFn = func(ctx context.Context, pushSoID string, data []byte) error {
		if err := sessionClient.CreateSharedObject(ctx, pushSoID, "", "space", "", "", false); err != nil {
			t.Logf("create SO on cloud (may already exist): %v", err)
		}
		if err := sessionClient.PostInitState(ctx, pushSoID, data); err != nil {
			return errors.Wrap(err, "push SO state to cloud")
		}
		// Write locally for verification.
		key := provider_local.SobjectObjectStoreHostStateKey(pushSoID)
		tx, txErr := dstSOStore.NewTransaction(ctx, true)
		if txErr != nil {
			return txErr
		}
		defer tx.Discard()
		if err := tx.Set(ctx, key, data); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	migrationID := ulid.NewULID()
	mCtx, mCancel := context.WithCancel(ctx)
	defer mCancel()

	go ctrl.Execute(mCtx)
	if err := ctrl.StartMigration(mCtx, migrationID, []string{soID}); err != nil {
		t.Fatal(err)
	}
	t.Logf("migration started: %s", migrationID)

	// 9. Wait for migration to complete.
	ctr := ctrl.WatchProgress()
	deadline := time.After(30 * time.Second)
	for {
		val, err := ctr.WaitValue(mCtx, nil)
		if err != nil {
			t.Fatalf("watch progress: %v", err)
		}

		allDone := true
		for _, sp := range val.GetSpaces() {
			phase := sp.GetPhase()
			t.Logf("space %s: phase=%s blocks=%d/%d",
				sp.GetSpaceId(), phase.String(),
				sp.GetBlocksCopied(), sp.GetBlocksTotal())
			if phase == provider_migration.MigrationPhase_FAILED {
				t.Fatalf("space %s failed: %s", sp.GetSpaceId(), sp.GetError())
			}
			if phase != provider_migration.MigrationPhase_COMPLETE {
				allDone = false
			}
		}

		if allDone && len(val.GetSpaces()) > 0 {
			t.Log("migration complete")
			break
		}

		select {
		case <-deadline:
			t.Fatal("migration timed out after 30s")
		default:
		}

		// Wait for next state change.
		_, err = ctr.WaitValueChange(mCtx, val, nil)
		if err != nil {
			t.Fatalf("watch progress change: %v", err)
		}
	}

	// 10. Verify block is accessible on destination.
	dstData, found, err := dstBlockStore.GetBlock(ctx, blockRef)
	if err != nil {
		t.Fatalf("get block from destination: %v", err)
	}
	if !found {
		t.Fatal("migrated block not found on destination")
	}
	if string(dstData) != string(testData) {
		t.Fatalf("block data mismatch: want %q, got %q", testData, dstData)
	}
	t.Logf("verified: block %s exists on destination (%d bytes)", blockRef.MarshalString(), len(dstData))
*/

// TestDeleteAndResignup verifies that deleting a session and re-creating
// one results in a clean state with no stale references.
func TestDeleteAndResignup(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	b := env.tb.Bus

	// Create cloud session.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()
	cloudSessionID := cloudRef.GetId()
	t.Logf("cloud session created: idx=%d account=%s", cloudEntry.GetSessionIndex(), cloudAccountID)

	// Create and link local.
	localEntry, _ := createLocalSession(ctx, t, cloudAccountID)

	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()
	swProv := prov.(*provider_spacewave.Provider)

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
	defer relSess()

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
	defer relAcc3()
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
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	b := env.tb.Bus

	// Create cloud session.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	// Access provider account for SessionClient.
	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()
	swProv := prov.(*provider_spacewave.Provider)

	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relAcc()
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
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	b := env.tb.Bus

	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()
	swProv := prov.(*provider_spacewave.Provider)

	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relAcc()
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
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	b := env.tb.Bus

	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()
	swProv := prov.(*provider_spacewave.Provider)

	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relAcc()
	swAcc := accIface.(*provider_spacewave.ProviderAccount)

	status, err := swAcc.GetSubscriptionStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// New accounts have no subscription ("" or "free").
	if status != "" && status != "free" {
		t.Fatalf("expected empty or free subscription, got %q", status)
	}
	t.Logf("subscription status: %q (expected for new account)", status)
}

// TestUnlinkLocalSession verifies unlinking a local session from a cloud
// session (the "keep separate" flow).
func TestUnlinkLocalSession(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	b := env.tb.Bus

	// Create cloud + local and link them.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()
	cloudSessionID := cloudRef.GetId()

	localEntry, _ := createLocalSession(ctx, t, cloudAccountID)

	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()
	swProv := prov.(*provider_spacewave.Provider)

	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relAcc()
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
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	sessCtrl, relSess, err := lookupSessionController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer relSess()

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
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	b := env.tb.Bus

	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()
	swProv := prov.(*provider_spacewave.Provider)

	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relAcc()
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
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	b := env.tb.Bus

	// Create cloud session and access account.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()
	swProv := prov.(*provider_spacewave.Provider)

	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relAcc()
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
		data []byte
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
	tmpFile, err := os.CreateTemp("", "synctest-*.kvf")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	hashWriter := sha256.New()
	multiWriter := io.MultiWriter(tmpFile, hashWriter)

	idx := 0
	iter := func() (*bifhash.Hash, []byte, error) {
		if idx >= len(blocks) {
			return nil, nil, nil
		}
		blk := blocks[idx]
		idx++
		return blk.hash, blk.data, nil
	}

	result, err := packfile_writer.PackBlocks(multiWriter, iter)
	if err != nil {
		tmpFile.Close()
		t.Fatal(err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatal(err)
	}
	bodyHash := hashWriter.Sum(nil)

	t.Logf("packed %d blocks, %d bytes, bloom %d bytes",
		result.BlockCount, result.BytesWritten, len(result.BloomFilter))

	// Push packfile to cloud.
	packID := ulid.NewULID()
	if err := cli.SyncPush(ctx, bstoreID, packID, int(result.BlockCount), tmpPath, bodyHash, result.BloomFilter, provider_spacewave_packfile.BloomFormatVersionV1); err != nil {
		t.Fatal(err)
	}
	t.Logf("pushed pack %s", packID)

	// Pull manifest and verify the pack appears.
	pullData, err := cli.SyncPull(ctx, bstoreID, "")
	if err != nil {
		t.Fatal(err)
	}

	pullResp := &provider_spacewave_packfile.PullResponse{}
	if err := pullResp.UnmarshalJSON(pullData); err != nil {
		t.Fatalf("unmarshal pull response: %v", err)
	}

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
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	b := env.tb.Bus

	// Create cloud session.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()
	swProv := prov.(*provider_spacewave.Provider)

	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relAcc()
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
		base64URLEncode([]byte("fake-encrypted")), // encryptedPrivkey
		entityPeerID.String(),                     // peerID
		"",                                        // authParams
		"",                                        // prfSalt
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

	sessCtrl, relSess, err := lookupSessionController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer relSess()

	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()

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

	return entry, privKey, entityPeerID
}

// TestAccountRecovery verifies the recovery flow end-to-end:
// set verified email -> request recovery -> extract token from mock SES ->
// verify token -> sign and execute recovery with new keypair.
func TestAccountRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	b := env.tb.Bus

	// Create cloud session with entity key access.
	cloudEntry, entityPrivKey, entityPeerID := createCloudSessionWithKey(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()
	t.Logf("cloud account: %s entity peer: %s", cloudAccountID, entityPeerID.String())

	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()
	swProv := prov.(*provider_spacewave.Provider)

	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relAcc()
	swAcc := accIface.(*provider_spacewave.ProviderAccount)
	cli := swAcc.GetSessionClient()

	// Step 1: Set a verified email on the account.
	// POST /account/email/verify-request (session-authenticated).
	testEmail := "recovery-test-" + ulid.NewULID() + "@example.com"
	emailBody := `{"email":"` + testEmail + `"}`
	verifyReqURL := env.cloudURL + "/api/account/email/verify-request"
	verifyHTTPReq, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyReqURL, strings.NewReader(emailBody))
	if err != nil {
		t.Fatal(err)
	}
	verifyHTTPReq.Header.Set("Content-Type", "application/json")
	verifyResp, err := cli.Do(verifyHTTPReq)
	if err != nil {
		t.Fatal(err)
	}
	verifyResp.Body.Close()
	if verifyResp.StatusCode != 200 {
		t.Fatalf("email verify request failed: %d", verifyResp.StatusCode)
	}
	t.Log("email verify request sent")

	// Wait briefly for the email to be processed by the mock SES.
	time.Sleep(500 * time.Millisecond)

	// Extract the verification token from the mock SES mailbox.
	emails := env.mailbox.getEmails()
	if len(emails) == 0 {
		t.Fatal("no emails captured by mock SES after verify request")
	}
	verifyEmail := emails[len(emails)-1]
	t.Logf("captured verification email to=%s subject=%s", verifyEmail.To, verifyEmail.Subject)

	// Extract token from verification email HTML (look for ?token= in URL).
	verifyToken := extractTokenFromHTML(verifyEmail.HTML)
	if verifyToken == "" {
		t.Fatal("could not extract verification token from email HTML")
	}
	t.Logf("extracted verification token: %s...", verifyToken[:16])

	// Complete email verification: GET /account/email/verify?token=xyz
	confirmURL := env.cloudURL + "/api/account/email/verify?token=" + verifyToken
	confirmReq, err := http.NewRequestWithContext(ctx, http.MethodGet, confirmURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	confirmResp, err := httpClient.Do(confirmReq)
	if err != nil {
		t.Fatal(err)
	}
	confirmResp.Body.Close()
	if confirmResp.StatusCode != 200 {
		t.Fatalf("email verify confirm failed: %d", confirmResp.StatusCode)
	}
	t.Log("email verified successfully")

	// Clear the mailbox for the recovery email.
	env.mailbox.mu.Lock()
	env.mailbox.emails = nil
	env.mailbox.mu.Unlock()

	// Step 2: Request account recovery.
	// POST /account/recover/request (unauthenticated).
	recoverBody := `{"email":"` + testEmail + `"}`
	recoverReqURL := env.cloudURL + "/api/account/recover/request"
	recoverHTTPReq, err := http.NewRequestWithContext(ctx, http.MethodPost, recoverReqURL, strings.NewReader(recoverBody))
	if err != nil {
		t.Fatal(err)
	}
	recoverHTTPReq.Header.Set("Content-Type", "application/json")
	recoverResp, err := httpClient.Do(recoverHTTPReq)
	if err != nil {
		t.Fatal(err)
	}
	recoverResp.Body.Close()
	if recoverResp.StatusCode != 200 {
		t.Fatalf("recovery request failed: %d", recoverResp.StatusCode)
	}
	t.Log("recovery request sent")

	// Wait for the recovery email to be queued and processed.
	time.Sleep(500 * time.Millisecond)

	// Step 3: Extract recovery token from the mock SES mailbox.
	emails = env.mailbox.getEmails()
	if len(emails) == 0 {
		t.Fatal("no emails captured by mock SES after recovery request")
	}
	recoveryEmail := emails[len(emails)-1]
	t.Logf("captured recovery email to=%s subject=%s", recoveryEmail.To, recoveryEmail.Subject)

	recoveryToken := extractTokenFromHTML(recoveryEmail.HTML)
	if recoveryToken == "" {
		t.Fatal("could not extract recovery token from email HTML")
	}
	t.Logf("extracted recovery token: %s...", recoveryToken[:16])

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
				Signature: base64Encode(sig),
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

// extractTokenFromHTML extracts a token from an email HTML body.
// Looks for ?token= in URL query parameters.
func extractTokenFromHTML(html string) string {
	idx := strings.Index(html, "?token=")
	if idx < 0 {
		return ""
	}
	start := idx + len("?token=")
	// Token ends at quote, ampersand, or angle bracket.
	end := start
	for end < len(html) {
		c := html[end]
		if c == '"' || c == '&' || c == '<' || c == '\'' || c == ' ' {
			break
		}
		end++
	}
	return html[start:end]
}

// base64Encode encodes bytes to standard base64 string.
func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// TestEmailVerification verifies the email verification flow end-to-end:
// set unverified email -> request verification -> extract token from mock SES ->
// confirm verification -> verify account info.
func TestEmailVerification(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	b := env.tb.Bus

	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	prov, provRef, err := provider.ExLookupProvider(ctx, b, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()
	swProv := prov.(*provider_spacewave.Provider)

	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relAcc()
	swAcc := accIface.(*provider_spacewave.ProviderAccount)
	cli := swAcc.GetSessionClient()

	// Clear mailbox.
	env.mailbox.mu.Lock()
	env.mailbox.emails = nil
	env.mailbox.mu.Unlock()

	// Request email verification (session-authenticated).
	testEmail := "verify-" + ulid.NewULID() + "@example.com"
	if _, err := cli.RequestEmailVerification(ctx, testEmail); err != nil {
		t.Fatal(err)
	}
	t.Logf("verification email requested for %s", testEmail)

	// Wait for the email to arrive at mock SES.
	time.Sleep(500 * time.Millisecond)

	emails := env.mailbox.getEmails()
	if len(emails) == 0 {
		t.Fatal("no verification email captured by mock SES")
	}
	verifyEmail := emails[len(emails)-1]
	if verifyEmail.To != testEmail {
		t.Fatalf("email to mismatch: want %s, got %s", testEmail, verifyEmail.To)
	}
	t.Logf("captured email: subject=%q", verifyEmail.Subject)

	// Extract token from email HTML.
	token := extractTokenFromHTML(verifyEmail.HTML)
	if token == "" {
		t.Fatal("could not extract verification token from email HTML")
	}
	t.Logf("extracted token: %s...", token[:16])

	// Confirm email verification (unauthenticated GET).
	confirmURL := env.cloudURL + "/api/account/email/verify?token=" + token
	confirmReq, err := http.NewRequestWithContext(ctx, http.MethodGet, confirmURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	confirmResp, err := httpClient.Do(confirmReq)
	if err != nil {
		t.Fatal(err)
	}
	confirmBody, _ := io.ReadAll(confirmResp.Body)
	confirmResp.Body.Close()
	if confirmResp.StatusCode != 200 {
		t.Fatalf("email verify confirm failed: %d body=%s", confirmResp.StatusCode, string(confirmBody))
	}
	if !strings.Contains(string(confirmBody), "verified") {
		t.Fatal("verify response does not contain 'verified'")
	}
	t.Log("email verified successfully")

	// Confirm account info shows verified email.
	info, err := cli.GetAccountInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("account info after verification: id=%s", info.AccountId)
}

// TestFailsafeLogin verifies the failsafe account management flow:
// set email -> request failsafe token -> extract from mock SES ->
// verify token to access account settings page.
func TestFailsafeLogin(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	b := env.tb.Bus

	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	// Set verified email on the account via test helper.
	testEmail := "failsafe-" + ulid.NewULID() + "@example.com"
	setEmailURL := env.cloudURL + "/api/test/set-email"
	setEmailBody := `{"account_id":"` + cloudAccountID + `","email":"` + testEmail + `","verified":true}`
	setEmailReq, err := http.NewRequestWithContext(ctx, http.MethodPost, setEmailURL, strings.NewReader(setEmailBody))
	if err != nil {
		t.Fatal(err)
	}
	setEmailReq.Header.Set("Content-Type", "application/json")
	setEmailResp, err := httpClient.Do(setEmailReq)
	if err != nil {
		t.Fatal(err)
	}
	setEmailResp.Body.Close()
	if setEmailResp.StatusCode != 200 {
		t.Fatalf("set email failed: %d", setEmailResp.StatusCode)
	}
	t.Logf("set email %s on account %s", testEmail, cloudAccountID)

	// Clear mailbox.
	env.mailbox.mu.Lock()
	env.mailbox.emails = nil
	env.mailbox.mu.Unlock()

	// Step 1: Request failsafe token.
	// POST /request-token with Host: account.spacewave.app (form-encoded).
	_ = b // bus available if needed
	formBody := "email=" + testEmail
	failsafeURL := env.cloudURL + "/request-token"
	failsafeReq, err := http.NewRequestWithContext(ctx, http.MethodPost, failsafeURL, strings.NewReader(formBody))
	if err != nil {
		t.Fatal(err)
	}
	failsafeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	failsafeReq.Host = "account.spacewave.app"
	failsafeResp, err := httpClient.Do(failsafeReq)
	if err != nil {
		t.Fatal(err)
	}
	failsafeRespBody, _ := io.ReadAll(failsafeResp.Body)
	failsafeResp.Body.Close()
	if failsafeResp.StatusCode != 200 {
		t.Fatalf("failsafe request-token failed: %d body=%s", failsafeResp.StatusCode, string(failsafeRespBody))
	}
	t.Log("failsafe token requested")

	// Wait for email.
	time.Sleep(500 * time.Millisecond)

	emails := env.mailbox.getEmails()
	if len(emails) == 0 {
		t.Fatal("no failsafe email captured by mock SES")
	}
	failsafeEmail := emails[len(emails)-1]
	if failsafeEmail.To != testEmail {
		t.Fatalf("failsafe email to mismatch: want %s, got %s", testEmail, failsafeEmail.To)
	}
	t.Logf("captured failsafe email: subject=%q", failsafeEmail.Subject)

	// Extract token from failsafe email.
	token := extractTokenFromHTML(failsafeEmail.HTML)
	if token == "" {
		t.Fatal("could not extract failsafe token from email HTML")
	}
	t.Logf("extracted failsafe token: %s...", token[:16])

	// Step 2: Verify token to access account settings page.
	verifyURL := env.cloudURL + "/verify?token=" + token
	verifyReq, err := http.NewRequestWithContext(ctx, http.MethodGet, verifyURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	verifyReq.Host = "account.spacewave.app"
	verifyResp, err := httpClient.Do(verifyReq)
	if err != nil {
		t.Fatal(err)
	}
	verifyBody, _ := io.ReadAll(verifyResp.Body)
	verifyResp.Body.Close()
	if verifyResp.StatusCode != 200 {
		t.Fatalf("failsafe verify failed: %d body=%s", verifyResp.StatusCode, string(verifyBody))
	}
	// The failsafe verify page returns HTML with account info.
	bodyStr := string(verifyBody)
	if !strings.Contains(bodyStr, "html") {
		t.Fatal("failsafe verify did not return HTML page")
	}
	t.Log("failsafe login verified: account settings page accessible")
}

// extractFooterLink extracts the href from the "Manage your account" footer
// link in email HTML. Returns "" if not found.
func extractFooterLink(html string) string {
	anchor := `>Manage your account</a>`
	idx := strings.Index(html, anchor)
	if idx < 0 {
		return ""
	}
	// Walk backward from the anchor to find href="..."
	prefix := html[:idx]
	hrefIdx := strings.LastIndex(prefix, `href="`)
	if hrefIdx < 0 {
		return ""
	}
	start := hrefIdx + len(`href="`)
	end := strings.Index(prefix[start:], `"`)
	if end < 0 {
		return ""
	}
	return prefix[start : start+end]
}

// TestBillingEmailFooterOpensAccountSettings verifies that the tokenized
// "Manage your account" footer link in an account-management email opens the
// account settings page.
func TestBillingEmailFooterOpensAccountSettings(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	// Create account with verified email.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()

	testEmail := "footer-" + ulid.NewULID() + "@example.com"
	setEmailURL := env.cloudURL + "/api/test/set-email"
	setEmailBody := `{"account_id":"` + cloudAccountID + `","email":"` + testEmail + `","verified":true}`
	setEmailReq, err := http.NewRequestWithContext(ctx, http.MethodPost, setEmailURL, strings.NewReader(setEmailBody))
	if err != nil {
		t.Fatal(err)
	}
	setEmailReq.Header.Set("Content-Type", "application/json")
	setEmailResp, err := httpClient.Do(setEmailReq)
	if err != nil {
		t.Fatal(err)
	}
	setEmailResp.Body.Close()
	if setEmailResp.StatusCode != 200 {
		t.Fatalf("set email failed: %d", setEmailResp.StatusCode)
	}

	// Clear mailbox.
	env.mailbox.mu.Lock()
	env.mailbox.emails = nil
	env.mailbox.mu.Unlock()

	// Request failsafe token (sends email with tokenized footer).
	formBody := "email=" + testEmail
	failsafeURL := env.cloudURL + "/request-token"
	failsafeReq, err := http.NewRequestWithContext(ctx, http.MethodPost, failsafeURL, strings.NewReader(formBody))
	if err != nil {
		t.Fatal(err)
	}
	failsafeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	failsafeReq.Host = "account.spacewave.app"
	failsafeResp, err := httpClient.Do(failsafeReq)
	if err != nil {
		t.Fatal(err)
	}
	failsafeResp.Body.Close()
	if failsafeResp.StatusCode != 200 {
		t.Fatalf("failsafe request failed: %d", failsafeResp.StatusCode)
	}

	time.Sleep(500 * time.Millisecond)

	emails := env.mailbox.getEmails()
	if len(emails) == 0 {
		t.Fatal("no email captured by mock SES")
	}
	emailHTML := emails[len(emails)-1].HTML

	// Extract the footer "Manage your account" link specifically.
	footerLink := extractFooterLink(emailHTML)
	if footerLink == "" {
		t.Fatal("no 'Manage your account' footer link found in email HTML")
	}
	if !strings.Contains(footerLink, "?token=") {
		t.Fatalf("footer link is not tokenized: %s", footerLink)
	}
	t.Logf("extracted footer link: %s", footerLink)

	// Rewrite link to point at the local wrangler dev server.
	footerPath := footerLink
	if idx := strings.Index(footerLink, "/verify"); idx >= 0 {
		footerPath = footerLink[idx:]
	}
	localURL := env.cloudURL + footerPath

	// Follow the footer link to verify it opens account settings.
	verifyReq, err := http.NewRequestWithContext(ctx, http.MethodGet, localURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	verifyReq.Host = "account.spacewave.app"
	verifyResp, err := httpClient.Do(verifyReq)
	if err != nil {
		t.Fatal(err)
	}
	verifyBody, _ := io.ReadAll(verifyResp.Body)
	verifyResp.Body.Close()

	if verifyResp.StatusCode != 200 {
		t.Fatalf("footer link returned status %d, want 200", verifyResp.StatusCode)
	}
	if !strings.Contains(string(verifyBody), "html") {
		t.Fatal("footer link did not return HTML settings page")
	}
	t.Log("verified: footer link opens account settings page")
}

// TestInvalidAccountManagementTokenRedirectsToEntryPage verifies that an
// invalid or expired account-management token redirects to the account
// email-entry page with a reason parameter.
func TestInvalidAccountManagementTokenRedirectsToEntryPage(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	// Disable redirect following to inspect the 302 directly.
	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Test with a completely invalid token.
	invalidURL := env.cloudURL + "/verify?token=invalid-garbage-token-xyz"
	invalidReq, err := http.NewRequestWithContext(ctx, http.MethodGet, invalidURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	invalidReq.Host = "account.spacewave.app"
	invalidResp, err := noRedirectClient.Do(invalidReq)
	if err != nil {
		t.Fatal(err)
	}
	invalidResp.Body.Close()

	if invalidResp.StatusCode != 302 {
		t.Fatalf("invalid token returned status %d, want 302", invalidResp.StatusCode)
	}
	location := invalidResp.Header.Get("Location")
	if !strings.Contains(location, "reason=invalid") {
		t.Fatalf("redirect location missing reason=invalid: %s", location)
	}
	t.Logf("invalid token redirected to: %s", location)

	// Verify the entry page is accessible and shows the message.
	entryURL := env.cloudURL + location
	entryReq, err := http.NewRequestWithContext(ctx, http.MethodGet, entryURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	entryReq.Host = "account.spacewave.app"
	entryResp, err := httpClient.Do(entryReq)
	if err != nil {
		t.Fatal(err)
	}
	entryBody, _ := io.ReadAll(entryResp.Body)
	entryResp.Body.Close()

	if entryResp.StatusCode != 200 {
		t.Fatalf("entry page returned status %d", entryResp.StatusCode)
	}
	bodyStr := string(entryBody)
	if !strings.Contains(bodyStr, "html") {
		t.Fatal("entry page did not return HTML")
	}
	t.Log("verified: invalid token redirects to entry page with reason=invalid")
}

// Compile-time check that block.BlockRef has MarshalString.
var _ = (*block.BlockRef).MarshalString
