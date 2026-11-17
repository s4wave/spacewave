package pairing

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// TestResultRequiresDurableEnrollment refuses a result before enrollment persists.
func TestResultRequiresDurableEnrollment(t *testing.T) {
	// Fail Result when no exchange is active.
	remote := newEngineTestPeer(t)
	engine := &Engine{}
	if _, err := engine.Result(remote); !errors.Is(err, ErrExchangeMissing) {
		t.Fatalf("missing exchange returned %v", err)
	}

	// Fail Result while the exchange is confirmed but not durably enrolled.
	engine.active = &attempt{snapshot: Snapshot{RemotePeerID: remote, Status: StatusBothConfirmed, Receiving: true}}
	if _, err := engine.Result(remote); !errors.Is(err, ErrExchangeUnconfirmed) {
		t.Fatalf("completed status without enrollment returned %v", err)
	}

	// Fail Result for a peer other than the confirmed exchange.
	if _, err := engine.Result(newEngineTestPeer(t)); !errors.Is(err, ErrExchangePeerMismatch) {
		t.Fatalf("different peer returned %v", err)
	}

	// Return an independent copy of the durably enrolled Session.
	ref := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{ProviderId: "local", ProviderAccountId: "account", Id: "session"}}
	engine.active.result = ref
	result, err := engine.Result(remote)
	if err != nil {
		t.Fatal(err)
	}
	if !result.EqualVT(ref) || result == ref {
		t.Fatal("result did not return an independent copy of the enrolled Session")
	}
}

// newEngineTestPeer generates an independent pairing peer identity.
func newEngineTestPeer(t *testing.T) peer.ID {
	// Generate an Ed25519 keypair and derive its peer ID.
	t.Helper()
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// TestRelayWorkerTeardown ends both relay paths with their attempt or transport.
func TestRelayWorkerTeardown(t *testing.T) {
	for _, offering := range []bool{false, true} {
		for _, ending := range []string{"before-start", "transport", "attempt"} {
			t.Run(ending+map[bool]string{false: "/complete", true: "/solicit"}[offering], func(t *testing.T) {
				// Start a real Session transport while keeping the Engine's Session alive.
				tb, err := testbed.Default(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(tb.Release)
				key, _, err := crypto.GenerateEd25519Key(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				st, err := transport.NewSessionTransport(logrus.NewEntry(logrus.New()), tb.Bus, key, "", "")
				if err != nil {
					t.Fatal(err)
				}
				transportCtx, stopTransport := context.WithCancel(t.Context())
				t.Cleanup(stopTransport)
				transportDone := make(chan error, 1)
				go func() { transportDone <- st.Execute(transportCtx) }()
				if err := st.AwaitReady(t.Context()); err != nil {
					t.Fatal(err)
				}

				// Observe the production demand before interrupting an active worker.
				started := make(chan struct{}, 1)
				remove, err := st.GetChildBus().AddHandler(directive.NewFuncHandler(func(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
					_, connect := di.GetDirective().(link.EstablishLinkWithPeer)
					_, solicit := di.GetDirective().(link_solicit.SolicitProtocol)
					if connect && !offering || solicit && offering {
						select {
						case started <- struct{}{}:
						default:
						}
					}
					return nil, nil
				}))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(remove)

				// Stop the transport before scheduling the worker to reproduce the nil bus.
				if ending == "before-start" {
					stopTransport()
					if err := <-transportDone; !errors.Is(err, context.Canceled) {
						t.Fatalf("transport exit: %v", err)
					}
				}
				engine := &Engine{ctx: t.Context(), peerID: st.GetPeerID()}
				ctx, active := engine.begin(offering, false, "", "", newEngineTestPeer(t), StatusWaitingForPeer)
				done := make(chan struct{})
				go func() {
					engine.runRelay(ctx, active, st)
					close(done)
				}()

				// End either owner after the demand has entered the live bus.
				if ending != "before-start" {
					select {
					case <-started:
					case <-time.After(5 * time.Second):
						t.Fatal("relay worker did not enter the transport bus")
					}
					if ending == "attempt" {
						engine.Clear()
					}
					if ending == "transport" {
						stopTransport()
					}
				}

				// Require the worker to exit while the Session context is still live.
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("relay worker outlived its canceled owner")
				}
				if t.Context().Err() != nil {
					t.Fatal("worker only exited with the Session")
				}
				if ending == "attempt" && st.GetChildBus() == nil {
					t.Fatal("clearing pairing stopped the Session transport")
				}
				if ending != "attempt" {
					snapshot, _ := engine.Snapshot()
					if snapshot.Status != StatusFailed {
						t.Fatalf("transport teardown left pairing in %v", snapshot.Status)
					}
				}

				// Join the transport before releasing its testbed.
				if ending != "before-start" {
					stopTransport()
					if err := <-transportDone; !errors.Is(err, context.Canceled) {
						t.Fatalf("transport exit: %v", err)
					}
				}
			})
		}
	}
}

// TestRetainedPairingResources releases both the prepared enrollment and the
// final migrated Session, including a late result from a replaced attempt.
func TestRetainedPairingResources(t *testing.T) {
	// Start an offering attempt and retain two resources under it.
	engine := &Engine{ctx: t.Context()}
	_, active := engine.begin(true, true, "", "", "", StatusPeerConnected)
	var released int
	for range 2 {
		if !engine.retain(active, func() { released++ }) {
			t.Fatal("active pairing did not retain its resource")
		}
	}

	// Clear the engine and require both retained resources released.
	engine.Clear()
	if released != 2 {
		t.Fatalf("released %d of 2 pairing resources", released)
	}

	// Reject a late retain on the replaced attempt.
	if engine.retain(active, func() { released++ }) || released != 3 {
		t.Fatal("replaced pairing retained a late resource")
	}
}
