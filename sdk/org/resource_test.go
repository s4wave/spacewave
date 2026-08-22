package s4wave_org

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
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
	if resp != nil {
		if err := s.Send(resp); err != nil {
			return err
		}
	}
	return s.CloseSend()
}

func recvOrgTestValue[T any](t *testing.T, ch <-chan T, name string) T {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

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

// setupOrgWatchWorld builds a mock world with one org object.
func setupOrgWatchWorld(
	t *testing.T,
	ctx context.Context,
	objKey string,
	state *OrgState,
) (*world_block.WorldState, func()) {
	t.Helper()

	log := logrus.New()
	le := logrus.NewEntry(log)
	tb, err := db_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		tb.Release()
		t.Fatal(err.Error())
	}
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		ocs.Release()
		tb.Release()
		t.Fatal(err.Error())
	}
	_, _, err = world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(state, true)
		return nil
	})
	if err == nil {
		err = ws.Commit(ctx)
	}
	if err != nil {
		ocs.Release()
		tb.Release()
		t.Fatal(err.Error())
	}
	return ws, func() {
		ocs.Release()
		tb.Release()
	}
}

// setOrgWorldState writes the org state block directly to the world object.
func setOrgWorldState(
	t *testing.T,
	ctx context.Context,
	ws *world_block.WorldState,
	objKey string,
	state *OrgState,
) {
	t.Helper()

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
	ctx := t.Context()
	objKey := "org/watch-shared"
	initial := &OrgState{DisplayName: "initial"}
	ws, cleanup := setupOrgWatchWorld(t, ctx, objKey, initial)
	defer cleanup()

	resource := NewOrgResource(ws, objKey, initial)
	defer resource.Close()

	strm := newOrgStateStream(ctx)
	done := make(chan error, 1)
	go func() {
		done <- resource.WatchOrgState(&WatchOrgStateRequest{}, strm)
	}()

	first := recvOrgTestValue(t, strm.sent, "initial snapshot").GetState()
	if first.GetDisplayName() != "initial" {
		t.Fatalf("initial display name = %q, want initial", first.GetDisplayName())
	}

	setOrgWorldState(t, ctx, ws, objKey, &OrgState{DisplayName: "updated"})
	second := recvOrgTestValue(t, strm.sent, "update emission").GetState()
	if second.GetDisplayName() != "updated" {
		t.Fatalf("update display name = %q, want updated", second.GetDisplayName())
	}
}
