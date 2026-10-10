//go:build !goscript

package sobject_sync

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	bus_inmem "github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/callback"
	"github.com/aperturerobotics/controllerbus/directive"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
)

// TestSOSyncExecuteRearmsRecoverableFailuresOnly verifies that a transient
// stream failure replaces the directive generation while typed peer states do
// not create a retry loop.
func TestSOSyncExecuteRearmsRecoverableFailuresOnly(t *testing.T) {
	tests := []struct {
		name       string
		streamErr  error
		admissions int32
	}{
		{
			name:       "recoverable",
			streamErr:  errors.New("peer snapshot conflicts with accepted root"),
			admissions: 2,
		},
		{name: "access denied", streamErr: ErrAccessDenied, admissions: 1},
		{
			name:       "participant revoked",
			streamErr:  sobject.ErrParticipantRevoked,
			admissions: 1,
		},
		{
			name:       "history recovery",
			streamErr:  sobject.ErrConfigHistoryUnavailable,
			admissions: 1,
		},
		{
			name:       "state too large",
			streamErr:  sobject.ErrStateTooLarge,
			admissions: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// Resolve the first solicitation with the selected stream failure and
				// leave any replacement generation admitted without another value.
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				le := gateLogger()
				b := bus_inmem.NewBus(directive_controller.NewController(ctx, le))

				// Perform the action.
				var admissions atomic.Int32
				admitted := make(chan directive.Instance, 2)
				info := controller.NewInfo(
					"test/so-sync-rearm",
					controller.MustParseVersion("0.0.1"),
					"SO sync rearm test",
				)
				resolver := callback.NewCallbackController(
					info,
					nil,
					func(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
						// Resolve only the protocol under test.
						if _, ok := di.GetDirective().(link_solicit.SolicitProtocol); !ok {
							return nil, nil
						}

						// Record each distinct directive admission.
						attempt := admissions.Add(1)
						admitted <- di
						if attempt != 1 {
							return nil, nil
						}

						// Fail the first stream with the selected error.
						return []directive.Resolver{directive.NewValueResolver(
							[]link_solicit.SolicitMountedStream{
								link_solicit.NewSolicitMountedStreamWithErr(test.streamErr),
							},
						)}, nil
					},
					nil,
				)
				releaseResolver, err := b.AddController(ctx, resolver, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer releaseResolver()

				// Run the production Execute loop and wait until all retry timers and
				// directive callbacks have settled.
				syncer := NewSOSync(le, b, "test-object", "", nil, nil, nil)
				errCh := make(chan error, 1)
				go func() {
					errCh <- syncer.Execute(ctx)
				}()
				first := <-admitted
				var second directive.Instance
				if test.admissions == 2 {
					second = <-admitted
				}
				synctest.Wait()

				// A replacement is a distinct directive admission, not another value
				// from the failed directive instance.
				if got := admissions.Load(); got != test.admissions {
					t.Fatalf("expected %d solicitation admissions, got %d", test.admissions, got)
				}
				if second != nil && first == second {
					t.Fatal("recoverable failure reused the failed directive instance")
				}

				// Cancellation returns only after the active generation has drained.
				cancel()
				if err := <-errCh; !errors.Is(err, context.Canceled) {
					t.Fatalf("expected context cancellation, got %v", err)
				}
			})
		})
	}
}

// TestSolicitationGenerationStopWaitsForWorkers verifies that a generation
// cannot finish shutdown while an admitted worker is still returning.
func TestSolicitationGenerationStopWaitsForWorkers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Admit one worker whose return is controlled independently of cancellation.
		ctx, cancel := context.WithCancel(t.Context())
		generation := &solicitationGeneration{}
		started := make(chan struct{})
		release := make(chan struct{})
		if !generation.startWorker(func() error {
			close(started)
			<-release
			return nil
		}) {
			t.Fatal("generation rejected its first worker")
		}
		<-started

		// Start shutdown and prove it remains blocked on the admitted worker.
		stopped := make(chan struct{})
		go func() {
			generation.stopAndWait(cancel)
			close(stopped)
		}()
		synctest.Wait()
		select {
		case <-stopped:
			close(release)
			t.Fatal("generation stopped before its worker returned")
		default:
		}

		// Releasing the worker completes shutdown and leaves admission fenced.
		close(release)
		<-stopped
		if generation.startWorker(func() error { return nil }) {
			t.Fatal("stopped generation admitted another worker")
		}
		if err := ctx.Err(); !errors.Is(err, context.Canceled) {
			t.Fatalf("expected worker context cancellation, got %v", err)
		}
	})
}

// TestSyncRetryBackoffIsCapped verifies the exact retry-delay ceiling.
func TestSyncRetryBackoffIsCapped(t *testing.T) {
	// Construct the policy and its expected capped sequence.
	backoff := newSyncRetryBackoff()
	want := []time.Duration{
		250 * time.Millisecond,
		500 * time.Millisecond,
		time.Second,
		2 * time.Second,
		2 * time.Second,
	}

	// Compare every delay through and beyond the ceiling.
	for i, expected := range want {
		if got := backoff.NextBackOff(); got != expected {
			t.Fatalf("delay %d: expected %s, got %s", i, expected, got)
		}
	}
}

// TestLogStreamResultReportsEachFailureChange verifies that a peer's blocked
// synchronization reaches the warn log once, however often the peer retries,
// and that recovery reaches the info log.
func TestLogStreamResultReportsEachFailureChange(t *testing.T) {
	// Capture the info and warn log of a syncer.
	log, hook := logtest.NewNullLogger()
	log.SetLevel(logrus.InfoLevel)
	le := logrus.NewEntry(log)
	syncer := NewSOSync(le, nil, "test-object", "", nil, nil, nil)

	// A terminal failure warns on its first stream and stays quiet after.
	blocked := errors.Wrap(sobject.ErrStateTooLarge, "12582912 bytes")
	for range 3 {
		syncer.logStreamResult(le, "peer-a", blocked)
	}
	if got := hook.AllEntries(); len(got) != 1 || got[0].Level != logrus.WarnLevel {
		t.Fatalf("repeated terminal failure logged %v; want one warning", got)
	}

	// Another peer is tracked on its own.
	hook.Reset()
	syncer.logStreamResult(le, "peer-b", blocked)
	if got := hook.AllEntries(); len(got) != 1 || got[0].Level != logrus.WarnLevel {
		t.Fatalf("second peer logged %v; want one warning", got)
	}

	// A clean stream reports recovery once, and a later failure warns again.
	hook.Reset()
	syncer.logStreamResult(le, "peer-a", nil)
	syncer.logStreamResult(le, "peer-a", nil)
	syncer.logStreamResult(le, "peer-a", blocked)
	got := hook.AllEntries()
	if len(got) != 2 || got[0].Level != logrus.InfoLevel || got[1].Level != logrus.WarnLevel {
		t.Fatalf("recovery and relapse logged %v; want info, warning", got)
	}

	// A recoverable failure that changes is reported at info.
	hook.Reset()
	syncer.logStreamResult(le, "peer-a", errors.New("EOF"))
	if got := hook.AllEntries(); len(got) != 1 || got[0].Level != logrus.InfoLevel {
		t.Fatalf("recoverable failure logged %v; want one info", got)
	}
}
