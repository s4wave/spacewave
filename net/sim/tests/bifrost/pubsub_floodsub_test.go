//go:build bifrost_floodsub

package bifrost

import (
	"bytes"
	"context"
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
	ctx, ctxCancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer ctxCancel()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

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

	// replicate p2 -> [lan2] -> p1 -> [lan1] -> p0

	// Add pubsub configurations.
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

	sim := initSimulator(t, ctx, le, g)

	assertConnectivity := func(p0, p1 *graph.Peer) {
		px0 := sim.GetPeerByID(p0.GetPeerID())
		px1 := sim.GetPeerByID(p1.GetPeerID())
		if err := simulate.TestConnectivity(ctx, px0, px1); err != nil {
			t.Fatal(err.Error())
		}
		le.Infof(
			"successful connectivity test between %s and %s",
			p0.GetPeerID().String(),
			p1.GetPeerID().String(),
		)
	}
	assertConnectivity(p0, p1)
	assertConnectivity(p1, p2)

	// Attempt to open a channel and communicate.
	testingData := []byte("hello world")
	for _, channelID := range topics {
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
		s2 := tpv2.GetValue().(pubsub.BuildChannelSubscriptionValue)
		le.Infof("built channel subscription for channel %s on peer p2", channelID)

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
		s0 := tpv0.GetValue().(pubsub.BuildChannelSubscriptionValue)
		le.Infof("built channel subscription for channel %s on peer p0", channelID)

		msgRx := make(chan pubsub.Message, 1)
		s0.AddHandler(func(m pubsub.Message) {
			select {
			case msgRx <- m:
			default:
			}
		})

		// Wait for the relay's actual subscription announcement before publishing.
		var routers []*floodsub.FloodSub
		for _, b := range []bus.Bus{lp0tb.Bus, lp2tb.Bus} {
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
		testReplicate := func() {
			le.Infof("publishing data on p2 with peer %s", p2.GetPeerID().String())
			started := time.Now()
			if err := s2.Publish(testingData); err != nil {
				t.Fatal(err)
			}
			var rmsg pubsub.Message
			select {
			case rmsg = <-msgRx:
				t.Logf("two-hop publication latency: %s", time.Since(started))
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if !bytes.Equal(rmsg.GetData(), testingData) {
				t.Fatalf("pubsub data mismatch %v != expected %v", rmsg.GetData(), testingData)
			}
			le.Info("successful pubsub replication from p2 -> [lan2] -> p1 -> [lan1] -> p0 ")
		}
		testReplicate()

		if churn {
			for range 20 {
				for range 100 {
					sub, err := routers[1].AddSubscription(ctx, lp2tb.PrivKey, "transient-topic")
					if err != nil {
						t.Fatal(err)
					}
					sub.Release()
				}
				testReplicate()
			}
		} else {
			le.Info("interrupting connectivity between p2 and p1")
			for _, l := range lp2.GetTransportController().GetPeerLinks(p1.GetPeerID()) {
				le.Infof("closing link %v", l.GetUUID())
				l.Close()
			}
			assertConnectivity(p2, p1)
		}

		tpv0Ref.Release()
		tpv2Ref.Release()
	}

	le.Info("tests successful")
}
