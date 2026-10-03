package cdn_world_controller

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/core/cdn"
	"github.com/sirupsen/logrus"
)

// TestRefreshRPCRefetchesMountedWorld proves invalidation reaches the mounted
// CDN owner without replacing its engine or fetching unrelated Spaces.
func TestRefreshRPCRefetchesMountedWorld(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// Build a root pointer for a world with an empty head.
	pointer := encodeRootPointer(t, &cdn.CdnRootPointer{
		SpaceId:    "release-space",
		Checkpoint: testHeadCheckpoint(t, "release-space"),
	})

	// Serve the pointer and signal each fetch after the first.
	var requests atomic.Int32
	refetched := make(chan struct{}, 1)
	cdnURL := serveTestCDN(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) > 1 {
			select {
			case refetched <- struct{}{}:
			default:
			}
		}
		_, _ = w.Write(pointer)
	}))

	// Mount the world.
	ctrl := NewController(logrus.NewEntry(logrus.New()), nil, NewConfig("release", "release-space", cdnURL))
	done := make(chan error, 1)
	go func() { done <- ctrl.Execute(ctx) }()
	defer func() { cancel(); <-done }()
	engine, err := ctrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// A refresh for another Space is refused without a fetch.
	client := NewSRPCWorldRefreshClientWithServiceID(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(ctrl))), WorldRefreshServiceID("release"))
	response, err := client.Refresh(ctx, &RefreshRequest{SpaceId: "unrelated"})
	if err != nil || response.GetAccepted() || requests.Load() != 1 {
		t.Fatalf("unrelated refresh: %v %v requests=%d", response, err, requests.Load())
	}

	// A refresh for the mounted Space refetches the pointer.
	response, err = client.Refresh(ctx, &RefreshRequest{SpaceId: "release-space"})
	if err != nil || !response.GetAccepted() {
		t.Fatalf("refresh: %v %v", response, err)
	}
	select {
	case <-refetched:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// The mounted engine is kept.
	current, err := ctrl.GetWorldEngine(ctx)
	if err != nil || current != engine {
		t.Fatal("refresh replaced the mounted engine")
	}
}
