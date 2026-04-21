package link_holdopen_controller

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
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
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/sirupsen/logrus"
)

func buildTestbed(t *testing.T, ctx context.Context) (*testbed.Testbed, *logrus.Entry) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le, testbed.TestbedOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(inproc.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))
	return tb, le
}

func execPeer(ctx context.Context, t *testing.T, tb *testbed.Testbed, conf *inproc.Config) (
	*transport_controller.Controller,
	*inproc.Inproc,
	directive.Reference,
) {
	peerId, err := peer.IDFromPrivateKey(tb.PrivKey)
	if err != nil {
		t.Fatal(err.Error())
	}

	if conf == nil {
		conf = &inproc.Config{}
	}
	conf.TransportPeerId = peerId.String()

	tpc1, _, tp1Ref, err := loader.WaitExecControllerRunningTyped[*transport_controller.Controller](
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(conf),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	tpt1, err := tpc1.GetTransport(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	return tpc1, tpt1.(*inproc.Inproc), tp1Ref
}

// TestHoldOpenWithMountedLink tests that the hold-open controller correctly
// handles MountedLink values from EstablishLinkWithPeer.
//
// This verifies the fix for issue #276 where the type assertion for link.Link
// failed because EstablishLinkWithPeerValue was changed to MountedLink.
func TestHoldOpenWithMountedLink(t *testing.T) {
	ctx := t.Context()

	tb1, le1 := buildTestbed(t, ctx)
	le1 = le1.WithField("testbed", 0)
	tb2, le2 := buildTestbed(t, ctx)
	le2 = le2.WithField("testbed", 1)

	_, tp1, tp1Ref := execPeer(ctx, t, tb1, nil)
	peerId1 := tp1.GetPeerID()
	defer tp1Ref.Release()

	_, tp2, tp2Ref := execPeer(ctx, t, tb2, &inproc.Config{
		Dialers: map[string]*dialer.DialerOpts{
			peerId1.String(): {
				Address: tp1.LocalAddr().String(),
			},
		},
	})
	peerId2 := tp2.GetPeerID()
	defer tp2Ref.Release()

	le1.Infof("constructed peer 1 with id %s", peerId1.String())
	le2.Infof("constructed peer 2 with id %s", peerId2.String())

	tp2.ConnectToInproc(ctx, tp1)
	tp1.ConnectToInproc(ctx, tp2)

	// Start the hold-open controller on tb2
	_, _, holdOpenRef, err := bus.ExecOneOff(
		ctx,
		tb2.Bus,
		resolver.NewLoadControllerWithConfig(&Config{}),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer holdOpenRef.Release()

	// Establish a link - the hold-open controller should handle the MountedLink
	lnk, lnkRel, err := link.EstablishLinkWithPeerEx(ctx, tb2.Bus, "", peerId1, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer lnkRel()

	le2.Infof("opened link from 2 -> 1 with uuid %v", lnk.GetLinkUUID())

	// Verify the link works by using the echo stream
	ms, err := lnk.OpenMountedStream(ctx, stream_echo.DefaultProtocolID, stream.OpenOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ms.GetStream().Close()

	data := []byte("hold-open test")
	_, err = ms.GetStream().Write(data)
	if err != nil {
		t.Fatal(err.Error())
	}

	outData := make([]byte, len(data)*2)
	n, err := ms.GetStream().Read(outData)
	if err != nil {
		t.Fatal(err.Error())
	}
	if n != len(data) {
		t.Fatalf("expected %d bytes, got %d", len(data), n)
	}

	le2.Infof("echoed data successfully: %s", string(outData[:n]))
}
