package cdn_world_controller

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_rootserve "github.com/s4wave/spacewave/core/cdn/rootserve"
	"github.com/sirupsen/logrus"
)

// TestMountsFromRootServeHandler proves a server that writes its own Space
// can serve the pointer from its own host: the World mounts with
// rootPointerBaseUrl at the handler, the pack host keeps the immutable rule,
// and a commit reaches the mount on its next pointer fetch.
func TestMountsFromRootServeHandler(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// Serve packs from one host and pointers from the handler on another.
	var packReads atomic.Int32
	packURL := serveTestCDN(t, cdn_rootserve.ImmutablePacks(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		packReads.Add(1)
		http.NotFound(w, r)
	})))
	roots := cdn_rootserve.NewHandler("", nil)
	rootURL := serveTestCDN(t, roots)

	// Mount the World before the Space has a pointer.
	conf := NewConfig("release", "release-space", packURL)
	conf.RootPointerBaseUrl = rootURL
	ctrl := NewController(logrus.NewEntry(logrus.New()), nil, conf)
	done := make(chan error, 1)
	go func() { done <- ctrl.Execute(ctx) }()
	defer func() { cancel(); <-done }()
	if _, err := ctrl.ctr.WaitValueWithValidator(ctx, func(state *mountState) (bool, error) {
		return state != nil && isMissingPublishedHead(state.err), nil
	}, nil); err != nil {
		t.Fatal(err)
	}

	// Commit the Space's first pointer and poll it.
	if err := roots.SetOwned(&cdn.CdnRootPointer{
		SpaceId:    "release-space",
		Checkpoint: testHeadCheckpoint(t, "release-space"),
	}); err != nil {
		t.Fatal(err)
	}
	client := NewSRPCWorldRefreshClientWithServiceID(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(ctrl))), WorldRefreshServiceID("release"))
	if response, err := client.Refresh(ctx, &RefreshRequest{SpaceId: "release-space"}); err != nil || !response.GetAccepted() {
		t.Fatalf("refresh: %v %v", response, err)
	}

	// The World mounts from the handler's pointer without asking the pack
	// host for it.
	engine, err := ctrl.GetWorldEngine(ctx)
	if err != nil || engine == nil {
		t.Fatalf("engine: %v %v", engine, err)
	}
	if n := packReads.Load(); n != 0 {
		t.Fatalf("pack host read %d times for an empty World", n)
	}
}
