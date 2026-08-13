//go:build !tinygo

package s4wave_forge_world

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/controllerbus/controller"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/aperturerobotics/starpc/srpc"
	worker_controller "github.com/s4wave/spacewave/forge/worker/controller"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	"github.com/sirupsen/logrus"
)

func TestForgeWorkerExecuteReturnsWorkerControllerError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	le := logrus.NewEntry(logrus.New())
	workerPeer, _, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	controllerErr := stderrors.New("worker watch failed")
	baseBus := inmem.NewBus(directive_controller.NewController(ctx, le))
	workerBus := &workerExitBus{Bus: baseBus, exitErr: controllerErr}
	stream := &forgeWorkerExecuteStream{ctx: ctx}
	resource := &forgeWorkerResource{
		objectKey: "worker/test",
		b:         workerBus,
		le:        le,
		peerID:    workerPeer.GetPeerID(),
	}

	err = resource.Execute(nil, stream)

	if !stderrors.Is(err, controllerErr) {
		t.Fatalf("Execute error = %v, want %v", err, controllerErr)
	}
	if stream.statuses != 1 {
		t.Fatalf("status count = %d, want 1", stream.statuses)
	}
}

type workerExitBus struct {
	bus.Bus
	exitErr error
}

func (b *workerExitBus) AddController(
	ctx context.Context,
	ctrl controller.Controller,
	cb func(error),
) (func(), error) {
	if _, ok := ctrl.(*worker_controller.Controller); ok {
		cb(b.exitErr)
		return func() {}, nil
	}
	return b.Bus.AddController(ctx, ctrl, cb)
}

type forgeWorkerExecuteStream struct {
	srpc.Stream
	ctx      context.Context
	statuses int
}

func (s *forgeWorkerExecuteStream) Context() context.Context {
	return s.ctx
}

func (s *forgeWorkerExecuteStream) Send(*s4wave_process.ExecuteStatus) error {
	s.statuses++
	return nil
}

func (s *forgeWorkerExecuteStream) SendAndClose(status *s4wave_process.ExecuteStatus) error {
	return s.Send(status)
}

var _ s4wave_process.SRPCPersistentExecutionService_ExecuteStream = (*forgeWorkerExecuteStream)(nil)
