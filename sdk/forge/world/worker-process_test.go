//go:build !tinygo

package s4wave_forge_world

import (
	"context"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	forge_testbed "github.com/s4wave/spacewave/forge/testbed"
	worker_controller "github.com/s4wave/spacewave/forge/worker/controller"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	"github.com/sirupsen/logrus"
)

// TestForgeWorkerProcessSteadyStatus checks that the in-process status stream
// stays silent between RUNNING and cancellation across virtual heartbeat periods.
func TestForgeWorkerProcessSteadyStatus(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Keep the controller bus alive after execution cancellation.
		le := logrus.NewEntry(logrus.New())
		base := inmem.NewBus(directive_controller.NewController(t.Context(), le))
		workerBus := &workerExitBus{Bus: base, started: make(chan struct{}), released: make(chan struct{})}
		workerPeer, _, _, err := peer.NewPeerWithGenerateED25519()
		if err != nil {
			t.Fatal(err)
		}
		resource := &forgeWorkerResource{
			objectKey:            "worker/steady-stream",
			b:                    workerBus,
			le:                   le,
			peerID:               workerPeer.GetPeerID(),
			admission:            testWorkerRuntime{},
			openDeclarationWatch: openTestWorkerDeclarationWatch,
		}

		// Start the persistent process over the production SRPC pipe.
		mux := resource_server.NewResourceMux(func(mux srpc.Mux) error {
			return s4wave_process.SRPCRegisterPersistentExecutionService(mux, resource)
		})
		client := s4wave_process.NewSRPCPersistentExecutionServiceClient(
			srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))),
		)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		stream, err := client.Execute(ctx, &s4wave_process.ExecuteRequest{})
		if err != nil {
			t.Fatal(err)
		}

		// Require the initial running state before observing steady execution.
		status, err := stream.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if status.GetState() != s4wave_process.ExecutionState_ExecutionState_RUNNING {
			t.Fatalf("initial status = %s, want RUNNING", status.GetState())
		}
		<-workerBus.started

		// A pending receive must stay blocked through former heartbeat periods.
		terminal := make(chan error, 1)
		go func() {
			_, err := stream.Recv()
			terminal <- err
		}()
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		select {
		case err := <-terminal:
			t.Fatalf("steady execution received another status or ended: %v", err)
		default:
		}

		// Cancellation ends the stream and releases all execution controllers.
		cancel()
		if err := <-terminal; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled stream = %v", err)
		}
		<-workerBus.released
		synctest.Wait()
		if remaining := len(base.GetControllers()); remaining != 0 {
			t.Fatalf("controllers after cancellation = %d, want zero", remaining)
		}
	})
}

// TestForgeWorkerProcessStream checks status and teardown through the same
// in-process transport used by Space process bindings.
func TestForgeWorkerProcessStream(t *testing.T) {
	// Exercise actual Worker cancellation and both controller exit results.
	for _, test := range []struct {
		// name identifies the terminal transition.
		name string
		// cancel keeps the real Worker running until stream cancellation.
		cancel bool
		// exitErr supplies the controller's terminal failure.
		exitErr error
	}{
		{name: "cancellation", cancel: true},
		{name: "clean exit"},
		{name: "failed exit", exitErr: errors.New("worker watch failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Keep the Forge testbed alive after the execution stream ends.
			testCtx, cancelTest := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancelTest()
			tb, err := forge_testbed.Default(testCtx)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tb.Release)
			baseline := len(tb.Bus.GetControllers())

			// Use the real Worker for cancellation and supply terminal exits
			// through the existing controller callback testbed.
			workerBus := tb.Bus
			if !test.cancel {
				workerBus = &workerExitBus{Bus: tb.Bus, exitErr: test.exitErr}
			}
			resource := &forgeWorkerResource{
				objectKey:            "worker/process-stream",
				ws:                   tb.WorldState,
				b:                    workerBus,
				le:                   tb.Logger,
				peerID:               tb.Volume.GetPeerID(),
				engineID:             tb.EngineID,
				admission:            testWorkerRuntime{},
				openDeclarationWatch: openTestWorkerDeclarationWatch,
			}

			// Start the persistent process over the production SRPC pipe.
			mux := resource_server.NewResourceMux(func(mux srpc.Mux) error {
				return s4wave_process.SRPCRegisterPersistentExecutionService(mux, resource)
			})
			client := s4wave_process.NewSRPCPersistentExecutionServiceClient(
				srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))),
			)
			ctx, cancel := context.WithCancel(testCtx)
			defer cancel()
			stream, err := client.Execute(ctx, &s4wave_process.ExecuteRequest{})
			if err != nil {
				t.Fatal(err)
			}

			// The first message describes running state, not a liveness lease.
			status, err := stream.Recv()
			if err != nil {
				t.Fatal(err)
			}
			if status.GetState() != s4wave_process.ExecutionState_ExecutionState_RUNNING {
				t.Fatalf("initial status = %s, want RUNNING", status.GetState())
			}
			terminal := make(chan error, 1)
			go func() {
				_, err := stream.Recv()
				terminal <- err
			}()

			// Cancel only after the real Worker controller has attached.
			if test.cancel {
				err := broadcast.WatchBroadcast(testCtx, tb.Bus.GetControllersBroadcast(), func() bool {
					for _, ctrl := range tb.Bus.GetControllers() {
						if _, ok := ctrl.(*worker_controller.Controller); ok {
							return true
						}
					}
					return false
				}, func(attached bool) error {
					if attached {
						return io.EOF
					}
					return nil
				})
				if !errors.Is(err, io.EOF) {
					t.Fatalf("wait for Worker controller: %v", err)
				}
				cancel()
			}

			// Require the stream to close with the controller's terminal result.
			err = <-terminal
			switch {
			case test.cancel:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancelled stream = %v", err)
				}
			case test.exitErr != nil:
				if err == nil || !strings.Contains(err.Error(), test.exitErr.Error()) {
					t.Fatalf("failed stream = %v, want %v", err, test.exitErr)
				}
			default:
				if !errors.Is(err, io.EOF) {
					t.Fatalf("clean stream = %v, want EOF", err)
				}
			}

			// Server cleanup must release its controllers without closing
			// the surrounding testbed or relying on another status message.
			err = broadcast.WatchBroadcast(testCtx, tb.Bus.GetControllersBroadcast(), func() int {
				return len(tb.Bus.GetControllers())
			}, func(remaining int) error {
				if remaining == baseline {
					return io.EOF
				}
				return nil
			})
			if !errors.Is(err, io.EOF) {
				t.Fatalf("controllers after execution = %d, want baseline %d: %v", len(tb.Bus.GetControllers()), baseline, err)
			}
		})
	}
}
