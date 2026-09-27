//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	resource_root "github.com/s4wave/spacewave/core/resource/root"
)

// TestCapacityObserverUsesDaemonInvokerWithoutHoldingPublicClient verifies
// policy reload through Resource while the last socket client starts idle expiry.
func TestCapacityObserverUsesDaemonInvokerWithoutHoldingPublicClient(t *testing.T) {
	// Isolate the policy and Device record under this worktree's disposable state.
	tmpRoot := filepath.Join("..", "..", "..", ".tmp")
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	statePath, err := os.MkdirTemp(tmpRoot, "capacity-observer-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(statePath) })
	t.Setenv("SPACEWAVE_STATE_PATH", statePath)
	if err := writeDeviceSetupRecord(statePath, &deviceSetupRecord{
		SetupState:   deviceSetupStateImported,
		SessionIndex: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := device_policy.WriteFile(statePath, &device_policy.DevicePolicy{Revision: 1}); err != nil {
		t.Fatal(err)
	}
	store, err := device_policy.NewPolicyStore(statePath)
	if err != nil {
		t.Fatal(err)
	}

	// Route the observer's projection probe through the real Resource invoker.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	le := observeLogger()
	rootMux := srpc.NewMux()
	rootServer := resource_root.NewCoreRootServer(le, nil)
	defer rootServer.Close()
	if err := rootServer.Register(rootMux); err != nil {
		t.Fatal(err)
	}
	resourceSrv := resource_server.NewResourceServer(rootMux)
	resourceMux := srpc.NewMux()
	if err := resourceSrv.Register(resourceMux); err != nil {
		t.Fatal(err)
	}
	projections := make(chan error, 3)
	oldUpsert := deviceUpsertObject
	deviceUpsertObject = func(ctx context.Context, client *sdkClient, _ string, _ *deviceSetupRecord) (string, error) {
		watch, err := client.root.WatchWebListeners(ctx)
		if err != nil {
			projections <- err
			return "", err
		}
		defer watch.Close()
		_, err = watch.Recv()
		projections <- err
		return "", errors.New("projection remains pending in fixture")
	}
	defer func() { deviceUpsertObject = oldUpsert }()

	// Serve one real socket client so the listener owns its counted lifetime.
	listener, err := net.Listen("unix", filepath.Join(statePath, socketName))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	idleTimeout := 40 * time.Millisecond
	idleCh := make(chan time.Time, 1)
	tracker := newDaemonIdleTracker(idleTimeout, func() { idleCh <- time.Now() })
	defer tracker.close()
	acceptDone := make(chan struct{})
	go func() {
		closeClients, _ := acceptDaemonListener(ctx, listener, srpc.NewServer(resourceMux), tracker)
		closeClients()
		close(acceptDone)
	}()
	defer func() {
		_ = listener.Close()
		<-acceptDone
	}()
	publicClient, err := connectDaemonAtSocket(ctx, listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if publicClient != nil {
			publicClient.close()
		}
	}()
	observerDone := startDeviceCapacityObserver(ctx, le, statePath, resourceMux, store)
	defer func() {
		cancel()
		<-observerDone
	}()

	// The initial projection runs before and after the first policy snapshot.
	for range 2 {
		select {
		case err := <-projections:
			if err != nil {
				t.Fatalf("initial Resource projection: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("initial observer projection missing")
		}
	}
	if err := device_policy.WriteFile(statePath, &device_policy.DevicePolicy{Revision: 2}); err != nil {
		t.Fatal(err)
	}
	if err := store.Reload(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-projections:
		if err != nil {
			t.Fatalf("policy-change Resource projection: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("policy change was not observed")
	}

	// The daemon's own Resource call must not delay expiry after the client leaves.
	tracker.mu.Lock()
	active := tracker.active
	tracker.mu.Unlock()
	if active != 1 {
		t.Fatalf("counted public clients = %d, want 1", active)
	}
	leftAt := time.Now()
	publicClient.close()
	publicClient = nil
	select {
	case firedAt := <-idleCh:
		if firedAt.Sub(leftAt) < idleTimeout {
			t.Fatalf("idle fired after %v, before %v deadline", firedAt.Sub(leftAt), idleTimeout)
		}
	case <-time.After(time.Second):
		t.Fatal("daemon did not become idle after its final public client left")
	}
}
