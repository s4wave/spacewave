package e2e

import (
	"bytes"
	"context"
	"testing"

	bifrost_core "github.com/s4wave/spacewave/net/core"
	egctr "github.com/s4wave/spacewave/net/entitygraph"
	link_holdopen_controller "github.com/s4wave/spacewave/net/link/hold-open"
	floodsub_controller "github.com/s4wave/spacewave/net/pubsub/floodsub/controller"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	csp "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	entitygraph_logger "github.com/aperturerobotics/entitygraph/logger"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	transform_chksum "github.com/s4wave/spacewave/db/block/transform/chksum"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	lc "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// PrepareTestbedFunc prepares a testbed and returns some configs to start.
type PrepareTestbedFunc func(t *testbed.Testbed, bc *bucket.Config) ([]config.Config, error)

// PrepareBucketConfigFunc prepares the bucket configuration
type PrepareBucketConfigFunc func(bc *bucket.Config) error

// TestMultiNodeDEX tests a multi-node data exchange.
func TestMultiNodeDEX(
	t *testing.T,
	prepareBcCb PrepareBucketConfigFunc,
	prepareTestbedCb PrepareTestbedFunc,
) {
	subCtx, subCtxCancel := context.WithCancel(context.Background())
	defer subCtxCancel()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	transformSet := transform_all.BuildFactorySet()
	tconf, err := block_transform.NewConfig([]config.Config{
		&transform_chksum.Config{},
		&transform_s2.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	nnodes := 3
	var testbeds []*testbed.Testbed
	var bridges []*inproc.Inproc
	var tptControllers []*transport_controller.Controller

	// 1. Create a testbed for each node.
	t.Log("constructing testbeds")
	for i := range nnodes {
		tb, err := testbed.NewTestbed(
			ctx,
			le.WithField("testbed-i", i),
		)
		if err != nil {
			t.Fatal(err.Error())
		}

		bifrost_core.AddFactories(tb.Bus, tb.StaticResolver)
		conf := &inproc.Config{} // defaults
		dv, _, dvRef, err := loader.WaitExecControllerRunning(
			ctx,
			tb.Bus,
			resolver.NewLoadControllerWithConfig(conf),
			nil,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		tptc := dv.(*transport_controller.Controller)
		tpt, err := tptc.GetTransport(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		bridges = append(bridges, tpt.(*inproc.Inproc))
		testbeds = append(testbeds, tb)
		tptControllers = append(tptControllers, tptc)
		defer dvRef.Release()
	}

	// 2. Create connections
	// Connect 0 -> 1, 1 -> 2, etc.
	t.Log("building connections")
	for i := 0; i < nnodes-1; i++ {
		t.Logf(
			"connecting %s <-> %s",
			bridges[i].LocalAddr().String(),
			bridges[i+1].LocalAddr().String(),
		)
		bridges[i].ConnectToInproc(ctx, bridges[i+1])
		bridges[i+1].ConnectToInproc(ctx, bridges[i])
	}

	// HACK
	// TODO
	{
		bridges[0].ConnectToInproc(ctx, bridges[len(bridges)-1])
		bridges[len(bridges)-1].ConnectToInproc(ctx, bridges[0])
		if _, err := tptControllers[0].DialPeerAddr(
			ctx,
			bridges[len(bridges)-1].GetPeerID(),
			&dialer.DialerOpts{
				Address: bridges[len(bridges)-1].LocalAddr().String(),
			},
		); err != nil {
			t.Fatal(err.Error())
		}
	}

	t.Log("executing inter-node dials")
	for i := 0; i < nnodes-1; i++ {
		t.Logf(
			"dialing %s -> %s",
			bridges[i].LocalAddr().String(),
			bridges[i+1].LocalAddr().String(),
		)
		if _, err := tptControllers[i].DialPeerAddr(
			ctx,
			bridges[i+1].GetPeerID(),
			&dialer.DialerOpts{
				Address: bridges[i+1].LocalAddr().String(),
			},
		); err != nil {
			t.Fatal(err.Error())
		}
	}

	lookupConf := &lc.Config{
		NotFoundBehavior: lc.NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE,
		PutBlockBehavior: lc.PutBlockBehavior_PutBlockBehavior_ALL,
	}
	cc, err := csp.NewControllerConfig(configset.NewControllerConfig(1, lookupConf), false)
	if err != nil {
		t.Fatal(err.Error())
	}
	bc := &bucket.Config{
		Id:  "test-bucket-dex",
		Rev: 1,
		Lookup: &bucket.LookupConfig{
			Controller: cc,
		},
	}
	if prepareBcCb != nil {
		if err := prepareBcCb(bc); err != nil {
			t.Fatal(err.Error())
		}
	}

	// 3. Negotiation + handshaking will occur.
	// Setup controllers for communication + storage transfer.
	for _, tb := range testbeds {
		addlControllers := []config.Config{
			&floodsub_controller.Config{},
			&node_controller.Config{},
			&link_holdopen_controller.Config{},
			&egctr.Config{},
		}
		if prepareTestbedCb != nil {
			ac, err := prepareTestbedCb(tb, bc)
			if err != nil {
				t.Fatal(err.Error())
			}
			addlControllers = append(addlControllers, ac...)
		}
		tb.StaticResolver.AddFactory(egctr.NewFactory(tb.Bus))
		for _, c := range addlControllers {
			_, _, dvRef, err := bus.ExecOneOff(
				ctx,
				tb.Bus,
				resolver.NewLoadControllerWithConfig(c),
				nil,
				nil,
			)
			if err != nil {
				t.Fatal(err.Error())
			}
			defer dvRef.Release()
		}

		_, err := entitygraph_logger.AttachBasicLogger(tb.Bus, le)
		if err != nil {
			t.Fatalf("start entitygraph logger: %v", err)
		}
	}

	for _, tbb := range testbeds {
		// apply bucket config
		_, _, bcRef, err := bus.ExecOneOff(
			subCtx,
			tbb.Bus,
			bucket.NewApplyBucketConfigToVolume(
				bc,
				tbb.Volume.GetID(),
			), nil, nil,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		bcRef.Release()
	}

	// data to transport
	dataXfer := []byte("hello world")

	// get bucket handle
	var dataXferRef *block.BlockRef
	{
		rootCursor, _, err := bucket_lookup.BuildEmptyCursor(
			subCtx,
			testbeds[0].Bus,
			le,
			transformSet,
			bc.GetId(),
			testbeds[0].Volume.GetID(),
			tconf,
			nil,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		dataXferRef, _, err = rootCursor.PutBlock(ctx, dataXfer, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		rootCursor.Release()
	}

	t.Logf(
		"placed block in first bucket with ref %s",
		dataXferRef.MarshalString(),
	)

	// request block from third peer
	{
		rootCursor, _, err := bucket_lookup.BuildEmptyCursor(
			subCtx,
			testbeds[2].Bus,
			le,
			transformSet,
			bc.GetId(),
			testbeds[2].Volume.GetID(),
			tconf,
			nil,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		lkDat, lkOk, err := rootCursor.GetBlock(ctx, dataXferRef)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !lkOk {
			t.Fatal("lookup on node 3 returned ok=false")
		}
		if len(lkDat) != len(dataXfer) || !bytes.Equal(lkDat, dataXfer) {
			t.Fatalf("data mismatch %v != %v (expected)", lkDat, dataXfer)
		}
		rootCursor.Release()
	}

	t.Log("data replicated successfully, checking")
	{
		targetVolID := testbeds[2].Volume.GetID()
		targetBus := testbeds[2].Bus
		bav, _, avRel, err := bucket.ExBuildBucketAPI(subCtx, targetBus, false, bc.GetId(), targetVolID, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		dat, datOk, err := bav.GetBucket().GetBlock(ctx, dataXferRef)
		if err != nil {
			avRel.Release()
			t.Fatal(err.Error())
		}
		if !datOk {
			avRel.Release()
			t.Fatal("volume lookup on node 3 returned ok=false")
		}
		_ = dat // encrypted here
	}
}
