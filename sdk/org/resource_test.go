package s4wave_org

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

type orgStateStream struct {
	ctx  context.Context
	sent chan *WatchOrgStateResponse
}

func newOrgStateStream(ctx context.Context) *orgStateStream {
	return &orgStateStream{ctx: ctx, sent: make(chan *WatchOrgStateResponse, 8)}
}

func (s *orgStateStream) Context() context.Context { return s.ctx }

func (s *orgStateStream) MsgSend(srpc.Message) error { panic("MsgSend should not be called") }

func (s *orgStateStream) MsgRecv(srpc.Message) error { panic("MsgRecv should not be called") }

func (s *orgStateStream) CloseSend() error { return nil }

func (s *orgStateStream) Close() error { return nil }

func (s *orgStateStream) Send(resp *WatchOrgStateResponse) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case s.sent <- resp.CloneVT():
		return nil
	}
}

func (s *orgStateStream) SendAndClose(resp *WatchOrgStateResponse) error {
	// Deliver the final response before closing the sending side.
	if resp != nil {
		if err := s.Send(resp); err != nil {
			return err
		}
	}

	// The test stream owns no transport beyond its response channel.
	return s.CloseSend()
}

func recvOrgTestValue[T any](t *testing.T, ch <-chan T, name string) T {
	t.Helper()

	// Cleanup joins the stream after t.Context is canceled, so its bounded
	// wait must remain live through test cleanup.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Wait for the event or report the missing observable behavior.
	select {
	case val, ok := <-ch:
		if !ok {
			t.Fatalf("%s channel closed", name)
		}
		return val
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %s", name)
	}

	var zero T
	return zero
}

// setupOrgWatchWorld builds a synchronized World with one org object.
func setupOrgWatchWorld(
	t *testing.T,
	ctx context.Context,
	objKey string,
	state *OrgState,
) *world_block.Tx {
	t.Helper()

	// Keep the backing store alive until the watcher and transaction close.
	log := logrus.New()
	le := logrus.NewEntry(log)
	tb, err := db_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Open a cursor for the test's World storage.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(ocs.Release)

	// Raw WorldState permits no concurrent access; Tx serializes the watcher
	// against the test's writes and commits through the production contract.
	stateWorld, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	ws := world_block.NewTx(stateWorld)
	t.Cleanup(ws.Discard)

	// Seed the object before starting its watch.
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(state, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err == nil {
		err = ws.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err.Error())
	}
	return ws
}

// setOrgWorldState writes the org state block directly to the world object.
func setOrgWorldState(
	t *testing.T,
	ctx context.Context,
	ws *world_block.Tx,
	objKey string,
	state *OrgState,
) {
	t.Helper()

	// Publish the object's new body and commit it through the synchronized Tx.
	_, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(state, true)
		return nil
	})
	if err == nil {
		err = ws.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err.Error())
	}
}

// TestOrgResourceWatchSharesWorldUpdates pins that WatchOrgState emits a new
// snapshot after each World revision instead of hanging after the first one.
func TestOrgResourceWatchSharesWorldUpdates(t *testing.T) {
	// Create an independent World and scope the stream to this test.
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	objKey := "org/watch-shared"
	initial := &OrgState{DisplayName: "initial"}
	ws := setupOrgWatchWorld(t, ctx, objKey, initial)

	// Stop the resource watcher before its backing World is discarded.
	resource := NewOrgResource(ws, objKey, initial)
	t.Cleanup(resource.Close)

	// Start the consumer and join it before releasing its World.
	strm := newOrgStateStream(ctx)
	done := make(chan error, 1)
	go func() {
		done <- resource.WatchOrgState(&WatchOrgStateRequest{}, strm)
	}()
	t.Cleanup(func() {
		cancel()
		if err := recvOrgTestValue(t, done, "watch shutdown"); !errors.Is(err, context.Canceled) {
			t.Fatalf("watch shutdown = %v, want context cancellation", err)
		}
	})

	// The initial emission must expose the seeded state.
	first := recvOrgTestValue(t, strm.sent, "initial snapshot").GetState()
	if first.GetDisplayName() != "initial" {
		t.Fatalf("initial display name = %q, want initial", first.GetDisplayName())
	}

	// A committed edit must reach the same stream without a new subscription.
	setOrgWorldState(t, ctx, ws, objKey, &OrgState{DisplayName: "updated"})
	second := recvOrgTestValue(t, strm.sent, "update emission").GetState()
	if second.GetDisplayName() != "updated" {
		t.Fatalf("update display name = %q, want updated", second.GetDisplayName())
	}
}
