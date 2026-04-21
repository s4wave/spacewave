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
	floodsub_controller "github.com/s4wave/spacewave/net/pubsub/floodsub/controller"
	pubsub_relay "github.com/s4wave/spacewave/net/pubsub/relay"
	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/sim/simulate"
	"github.com/sirupsen/logrus"
)

// TestPubsubFloodsub performs a simple pubsub / floodsub test.
func TestPubsubFloodsub(t *testing.T) {
	ctx, ctxCancel := context.WithCancel(context.Background())
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

		// TODO: remove this delay... needs a little time to "settle"
		<-time.After(time.Millisecond * 100)
		testReplicate := func() {
			le.Infof("publishing data on p2 with peer %s", p2.GetPeerID().String())
			s2.Publish(testingData)
			rmsg := <-msgRx
			if bytes.Compare(rmsg.GetData(), testingData) != 0 {
				t.Fatalf("pubsub data mismatch %v != expected %v", rmsg.GetData(), testingData)
			}
			le.Info("successful pubsub replication from p2 -> [lan2] -> p1 -> [lan1] -> p0 ")
		}
		testReplicate()

		le.Info("interrupting connectivity between p2 and p1")
		for _, l := range lp2.GetTransportController().GetPeerLinks(p1.GetPeerID()) {
			le.Infof("closing link %v", l.GetUUID())
			l.Close()
		}

		// re-connect
		le.Info("expecting re-connect between peers")
		assertConnectivity(p2, p1)

		tpv0Ref.Release()
		tpv2Ref.Release()
	}

	le.Info("tests successful")
}
