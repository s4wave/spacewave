package s4wave_layout_world_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	"github.com/sirupsen/logrus"
)

type layoutWatchSeedStream struct {
	ctx    context.Context
	cancel context.CancelFunc
	sent   chan []byte
}

func newLayoutWatchSeedStream(ctx context.Context) *layoutWatchSeedStream {
	streamCtx, cancel := context.WithCancel(ctx)
	return &layoutWatchSeedStream{
		ctx:    streamCtx,
		cancel: cancel,
		sent:   make(chan []byte, 1),
	}
}

func (s *layoutWatchSeedStream) Context() context.Context {
	return s.ctx
}

func (s *layoutWatchSeedStream) MsgSend(msg srpc.Message) error {
	data, err := msg.MarshalVT()
	if err != nil {
		return err
	}
	select {
	case s.sent <- data:
	default:
	}
	return nil
}

func (s *layoutWatchSeedStream) MsgRecv(srpc.Message) error {
	<-s.ctx.Done()
	return s.ctx.Err()
}

func (s *layoutWatchSeedStream) CloseSend() error {
	return nil
}

func (s *layoutWatchSeedStream) Close() error {
	s.cancel()
	return nil
}

func TestObjectLayoutFactoryPublishesSeedModelFromEngineWorldState(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer btb.Release()

	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	objectKey := "object-layout/test-factory-engine-state"
	writeState := world.NewEngineWorldState(wtb.Engine, true)
	if _, _, err := space_world_ops.InitObjectLayout(ctx, writeState, wtb.Volume.GetPeerID(), objectKey, time.Now()); err != nil {
		t.Fatal(err)
	}

	readState := world.NewEngineWorldState(wtb.Engine, true)
	invoker, cleanup, err := s4wave_layout_world.ObjectLayoutFactory(ctx, le, btb.Bus, wtb.Engine, readState, objectKey)
	if err != nil {
		t.Fatalf("ObjectLayoutFactory: %v", err)
	}
	defer cleanup()

	stream := newLayoutWatchSeedStream(ctx)
	done := make(chan error, 1)
	go func() {
		ok, err := invoker.InvokeMethod(s4wave_layout.SRPCLayoutHostServiceID, "WatchLayoutModel", stream)
		if err != nil {
			done <- err
			return
		}
		if !ok {
			done <- srpc.ErrUnimplemented
			return
		}
		done <- nil
	}()

	var model s4wave_layout.LayoutModel
	select {
	case data := <-stream.sent:
		if err := model.UnmarshalVT(data); err != nil {
			t.Fatalf("unmarshal watched model: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for seeded layout model")
	}
	stream.cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out stopping layout watch")
	}

	root := model.GetLayout()
	if root.GetId() != "root" {
		t.Fatalf("layout root id = %q, want root", root.GetId())
	}
	if got := len(root.GetChildren()); got != 1 {
		t.Fatalf("layout root children = %d, want 1", got)
	}
	tabSet := root.GetChildren()[0].GetTabSet()
	if tabSet == nil {
		t.Fatalf("layout first child is %T, want tabset", root.GetChildren()[0].GetNode())
	}
	if got := len(tabSet.GetChildren()); got != 1 {
		t.Fatalf("tabset children = %d, want 1", got)
	}
	tab := tabSet.GetChildren()[0]
	if tab.GetId() != "files" {
		t.Fatalf("tab id = %q, want files", tab.GetId())
	}
	if tab.GetName() != "Files" {
		t.Fatalf("tab name = %q, want Files", tab.GetName())
	}
	var tabData s4wave_layout_world.ObjectLayoutTab
	if err := tabData.UnmarshalVT(tab.GetData()); err != nil {
		t.Fatal(err)
	}
	if got := tabData.GetObjectInfo().GetWorldObjectInfo().GetObjectKey(); got != "files" {
		t.Fatalf("tab object key = %q, want files", got)
	}
}
