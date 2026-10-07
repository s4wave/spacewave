//go:build bifrost_floodsub

package bifrost

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/net/pubsub"
	"github.com/s4wave/spacewave/net/pubsub/floodsub"
	floodsub_controller "github.com/s4wave/spacewave/net/pubsub/floodsub/controller"
	pubsub_relay "github.com/s4wave/spacewave/net/pubsub/relay"
	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/sim/simulate"
	"github.com/sirupsen/logrus"
)

// TestPubsubFloodsub performs a simple pubsub / floodsub test.
func TestPubsubFloodsub(t *testing.T) {
	testPubsubFloodsub(t, false)
}

// TestPubsubFloodsubChurn measures real two-hop delivery during local churn.
func TestPubsubFloodsubChurn(t *testing.T) {
	testPubsubFloodsub(t, true)
}

// testPubsubFloodsub uses the same three-peer topology for both scenarios.
func testPubsubFloodsub(t *testing.T, churn bool) {
	// Build three real peers joined by two local transport networks.
	ctx, ctxCancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer ctxCancel()

	// Count received subscription packets without logging transport traffic.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	log.SetOutput(io.Discard)
	counter := &subscriptionCounter{}
	log.AddHook(counter)
	le := logrus.NewEntry(log)

	// Connect the peers through two simulated LANs.
	g := graph.NewGraph()
	p0 := addPeer(t, g)
	p1 := addPeer(t, g)
	p2 := addPeer(t, g)
	lan1 := graph.AddLAN(g)
	lan1.AddPeer(g, p0)
	lan1.AddPeer(g, p1)
	lan2 := graph.AddLAN(g)
	lan2.AddPeer(g, p1)
	lan2.AddPeer(g, p2)

	// Relay the test channel from p2 through p1 to p0.
	topics := []string{"test-topic-1"}
	for _, peer := range g.AllPeers() {
		peer.AddFactory(func(b bus.Bus) controller.Factory { return floodsub_controller.NewFactory(b) })
		peer.AddConfig("pubsub", &floodsub_controller.Config{})
		peer.AddFactory(func(b bus.Bus) controller.Factory { return pubsub_relay.NewFactory(b) })
		peer.AddConfig("pubsub-relay", &pubsub_relay.Config{
			TopicIds: topics,
			PeerId:   peer.GetPeerID().String(),
		})
	}

	// Require transport connectivity before opening channel subscriptions.
	sim := initSimulator(t, ctx, le, g)
	assertConnectivity := func(p0, p1 *graph.Peer) {
		// Exercise each pair's actual transport connection.
		px0 := sim.GetPeerByID(p0.GetPeerID())
		px1 := sim.GetPeerByID(p1.GetPeerID())
		if err := simulate.TestConnectivity(ctx, px0, px1); err != nil {
			t.Fatal(err.Error())
		}

		// Record which transport connection was established.
		le.Infof(
			"successful connectivity test between %s and %s",
			p0.GetPeerID().String(),
			p1.GetPeerID().String(),
		)
	}
	assertConnectivity(p0, p1)
	assertConnectivity(p1, p2)

	// Publish fixed-size signed messages through the real two-hop pubsub path.
	testingData := make([]byte, 1024)
	for _, channelID := range topics {
		// Subscribe the publishing peer through its controller bus.
		lp2 := sim.GetPeerByID(p2.GetPeerID())
		lp2tb := lp2.GetTestbed()
		tpv2, _, tpv2Ref, err := bus.ExecOneOff(
			ctx,
			lp2tb.Bus,
			pubsub.NewBuildChannelSubscription(channelID, lp2tb.PrivKey),
			nil,
			nil,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer tpv2Ref.Release()
		s2 := tpv2.GetValue().(pubsub.BuildChannelSubscriptionValue)
		le.Infof("built channel subscription for channel %s on peer p2", channelID)

		// Subscribe the receiving peer through its controller bus.
		lp0 := sim.GetPeerByID(p0.GetPeerID())
		lp0tb := lp0.GetTestbed()
		tpv0, _, tpv0Ref, err := bus.ExecOneOff(
			ctx,
			lp0tb.Bus,
			pubsub.NewBuildChannelSubscription(channelID, lp0tb.PrivKey),
			nil,
			nil,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer tpv0Ref.Release()
		s0 := tpv0.GetValue().(pubsub.BuildChannelSubscriptionValue)
		le.Infof("built channel subscription for channel %s on peer p0", channelID)

		// Retain received messages without blocking the peer's read pump.
		msgRx := make(chan pubsub.Message, 32)
		s0.AddHandler(func(m pubsub.Message) {
			select {
			case msgRx <- m:
			default:
			}
		})

		// Wait for the relay's actual subscription announcement before publishing.
		var routers []*floodsub.FloodSub
		for _, b := range []bus.Bus{lp0tb.Bus, lp2tb.Bus} {
			// Find each attached pubsub router and wait for its relay announcement.
			ready := false
			for _, ctrl := range b.GetControllers() {
				if pubCtrl, ok := ctrl.(pubsub.Controller); ok {
					router, err := pubCtrl.GetPubSub(ctx)
					if err != nil {
						t.Fatal(err)
					}
					if err := router.(*floodsub.FloodSub).WaitForPeerSubscription(ctx, channelID, p1.GetPeerID()); err != nil {
						t.Fatal(err)
					}
					routers = append(routers, router.(*floodsub.FloodSub))
					ready = true
				}
			}
			if !ready {
				t.Fatal("missing pubsub controller")
			}
		}

		// Verify payload order while timing each publication from send to receipt.
		sequence := uint64(0)
		testReplicate := func() time.Duration {
			// Send a uniquely numbered payload of the fixed message size.
			binary.BigEndian.PutUint64(testingData, sequence)
			sequence++
			started := time.Now()
			if err := s2.Publish(testingData); err != nil {
				t.Fatal(err)
			}

			// Await the receiving handler and compare its complete payload.
			var rmsg pubsub.Message
			select {
			case rmsg = <-msgRx:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if !bytes.Equal(rmsg.GetData(), testingData) {
				t.Fatalf("pubsub data mismatch %v != expected %v", rmsg.GetData(), testingData)
			}
			return time.Since(started)
		}
		testReplicate()

		// Measure the same peers and payload during repeated subscription bursts.
		if churn {
			before := counter.Count()
			latencies := make([]time.Duration, 0, 20)
			for range 20 {
				// Add and release a hundred transient subscriptions before each send.
				for range 100 {
					sub, err := routers[1].AddSubscription(ctx, lp2tb.PrivKey, "transient-topic")
					if err != nil {
						t.Fatal(err)
					}
					sub.Release()
				}

				// Record the next ordered message through both real transports.
				latencies = append(latencies, testReplicate())
			}

			// Report packet counts and latency with fixed peers and message sizes.
			slices.Sort(latencies)
			var total time.Duration
			for _, latency := range latencies {
				total += latency
			}
			t.Logf("churn: peers=3 hops=2 payload=1024B bursts=20 refs/burst=100 mean=%s median=%s p95=%s max=%s subscription_packets=%d",
				total/time.Duration(len(latencies)), latencies[10], latencies[18], latencies[19], counter.Count()-before)

			// Require exactly one ordered delivery for each signed publication.
			select {
			case <-msgRx:
				t.Fatal("duplicate publication delivered")
			default:
			}
			continue
		}

		// Interrupt the relay link and verify that the transport reconnects.
		le.Info("interrupting connectivity between p2 and p1")
		for _, l := range lp2.GetTransportController().GetPeerLinks(p1.GetPeerID()) {
			le.Infof("closing link %v", l.GetUUID())
			l.Close()
		}
		assertConnectivity(p2, p1)
	}

	le.Info("tests successful")
}
