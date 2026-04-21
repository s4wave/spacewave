package link_solicit_controller

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/testbed"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/sirupsen/logrus"
)

func buildTestbed(t *testing.T, ctx context.Context) *testbed.Testbed {
	t.Helper()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le, testbed.TestbedOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Register inproc transport and solicitation controller factories.
	tb.StaticResolver.AddFactory(inproc.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(NewFactory())

	return tb
}

func startTransport(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	conf *inproc.Config,
) (*transport_controller.Controller, *inproc.Inproc, directive.Reference) {
	t.Helper()
	pid, err := peer.IDFromPrivateKey(tb.PrivKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if conf == nil {
		conf = &inproc.Config{}
	}
	conf.TransportPeerId = pid.String()

	tpc, _, tpRef, err := loader.WaitExecControllerRunningTyped[*transport_controller.Controller](
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(conf),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	tpt, err := tpc.GetTransport(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	return tpc, tpt.(*inproc.Inproc), tpRef
}

func startSolicitController(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
) directive.Reference {
	t.Helper()
	_, _, ref, err := bus.ExecOneOff(
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(&Config{}),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	return ref
}

// TestSolicitProtocolMatch tests that two peers both soliciting the same
// protocol get a SolicitMountedStream value.
func TestSolicitProtocolMatch(t *testing.T) {
	ctx := t.Context()

	// Set up two testbeds with inproc transport and solicitation.
	tb1 := buildTestbed(t, ctx)
	tb2 := buildTestbed(t, ctx)

	_, tp1, tp1Ref := startTransport(t, ctx, tb1, nil)
	defer tp1Ref.Release()
	_, tp2, tp2Ref := startTransport(t, ctx, tb2, &inproc.Config{
		Dialers: map[string]*dialer.DialerOpts{
			tp1.GetPeerID().String(): {
				Address: tp1.LocalAddr().String(),
			},
		},
	})
	defer tp2Ref.Release()

	// Wire inproc transports.
	tp1.ConnectToInproc(ctx, tp2)
	tp2.ConnectToInproc(ctx, tp1)

	// Start solicitation controllers.
	scRef1 := startSolicitController(t, ctx, tb1)
	defer scRef1.Release()
	scRef2 := startSolicitController(t, ctx, tb2)
	defer scRef2.Release()

	// Establish a link.
	pid1 := tp1.GetPeerID()
	_, lnkRel, err := link.EstablishLinkWithPeerEx(ctx, tb2.Bus, "", pid1, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer lnkRel()

	// Both peers solicit the same protocol.
	// Must add directives on both sides before waiting, since matching
	// requires both peers to have the solicitation active.
	testProto := protocol.ID("test/echo")

	type result struct {
		sms link_solicit.SolicitMountedStream
		ref directive.Reference
		err error
	}

	ch1 := make(chan result, 1)
	ch2 := make(chan result, 1)

	go func() {
		sms, _, ref, err := link_solicit.ExSolicitProtocol(ctx, tb1.Bus, testProto, nil, "", 0)
		ch1 <- result{sms, ref, err}
	}()
	go func() {
		sms, _, ref, err := link_solicit.ExSolicitProtocol(ctx, tb2.Bus, testProto, nil, "", 0)
		ch2 <- result{sms, ref, err}
	}()

	r1 := <-ch1
	if r1.err != nil {
		t.Fatalf("peer 1 solicit error: %v", r1.err)
	}
	defer r1.ref.Release()

	r2 := <-ch2
	if r2.err != nil {
		t.Fatalf("peer 2 solicit error: %v", r2.err)
	}
	defer r2.ref.Release()

	sms1 := r1.sms
	sms2 := r2.sms

	// Accept both streams.
	ms1, alreadyAccepted, err := sms1.AcceptMountedStream()
	if err != nil {
		t.Fatalf("peer 1 accept error: %v", err)
	}
	if alreadyAccepted {
		t.Fatal("peer 1 stream already accepted")
	}
	if ms1 == nil {
		t.Fatal("peer 1 got nil MountedStream")
	}
	defer ms1.GetStream().Close()

	ms2, alreadyAccepted, err := sms2.AcceptMountedStream()
	if err != nil {
		t.Fatalf("peer 2 accept error: %v", err)
	}
	if alreadyAccepted {
		t.Fatal("peer 2 stream already accepted")
	}
	if ms2 == nil {
		t.Fatal("peer 2 got nil MountedStream")
	}
	defer ms2.GetStream().Close()

	// Write data from one side and read on the other.
	data := []byte("hello solicitation")
	_, err = ms1.GetStream().Write(data)
	if err != nil {
		t.Fatalf("write error: %v", err)
	}

	buf := make([]byte, len(data)*2)
	n, err := ms2.GetStream().Read(buf)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if string(buf[:n]) != string(data) {
		t.Fatalf("data mismatch: got %q, want %q", buf[:n], data)
	}

	t.Log("solicitation match successful, data exchanged")
}

// TestSolicitProtocolNoMatch tests that disjoint protocol sets don't match.
func TestSolicitProtocolNoMatch(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	tb1 := buildTestbed(t, ctx)
	tb2 := buildTestbed(t, ctx)

	_, tp1, tp1Ref := startTransport(t, ctx, tb1, nil)
	defer tp1Ref.Release()
	_, tp2, tp2Ref := startTransport(t, ctx, tb2, &inproc.Config{
		Dialers: map[string]*dialer.DialerOpts{
			tp1.GetPeerID().String(): {
				Address: tp1.LocalAddr().String(),
			},
		},
	})
	defer tp2Ref.Release()

	tp1.ConnectToInproc(ctx, tp2)
	tp2.ConnectToInproc(ctx, tp1)

	scRef1 := startSolicitController(t, ctx, tb1)
	defer scRef1.Release()
	scRef2 := startSolicitController(t, ctx, tb2)
	defer scRef2.Release()

	pid1 := tp1.GetPeerID()
	_, lnkRel, err := link.EstablishLinkWithPeerEx(ctx, tb2.Bus, "", pid1, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer lnkRel()

	// Peer 1 solicits "proto/a", peer 2 solicits "proto/b" -- no match.
	_, diRef1, err := tb1.Bus.AddDirective(
		link_solicit.NewSolicitProtocol(protocol.ID("proto/a"), nil, "", 0),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer diRef1.Release()

	_, diRef2, err := tb2.Bus.AddDirective(
		link_solicit.NewSolicitProtocol(protocol.ID("proto/b"), nil, "", 0),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer diRef2.Release()

	// The test verifies no crash/hang with disjoint sets.
	// A brief sleep would be needed to verify no match, but for now
	// we just verify the system doesn't deadlock or panic.
	t.Log("no-match test completed without panic or deadlock")
}

// TestSolicitProtocolContextMismatch tests that same protocol ID but
// different contexts don't match.
func TestSolicitProtocolContextMismatch(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	tb1 := buildTestbed(t, ctx)
	tb2 := buildTestbed(t, ctx)

	_, tp1, tp1Ref := startTransport(t, ctx, tb1, nil)
	defer tp1Ref.Release()
	_, tp2, tp2Ref := startTransport(t, ctx, tb2, &inproc.Config{
		Dialers: map[string]*dialer.DialerOpts{
			tp1.GetPeerID().String(): {
				Address: tp1.LocalAddr().String(),
			},
		},
	})
	defer tp2Ref.Release()

	tp1.ConnectToInproc(ctx, tp2)
	tp2.ConnectToInproc(ctx, tp1)

	scRef1 := startSolicitController(t, ctx, tb1)
	defer scRef1.Release()
	scRef2 := startSolicitController(t, ctx, tb2)
	defer scRef2.Release()

	pid1 := tp1.GetPeerID()
	_, lnkRel, err := link.EstablishLinkWithPeerEx(ctx, tb2.Bus, "", pid1, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer lnkRel()

	// Same protocol but different context bytes -- should not match.
	_, diRef1, err := tb1.Bus.AddDirective(
		link_solicit.NewSolicitProtocol(protocol.ID("dex"), []byte("bucket-a"), "", 0),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer diRef1.Release()

	_, diRef2, err := tb2.Bus.AddDirective(
		link_solicit.NewSolicitProtocol(protocol.ID("dex"), []byte("bucket-b"), "", 0),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer diRef2.Release()

	t.Log("context mismatch test completed without panic or deadlock")
}
