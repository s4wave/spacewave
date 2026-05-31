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

type blockingLeaderRoleHandler struct {
	leaderCh       chan struct{}
	leaderCancelCh chan struct{}
	releaseLeader  chan struct{}
	leaderDone     chan struct{}
}

func newBlockingLeaderRoleHandler() *blockingLeaderRoleHandler {
	return &blockingLeaderRoleHandler{
		leaderCh:       make(chan struct{}, 1),
		leaderCancelCh: make(chan struct{}, 1),
		releaseLeader:  make(chan struct{}),
		leaderDone:     make(chan struct{}),
	}
}

func (h *blockingLeaderRoleHandler) OnBecomeLeader(ctx context.Context) error {
	defer close(h.leaderDone)
	select {
	case h.leaderCh <- struct{}{}:
	default:
	}
	<-ctx.Done()
	select {
	case h.leaderCancelCh <- struct{}{}:
	default:
	}
	<-h.releaseLeader
	return nil
}

func (h *blockingLeaderRoleHandler) OnBecomeFollower(ctx context.Context, leaderSocketPath string) error {
	<-ctx.Done()
	return nil
}

// _ is a type assertion.
var _ RoleChangeHandler = (*blockingLeaderRoleHandler)(nil)

type blockingFollowerRoleHandler struct {
	leaderCh         chan struct{}
	followerCh       chan struct{}
	followerCancelCh chan struct{}
	releaseFollower  chan struct{}
	followerDone     chan struct{}
}

func newBlockingFollowerRoleHandler() *blockingFollowerRoleHandler {
	return &blockingFollowerRoleHandler{
		leaderCh:         make(chan struct{}, 1),
		followerCh:       make(chan struct{}, 1),
		followerCancelCh: make(chan struct{}, 1),
		releaseFollower:  make(chan struct{}),
		followerDone:     make(chan struct{}),
	}
}

func (h *blockingFollowerRoleHandler) OnBecomeLeader(ctx context.Context) error {
	select {
	case h.leaderCh <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil
}

func (h *blockingFollowerRoleHandler) OnBecomeFollower(ctx context.Context, leaderSocketPath string) error {
	defer close(h.followerDone)
	select {
	case h.followerCh <- struct{}{}:
	default:
	}
	<-ctx.Done()
	select {
	case h.followerCancelCh <- struct{}{}:
	default:
	}
	<-h.releaseFollower
	return nil
}

// _ is a type assertion.
var _ RoleChangeHandler = (*blockingFollowerRoleHandler)(nil)

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

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
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
	pid := currentPID()
	if lease.GetLeaderPid() != pid {
		t.Fatalf("expected leader PID %d, got %d", os.Getpid(), lease.GetLeaderPid())
	}

	// Verify socket file exists.
	socketPath := coordinator.GetMesh().SocketPath()
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("socket file does not exist: %s", socketPath)
	}

	// Test SRPC: connect to ourselves via the mesh and call ParticipantInfo.
	client, err := coordinator.GetMesh().Connect(ctx, pid, socketPath)
	if err != nil {
		t.Fatal("Connect to self:", err)
	}
	pClient := NewSRPCParticipantServiceClient(client)
	info, err := pClient.GetParticipantInfo(ctx, &GetParticipantInfoRequest{})
	if err != nil {
		t.Fatal("GetParticipantInfo:", err)
	}
	if info.GetPid() != pid {
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
		postRec, readErr = GetParticipant(tx, pid)
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

func TestCoordinatorShutdownWaitsForLeaderHandlerBeforeLeaseRelease(t *testing.T) {
	dir := shortTempDir(t)
	dbPath := filepath.Join(dir, "test.db")
	db, err := bdb.Open(dbPath, 0o600, &bdb.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	le := logrus.NewEntry(logrus.New())
	le.Logger.SetLevel(logrus.DebugLevel)

	handler := newBlockingLeaderRoleHandler()
	coordinator := NewCoordinator(le, db, dir, []string{"test"}, handler)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	coordDone := make(chan error, 1)
	go func() {
		coordDone <- coordinator.Run(ctx)
	}()

	select {
	case <-handler.leaderCh:
	case err := <-coordDone:
		t.Fatal("coordinator exited early:", err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for leader")
	}

	cancel()
	select {
	case <-handler.leaderCancelCh:
	case <-time.After(5 * time.Second):
		t.Fatal("leader handler was not canceled")
	}

	select {
	case err := <-coordDone:
		t.Fatalf("coordinator returned before leader handler exited: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	lease, err := coordinator.GetElection().CurrentLeader()
	if err != nil {
		t.Fatal("CurrentLeader:", err)
	}
	if lease == nil {
		t.Fatal("lease released before leader handler exited")
	}

	close(handler.releaseLeader)
	select {
	case <-handler.leaderDone:
	case <-time.After(5 * time.Second):
		t.Fatal("leader handler did not exit")
	}
	select {
	case err := <-coordDone:
		if err != nil && err != context.Canceled {
			t.Fatal("coordinator run:", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("coordinator did not shut down")
	}

	lease, err = coordinator.GetElection().CurrentLeader()
	if err != nil {
		t.Fatal("CurrentLeader after shutdown:", err)
	}
	if lease != nil {
		t.Fatal("lease should be released after leader handler exits")
	}
}

func TestCoordinatorShutdownWaitsForFollowerHandler(t *testing.T) {
	dir := shortTempDir(t)
	dbPath := filepath.Join(dir, "test.db")
	db, err := bdb.Open(dbPath, 0o600, &bdb.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	err = db.Update(func(tx *bdb.Tx) error {
		return PutLease(tx, &LeaseRecord{
			LeaderPid:           currentPID(),
			LeaseTimestampNanos: time.Now().UnixNano(),
			LeaderSocketPath:    filepath.Join(dir, "leader.sock"),
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	le := logrus.NewEntry(logrus.New())
	le.Logger.SetLevel(logrus.DebugLevel)

	handler := newBlockingFollowerRoleHandler()
	coordinator := NewCoordinator(le, db, dir, []string{"test"}, handler)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	coordDone := make(chan error, 1)
	go func() {
		coordDone <- coordinator.Run(ctx)
	}()

	select {
	case <-handler.followerCh:
	case <-handler.leaderCh:
		t.Fatal("coordinator unexpectedly became leader")
	case err := <-coordDone:
		t.Fatal("coordinator exited early:", err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for follower")
	}

	cancel()
	select {
	case <-handler.followerCancelCh:
	case <-time.After(5 * time.Second):
		t.Fatal("follower handler was not canceled")
	}

	select {
	case err := <-coordDone:
		t.Fatalf("coordinator returned before follower handler exited: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(handler.releaseFollower)
	select {
	case <-handler.followerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("follower handler did not exit")
	}
	select {
	case err := <-coordDone:
		if err != nil && err != context.Canceled {
			t.Fatal("coordinator run:", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("coordinator did not shut down")
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

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
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
