//go:build !js

package spacewave_compose

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/s4wave/spacewave/core/daemon"
)

// TestDesktopActionKeepsStateRootsSeparate checks that desktop requests and a
// retained Resource stream follow their selected protected socket.
func TestDesktopActionKeepsStateRootsSeparate(t *testing.T) {
	rootA := desktopActionStatePath(t)
	rootB := desktopActionStatePath(t)
	fixtureA := newDesktopActionFixture(t, rootA, true)
	fixtureB := newDesktopActionFixture(t, rootB, true)
	connector := daemon.NewConnector(nil, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// Open each root's desktop through the same launcher entrypoint.
	if err := os.Setenv("SPACEWAVE_STATE_PATH", rootA); err != nil {
		t.Fatal(err)
	}
	if err := openDesktopWithConnector(ctx, connector); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("SPACEWAVE_STATE_PATH", rootB); err != nil {
		t.Fatal(err)
	}
	if err := openDesktopWithConnector(ctx, connector); err != nil {
		t.Fatal(err)
	}
	retainedB, err := connector.Connect(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer retainedB.Close()

	// Reopening A must leave B's desktop and Resource stream on B's daemon.
	rootRPC, err := retainedB.Root().GetResourceRef().GetClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := rootRPC.NewStream(ctx, "test.DesktopActionWatch", "Watch", &emptypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("SPACEWAVE_STATE_PATH", rootA); err != nil {
		t.Fatal(err)
	}
	if err := openDesktopWithConnector(ctx, connector); err != nil {
		t.Fatal(err)
	}
	if fixtureA.control.openCount() != 2 || fixtureB.control.openCount() != 1 {
		t.Fatalf("desktop opens: A=%d B=%d, want 2 and 1", fixtureA.control.openCount(), fixtureB.control.openCount())
	}
	fixtureB.events <- struct{}{}
	if err := stream.MsgRecv(&emptypb.Empty{}); err != nil {
		t.Fatalf("root B Resource stream after root A reopen: %v", err)
	}
}
