package resource_server

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
)

func ownershipTestClient(t *testing.T) (*ResourceServer, *RemoteResourceClient) {
	t.Helper()
	server := NewResourceServer(nil)
	client := &RemoteResourceClient{
		server:         server,
		clientID:       1,
		rootResourceID: 1,
		ctx:            t.Context(),
		resources:      make(map[uint32]*trackedResource),
		children:       make(map[uint32]map[uint32]struct{}),
		tombstones:     make(map[uint32]struct{}),
	}
	server.clients[1] = client
	server.resourceIDCtr = 1
	client.resources[1] = &trackedResource{
		mux:           srpc.NewMux(),
		ownerClientID: 1,
		createdAt:     server.now(),
	}
	return server, client
}

func TestPendingChildrenReleasePostorderAndRootRetention(t *testing.T) {
	_, client := ownershipTestClient(t)
	var order []uint32
	releaseChild := func() { order = append(order, 2) }
	child, err := client.addInvocationResource(
		1, "svc", "method", srpc.NewMux(), nil, releaseChild,
	)
	if err != nil || child != 2 {
		t.Fatalf("child = %d/%v", child, err)
	}
	releaseGrandchild := func() { order = append(order, 3) }
	grandchild, err := client.addInvocationResource(
		child, "svc", "method", srpc.NewMux(), nil, releaseGrandchild,
	)
	if err != nil || grandchild != 3 {
		t.Fatalf("grandchild = %d/%v", grandchild, err)
	}
	if _, err := client.releaseClientControl(1); err != nil {
		t.Fatalf("root release: %v", err)
	}
	if client.resources[1] == nil {
		t.Fatal("root was released")
	}
	if client.resources[2] != nil || client.resources[3] != nil {
		t.Fatal("pending descendants survived root release")
	}
	if len(order) != 2 || order[0] != 3 || order[1] != 2 {
		t.Fatalf("release order = %v", order)
	}
	if len(client.txQueue) != 2 {
		t.Fatalf("notifications = %d, want 2", len(client.txQueue))
	}
}

func TestGenerationCleanupReleasesAdoptedTreeChildFirst(t *testing.T) {
	_, client := ownershipTestClient(t)
	var order []uint32
	releaseChild := func() { order = append(order, 2) }
	child, err := client.addInvocationResource(
		1, "svc", "method", srpc.NewMux(), nil, releaseChild,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !client.adoptResource(child) {
		t.Fatal("adopt rejected")
	}
	releaseGrandchild := func() { order = append(order, 3) }
	grandchild, err := client.addInvocationResource(
		child, "svc", "method", srpc.NewMux(), nil, releaseGrandchild,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !client.adoptResource(grandchild) {
		t.Fatal("grandchild adopt rejected")
	}
	var releaseFns []func()
	client.releaseAllChildrenLocked(1, &releaseFns)
	for _, releaseFn := range releaseFns {
		releaseFn()
	}
	if len(order) != 2 || order[0] != 3 || order[1] != 2 {
		t.Fatalf("cleanup order = %v", order)
	}
	if len(client.resources) != 0 {
		t.Fatalf("resources after cleanup = %d", len(client.resources))
	}
}

func TestAdoptedChildSurvivesParentReleaseAndTombstoneNotifies(t *testing.T) {
	_, client := ownershipTestClient(t)
	child, err := client.addInvocationResource(1, "svc", "method", srpc.NewMux(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !client.adoptResource(child) {
		t.Fatal("adopt rejected")
	}
	if _, err := client.releaseClientControl(1); err != nil {
		t.Fatal(err)
	}
	if client.resources[child] == nil {
		t.Fatal("adopted child was released with parent")
	}
	if !client.ReleaseResource(child) {
		t.Fatal("server release rejected")
	}
	if len(client.txQueue) != 1 {
		t.Fatalf("server release notifications = %d, want 1", len(client.txQueue))
	}
	client.txQueue = nil
	if !client.adoptResource(child) {
		t.Fatal("stale adopt did not succeed")
	}
	if len(client.txQueue) != 1 {
		t.Fatalf("stale adopt notifications = %d, want 1", len(client.txQueue))
	}
}

func TestForeignAndNeverAllocatedControlsTerminate(t *testing.T) {
	_, client := ownershipTestClient(t)
	if _, err := client.releaseClientControl(999); err != resource.ErrResourceNotFound {
		t.Fatalf("unknown release error = %v", err)
	}
	if client.adoptResource(999) {
		t.Fatal("unknown adopt accepted")
	}
}

func TestPendingWarningRunsOnceWithoutChangingLifetime(t *testing.T) {
	server, client := ownershipTestClient(t)
	server.pendingWarningAge = 0
	warnings := make(chan pendingResourceWarning, 1)
	server.pendingWarningHandler = func(warning pendingResourceWarning) {
		warnings <- warning
	}

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	scanDone := make(chan struct{})
	go func() {
		server.scanPendingResources(ctx, client)
		close(scanDone)
	}()

	firstID, err := client.addInvocationResource(1, "svc", "first", srpc.NewMux(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case warning := <-warnings:
		if warning.resourceID != firstID {
			t.Fatalf("first warning resource = %d, want %d", warning.resourceID, firstID)
		}
	case <-time.After(time.Second):
		t.Fatal("first pending resource warning was not reported")
	}

	secondID, err := client.addInvocationResource(1, "svc", "second", srpc.NewMux(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case warning := <-warnings:
		if warning.resourceID != secondID {
			t.Fatalf("second warning resource = %d, want %d", warning.resourceID, secondID)
		}
	case <-time.After(time.Second):
		t.Fatal("second pending resource warning was not reported")
	}

	cancel()
	<-scanDone
	select {
	case warning := <-warnings:
		t.Fatalf("duplicate warning = %#v", warning)
	default:
	}

	var resourceCount int
	server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		resourceCount = len(client.resources)
	})
	if resourceCount != 3 {
		t.Fatalf("scanner changed resources: %d", resourceCount)
	}
}
