package inproc

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/stream"
	stream_echo "github.com/s4wave/spacewave/net/stream/echo"
	"github.com/s4wave/spacewave/net/testbed"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/sirupsen/logrus"
)

// buildTestbed builds a new testbed for udp.
func buildTestbed(t *testing.T, ctx context.Context) (*testbed.Testbed, *logrus.Entry) {
	// Configure the testbed logger to expose transport activity.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the network testbed with the in-process transport factory.
	tb, err := testbed.NewTestbed(ctx, le, testbed.TestbedOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))
	return tb, le
}

func execPeer(ctx context.Context, t *testing.T, tb *testbed.Testbed, conf *Config) (
	*transport_controller.Controller,
	*Inproc,
	directive.Reference,
) {
	// Derive the transport peer identity from the testbed key.
	peerId, err := peer.IDFromPrivateKey(tb.PrivKey)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Bind the in-process transport configuration to the testbed peer.
	if conf == nil {
		conf = &Config{}
	}
	conf.TransportPeerId = peerId.String()

	// Load the transport controller through the testbed bus.
	tpc1, _, tp1Ref, err := loader.WaitExecControllerRunningTyped[*transport_controller.Controller](
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(conf),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Obtain the running in-process transport from its controller.
	tpt1, err := tpc1.GetTransport(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	return tpc1, tpt1.(*Inproc), tp1Ref
}

// establishLink connects two in-memory peers and establishes a link from the
// second to the first.
func establishLink(ctx context.Context, t *testing.T) link.MountedLink {
	// Start two network testbeds with distinct peer loggers.
	tb1, le1 := buildTestbed(t, ctx)
	le1 = le1.WithField("testbed", 0)
	tb2, le2 := buildTestbed(t, ctx)
	le2 = le2.WithField("testbed", 1)

	// Start the first peer and retain its transport for the test.
	_, tp1, tp1Ref := execPeer(ctx, t, tb1, nil)
	peerId1 := tp1.GetPeerID()
	t.Cleanup(tp1Ref.Release)

	// Start the second peer with a dialer targeting the first transport.
	_, tp2, tp2Ref := execPeer(ctx, t, tb2, &Config{
		Dialers: map[string]*dialer.DialerOpts{
			peerId1.String(): {
				Address: tp1.LocalAddr().String(),
			},
		},
	})
	peerId2 := tp2.GetPeerID()
	t.Cleanup(tp2Ref.Release)

	// Report the peer identities used by the in-process link.
	le1.Infof("constructed peer 1 with id %s", peerId1.String())
	le2.Infof("constructed peer 2 with id %s", peerId2.String())

	// Connect the two transports so both peers can accept streams.
	tp2.ConnectToInproc(ctx, tp1)
	tp1.ConnectToInproc(ctx, tp2)

	// Establish and retain a mounted link from the second peer to the first.
	lnk2to1, lnkRel, err := link.EstablishLinkWithPeerEx(ctx, tb2.Bus, "", peerId1, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(lnkRel)
	le1.Infof("opened link from 2 -> 1 with id %v", lnk2to1.GetLinkUUID())
	return lnk2to1
}

// TestEstablishLink tests creating a link with two in-memory nodes.
func TestEstablishLink(t *testing.T) {
	// Establish an in-process link for the reliable echo stream.
	ctx := t.Context()
	lnk2to1 := establishLink(ctx, t)

	// Open the echo protocol stream and close it after the test.
	ms1, err := lnk2to1.OpenMountedStream(ctx, stream_echo.DefaultProtocolID, stream.OpenOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ms1.GetStream().Close()

	// Send a proof payload over the reliable stream.
	data := []byte("testing 1234")
	_, err = ms1.GetStream().Write(data)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Require the echo stream to return the complete proof payload.
	outData := make([]byte, len(data)*2)
	on, oe := ms1.GetStream().Read(outData)
	if oe != nil {
		t.Fatal(oe.Error())
	}
	if on != len(data) {
		t.Fatalf("length incorrect received %v != %v", on, len(data))
	}
}

// TestUnreliableStream echoes messages over an unreliable mounted stream.
func TestUnreliableStream(t *testing.T) {
	// Establish an in-process link with a bounded unreliable echo test.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	lnk2to1 := establishLink(ctx, t)

	// Open a message stream for the echo protocol and retain it for the test.
	ms, err := lnk2to1.OpenMountedStream(ctx, stream_echo.DefaultProtocolID, stream.OpenOpts{Unreliable: true})
	if err != nil {
		t.Fatal(err.Error())
	}
	msgs, ok := ms.GetStream().(stream.MessageStream)
	if !ok {
		t.Fatalf("unreliable stream is %T", ms.GetStream())
	}
	defer msgs.Close()

	// Messages sent before the peer accepts the stream are dropped, so repeat
	// the message until its echo arrives.
	data := []byte("testing 1234")
	outData := make([]byte, 64)
	for {
		// Send the proof message while the remote peer accepts the stream.
		if _, err := msgs.Write(data); err != nil {
			t.Fatal(err.Error())
		}

		// Read the next echo within the message receive window.
		_ = msgs.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := msgs.Read(outData)

		// Verify the echoed message against the proof payload.
		if err == nil {
			if string(outData[:n]) != string(data) {
				t.Fatalf("echoed %q", outData[:n])
			}
			return
		}

		// Fail when the unreliable echo exceeds the test context deadline.
		if ctx.Err() != nil {
			t.Fatal("no echo before the deadline")
		}
	}
}
