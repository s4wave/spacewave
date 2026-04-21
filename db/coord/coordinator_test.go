//go:build !js

package coord

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/sirupsen/logrus"
)

// testRoleHandler records role changes for test assertions.
type testRoleHandler struct {
	leaderCh chan struct{}
}

func newTestRoleHandler() *testRoleHandler {
	return &testRoleHandler{
		leaderCh: make(chan struct{}, 1),
	}
}

func (h *testRoleHandler) OnBecomeLeader(ctx context.Context) error {
	select {
	case h.leaderCh <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil
}

func (h *testRoleHandler) OnBecomeFollower(ctx context.Context, leaderSocketPath string) error {
	<-ctx.Done()
	return nil
}

// _ is a type assertion.
var _ RoleChangeHandler = (*testRoleHandler)(nil)

// shortTempDir creates a short temp directory suitable for Unix sockets
// (macOS has a 104-byte path limit for sun_path).
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "coord-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestCoordinatorLifecycle(t *testing.T) {
	dir := shortTempDir(t)
	dbPath := filepath.Join(dir, "test.db")
	db, err := bdb.Open(dbPath, 0o600, &bdb.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	le := logrus.NewEntry(logrus.New())
	le.Logger.SetLevel(logrus.DebugLevel)

	handler := newTestRoleHandler()
	coordinator := NewCoordinator(le, db, dir, []string{"test"}, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Run coordinator in background.
	coordDone := make(chan error, 1)
	go func() {
		coordDone <- coordinator.Run(ctx)
	}()

	// Wait for role to be determined.
	role, err := coordinator.WaitRole(ctx)
	if err != nil {
		t.Fatal("WaitRole:", err)
	}
	if role != ParticipantRole_ParticipantRole_LEADER {
		t.Fatalf("expected leader role, got %v", role)
	}

	// Wait for handler to confirm leadership.
	select {
	case <-handler.leaderCh:
	case <-ctx.Done():
		t.Fatal("timed out waiting for OnBecomeLeader")
	}

	// Verify participant count.
	count, err := coordinator.CountParticipants()
	if err != nil {
		t.Fatal("CountParticipants:", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 participant, got %d", count)
	}

	// Verify lease exists with our PID.
	lease, err := coordinator.GetElection().CurrentLeader()
	if err != nil {
		t.Fatal("CurrentLeader:", err)
	}
	if lease == nil {
		t.Fatal("expected lease, got nil")
	}
	if lease.GetLeaderPid() != uint32(os.Getpid()) {
		t.Fatalf("expected leader PID %d, got %d", os.Getpid(), lease.GetLeaderPid())
	}

	// Verify socket file exists.
	socketPath := coordinator.GetMesh().SocketPath()
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("socket file does not exist: %s", socketPath)
	}

	// Test SRPC: connect to ourselves via the mesh and call ParticipantInfo.
	client, err := coordinator.GetMesh().Connect(ctx, uint32(os.Getpid()), socketPath)
	if err != nil {
		t.Fatal("Connect to self:", err)
	}
	pClient := NewSRPCParticipantServiceClient(client)
	info, err := pClient.GetParticipantInfo(ctx, &GetParticipantInfoRequest{})
	if err != nil {
		t.Fatal("GetParticipantInfo:", err)
	}
	if info.GetPid() != uint32(os.Getpid()) {
		t.Fatalf("expected PID %d, got %d", os.Getpid(), info.GetPid())
	}
	if info.GetRole() != ParticipantRole_ParticipantRole_LEADER {
		t.Fatalf("expected leader role from SRPC, got %v", info.GetRole())
	}
	if len(info.GetCapabilities()) != 1 || info.GetCapabilities()[0] != "test" {
		t.Fatalf("expected capabilities [test], got %v", info.GetCapabilities())
	}

	// Shut down gracefully.
	cancel()

	select {
	case err := <-coordDone:
		if err != nil && err != context.Canceled {
			t.Fatal("coordinator run:", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("coordinator did not shut down in time")
	}

	// After shutdown: lease should be released.
	var postLease *LeaseRecord
	err = db.View(func(tx *bdb.Tx) error {
		var readErr error
		postLease, readErr = GetLease(tx)
		return readErr
	})
	if err != nil {
		t.Fatal("read post-shutdown lease:", err)
	}
	if postLease != nil {
		t.Fatal("lease should be nil after graceful shutdown")
	}

	// Participant record should be removed.
	var postRec *ParticipantRecord
	err = db.View(func(tx *bdb.Tx) error {
		var readErr error
		postRec, readErr = GetParticipant(tx, uint32(os.Getpid()))
		return readErr
	})
	if err != nil {
		t.Fatal("read post-shutdown participant:", err)
	}
	if postRec != nil {
		t.Fatal("participant record should be nil after graceful shutdown")
	}

	// Socket file should be cleaned up.
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Fatal("socket file should be removed after shutdown")
	}
}

func TestCoordinatorLeaseRenewal(t *testing.T) {
	dir := shortTempDir(t)
	dbPath := filepath.Join(dir, "test.db")
	db, err := bdb.Open(dbPath, 0o600, &bdb.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	le := logrus.NewEntry(logrus.New())
	le.Logger.SetLevel(logrus.DebugLevel)

	handler := newTestRoleHandler()
	coordinator := NewCoordinator(le, db, dir, []string{"test"}, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	coordDone := make(chan error, 1)
	go func() {
		coordDone <- coordinator.Run(ctx)
	}()

	// Wait for leadership or coordinator error.
	select {
	case <-handler.leaderCh:
	case err := <-coordDone:
		t.Fatal("coordinator exited early:", err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for leader")
	}

	// Read initial lease timestamp.
	lease1, err := coordinator.GetElection().CurrentLeader()
	if err != nil {
		t.Fatal(err)
	}
	ts1 := lease1.GetLeaseTimestampNanos()

	// Wait for at least one lease renewal cycle (250ms + margin).
	time.Sleep(400 * time.Millisecond)

	lease2, err := coordinator.GetElection().CurrentLeader()
	if err != nil {
		t.Fatal(err)
	}
	ts2 := lease2.GetLeaseTimestampNanos()

	if ts2 <= ts1 {
		t.Fatalf("lease timestamp not renewed: %d <= %d", ts2, ts1)
	}

	cancel()
	<-coordDone
}
