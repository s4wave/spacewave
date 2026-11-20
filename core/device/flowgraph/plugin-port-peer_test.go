//go:build !js

package device_flowgraph

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/core/transport"
	net_testbed "github.com/s4wave/spacewave/net/testbed"
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/sirupsen/logrus"
)

// newPluginPortPeer runs an independent testbed Session on the fixture network.
// It returns the Session and the testbed bus where the Session is published.
func newPluginPortPeer(t *testing.T, ctx context.Context, le *logrus.Entry, network *inproc.Network) (*transport.SessionTransport, bus.Bus) {
	// Own the independent bus and its Session until test cleanup.
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	tb, err := net_testbed.NewTestbed(ctx, le, net_testbed.TestbedOpts{NoEcho: true})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	st, err := transport.NewSessionTransport(le, tb.Bus, tb.PrivKey, "", "", transport.WithInprocNetwork(network))
	if err != nil {
		cancel()
		tb.Release()
		t.Fatal(err)
	}

	// Wait for the Session's actual readiness and drain its execution at cleanup.
	done := make(chan error, 1)
	go func() { done <- st.Execute(ctx) }()
	t.Cleanup(func() { cancel(); <-done; tb.Release() })
	if err := st.AwaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	return st, tb.Bus
}
