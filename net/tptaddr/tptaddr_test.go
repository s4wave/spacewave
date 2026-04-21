package tptaddr_test

import (
	"strings"
	"testing"

	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/testbed"
	tptaddr_controller "github.com/s4wave/spacewave/net/tptaddr/controller"
	tptaddr_static "github.com/s4wave/spacewave/net/tptaddr/static"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/sirupsen/logrus"
)

// TestTptAddr tests the tpt addr controllers end-to-end.
func TestTptAddr(t *testing.T) {
	ctx := t.Context()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// tb1: client
	tb1, err := testbed.NewTestbed(ctx, le.WithField("testbed", 1), testbed.TestbedOpts{NoEcho: true})
	if err != nil {
		t.Fatal(err.Error())
	}

	// tb2: server
	tb2, err := testbed.NewTestbed(ctx, le.WithField("testbed", 2), testbed.TestbedOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}

	tb1PeerID, err := peer.IDFromPrivateKey(tb1.PrivKey)
	if err != nil {
		t.Fatal(err.Error())
	}

	tb2PeerID, err := peer.IDFromPrivateKey(tb2.PrivKey)
	if err != nil {
		t.Fatal(err.Error())
	}

	// tb1 -> tb2 inproc
	tp2 := inproc.BuildInprocController(tb2.Logger, tb2.Bus, tb2PeerID, &inproc.Config{
		TransportPeerId: tb2PeerID.String(),
	})
	relTp2, err := tb2.Bus.AddController(ctx, tp2, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relTp2()

	// tb2 -> tb1 inproc
	tpt2dialer := &dialer.DialerOpts{
		Address: inproc.NewAddr(tb2PeerID).String(),
	}
	tp1 := inproc.BuildInprocController(tb1.Logger, tb1.Bus, tb1PeerID, &inproc.Config{
		TransportPeerId: tb1PeerID.String(),
		/* Using tptaddr instead.
		Dialers: map[string]*dialer.DialerOpts{
			tb2PeerID.String(): tpt2dialer,
		},
		*/
	})
	relTp1, err := tb1.Bus.AddController(ctx, tp1, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relTp1()

	// connect tb1 <-> tb2
	tpt2, _ := tp2.GetTransport(ctx)
	tpt1, _ := tp1.GetTransport(ctx)
	tpt1.(*inproc.Inproc).ConnectToInproc(ctx, tpt2.(*inproc.Inproc))
	tpt2.(*inproc.Inproc).ConnectToInproc(ctx, tpt1.(*inproc.Inproc))

	// register the tpt address
	tptaddrStatic, err := tptaddr_static.NewController(&tptaddr_static.Config{
		Addresses: []string{strings.Join([]string{tb2PeerID.String(), "inproc", tpt2dialer.GetAddress()}, "|")},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	tptaddrStaticRel, err := tb1.Bus.AddController(ctx, tptaddrStatic, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tptaddrStaticRel()

	// register the tpt address dialer controller
	tptAddrCtrl := tptaddr_controller.NewController(le, tb1.Bus, &tptaddr_controller.Config{})
	relTptAddrCtrl, err := tb1.Bus.AddController(ctx, tptAddrCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relTptAddrCtrl()

	// attempt to dial tb2, which should create a LookupTptAddr and a DialTptAddr directive.
	lnk, lnkRel, err := link.EstablishLinkWithPeerEx(ctx, tb1.Bus, tb1.PeerID, tb2.PeerID, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Infof("successfully opened link with uuid %v using tptaddr", lnk.GetLinkUUID())
	lnkRel()
}
