package dex_solicit

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	bifrost_core "github.com/s4wave/spacewave/net/core"
	link_holdopen_controller "github.com/s4wave/spacewave/net/link/hold-open"
	link_solicit_controller "github.com/s4wave/spacewave/net/link/solicit/controller"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/sirupsen/logrus"
)

// TestThreeNodeRelayRequiresOneForwardHop proves a cold immediate relay can
// reach a writer only when the DEX forwarding budget permits one hop, and that
// the relayed block carries the writer's recorded refs.
func TestThreeNodeRelayRequiresOneForwardHop(t *testing.T) {
	for _, tc := range []struct {
		name  string
		hops  uint32
		found bool
	}{
		{name: "zero hops", hops: 0},
		{name: "one hop", hops: 1, found: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testThreeNodeRelay(t, tc.hops, tc.found)
		})
	}
}

// testThreeNodeRelay exercises one forwarding budget against a real three-node
// star with distinct local buckets.
func testThreeNodeRelay(t *testing.T, hops uint32, wantFound bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	le := logrus.NewEntry(logrus.New())
	protocolContext := []byte("shared-object")

	recipient := newTestRelayNode(t, ctx, le, "recipient", protocolContext, hops)
	relay := newTestRelayNode(t, ctx, le, "relay", protocolContext, hops)
	writer := newTestRelayNode(t, ctx, le, "writer", protocolContext, hops)

	// Connect only recipient-relay and relay-writer so the recipient cannot
	// request the writer directly.
	connectTestRelayNodes(t, ctx, recipient, relay)
	connectTestRelayNodes(t, ctx, writer, relay)
	waitTestDexCondition(t, "recipient-relay-writer DEX sessions", func() bool {
		return len(recipient.dex.snapshotSessions()) == 1 &&
			len(relay.dex.snapshotSessions()) == 2 &&
			len(writer.dex.snapshotSessions()) == 1
	})

	// Store the block and its child only in the writer's distinct local
	// volume and bucket.
	data := []byte("writer-only block")
	writerBucket, _, writerBucketRef, err := bucket.ExBuildBucketAPI(
		ctx,
		writer.tb.Bus,
		false,
		writer.bucketID,
		writer.tb.Volume.GetID(),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(writerBucketRef.Release)
	child, _, err := writerBucket.GetBucket().PutBlock(ctx, []byte("writer-only child"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ref, _, err := writerBucket.GetBucket().PutBlock(ctx, data, &block.PutOpts{
		Refs: []*block.BlockRef{child},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Prove the relay is cold without reading through its DEX lookup.
	relayFound, err := relay.tb.Volume.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if relayFound {
		t.Fatal("relay volume contained writer block before recipient read")
	}

	// Read through the recipient's DEX view without pre-reading the relay.
	stored, err := NewStore(recipient.dex).GetStoredBlock(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if found := stored != nil; found != wantFound {
		t.Fatalf("relay lookup found = %t, want %t", found, wantFound)
	}
	if !wantFound {
		return
	}
	if !bytes.Equal(stored.Data, data) {
		t.Fatalf("relay lookup data = %q, want %q", stored.Data, data)
	}
	if !stored.RefsKnown || len(stored.Refs) != 1 || !stored.Refs[0].EqualsRef(child) {
		t.Fatalf("relay lookup refs = %v known %v, want [%v]", stored.Refs, stored.RefsKnown, child)
	}
}

// testRelayNode holds one real DEX test node and its distinct local bucket.
type testRelayNode struct {
	// tb owns the node's bus and local volume.
	tb *testbed.Testbed
	// transport connects the node to its immediate star neighbors.
	transport *transport_controller.Controller
	// dex exchanges blocks within protocolContext.
	dex *Controller
	// bucketID identifies the node's distinct local bucket.
	bucketID string
}

// newTestRelayNode constructs one DEX node with real local storage and inproc
// transport for a shared protocol context.
func newTestRelayNode(
	t *testing.T,
	ctx context.Context,
	le *logrus.Entry,
	name string,
	protocolContext []byte,
	hops uint32,
) *testRelayNode {
	t.Helper()

	// Construct one real bus, volume, transport, solicitation controller, and
	// account-specific DEX bucket.
	tb, err := testbed.NewTestbed(ctx, le.WithField("node", name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	bifrost_core.AddFactories(tb.Bus, tb.StaticResolver)
	tb.StaticResolver.AddFactory(link_solicit_controller.NewFactory())
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	_, _, _, err = tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  "relay-test-" + name,
		Rev: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	transport, _, transportRef, err := loader.WaitExecControllerRunningTyped[*transport_controller.Controller](
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(&inproc.Config{}),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(transportRef.Release)

	_, _, holdOpenRef, err := loader.WaitExecControllerRunning(
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(&link_holdopen_controller.Config{}),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(holdOpenRef.Release)

	_, _, solicitRef, err := loader.WaitExecControllerRunning(
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(&link_solicit_controller.Config{}),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(solicitRef.Release)

	bucketID := "relay-test-" + name
	dexController, _, dexRef, err := loader.WaitExecControllerRunningTyped[*Controller](
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(&Config{
			BucketId:        bucketID,
			MaxForwardHops:  hops,
			ProtocolContext: protocolContext,
		}),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dexRef.Release)

	return &testRelayNode{
		tb:        tb,
		transport: transport,
		dex:       dexController,
		bucketID:  bucketID,
	}
}

// connectTestRelayNodes creates and dials one direct inproc star edge.
func connectTestRelayNodes(t *testing.T, ctx context.Context, from, to *testRelayNode) {
	t.Helper()

	fromTransport, err := from.transport.GetTransport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	toTransport, err := to.transport.GetTransport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fromInproc := fromTransport.(*inproc.Inproc)
	toInproc := toTransport.(*inproc.Inproc)
	fromInproc.ConnectToInproc(ctx, toInproc)
	toInproc.ConnectToInproc(ctx, fromInproc)

	if _, err := from.transport.DialPeerAddr(ctx, toInproc.GetPeerID(), &dialer.DialerOpts{
		Address: toInproc.LocalAddr().String(),
	}); err != nil {
		t.Fatal(err)
	}
}
