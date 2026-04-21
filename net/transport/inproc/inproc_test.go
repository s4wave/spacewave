package inproc

import (
	"context"
	"testing"

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
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

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
	peerId, err := peer.IDFromPrivateKey(tb.PrivKey)
	if err != nil {
		t.Fatal(err.Error())
	}

	if conf == nil {
		conf = &Config{}
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
	return tpc1, tpt1.(*Inproc), tp1Ref
}

// TestEstablishLink tests creating a UDP link with two in-memory nodes.
func TestEstablishLink(t *testing.T) {
	ctx := t.Context()

	tb1, le1 := buildTestbed(t, ctx)
	le1 = le1.WithField("testbed", 0)
	tb2, le2 := buildTestbed(t, ctx)
	le2 = le2.WithField("testbed", 1)

	_, tp1, tp1Ref := execPeer(ctx, t, tb1, nil)
	peerId1 := tp1.GetPeerID()
	defer tp1Ref.Release()

	_, tp2, tp2Ref := execPeer(ctx, t, tb2, &Config{
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

	// Attempt to open a link between them.
	lnk2to1, lnk1Rel, err := link.EstablishLinkWithPeerEx(ctx, tb2.Bus, "", peerId1, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer lnk1Rel()

	le1.Infof(
		"opened link from 2 -> 1 with id %v",
		lnk2to1.GetLinkUUID(),
	)

	ms1, err := lnk2to1.OpenMountedStream(ctx, stream_echo.DefaultProtocolID, stream.OpenOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ms1.GetStream().Close()

	data := []byte("testing 1234")
	_, err = ms1.GetStream().Write(data)
	if err != nil {
		t.Fatal(err.Error())
	}
	outData := make([]byte, len(data)*2)
	on, oe := ms1.GetStream().Read(outData)
	if oe != nil {
		t.Fatal(oe.Error())
	}
	if on != len(data) {
		t.Fatalf("length incorrect received %v != %v", on, len(data))
	}
	outData = outData[:on]
	le1.Infof("echoed data successfully: %v", string(outData))
}
