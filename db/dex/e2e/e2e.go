package e2e

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	csp "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	lc "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/db/testbed"
	bifrost_core "github.com/s4wave/spacewave/net/core"
	link_holdopen_controller "github.com/s4wave/spacewave/net/link/hold-open"
	floodsub_controller "github.com/s4wave/spacewave/net/pubsub/floodsub/controller"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/sirupsen/logrus"
)

// PrepareTestbedFunc prepares a testbed and returns some configs to start.
type PrepareTestbedFunc func(t *testbed.Testbed, bc *bucket.Config) ([]config.Config, error)

// PrepareBucketConfigFunc prepares the bucket configuration.
type PrepareBucketConfigFunc func(bc *bucket.Config) error

// Retention is what the reading node's Volume keeps of a block it read from a
// peer and decoded. A Volume does not trust the references a peer lists, so
// it keeps the block with the references its reader decodes.
type Retention int

const (
	// RetainAny accepts either outcome.
	RetainAny Retention = iota
	// RetainStored requires the block stored, as a bucket lookup that writes
	// back keeps a block once its reader decodes it.
	RetainStored
	// RetainNone requires the block not stored, as a bucket lookup that does
	// not write back serves it and stores nothing.
	RetainNone
)

// TestMultiNodeDEX tests a multi-node data exchange whose reading node keeps
// what retain requires of the block it read.
func TestMultiNodeDEX(
	t *testing.T,
	prepareBcCb PrepareBucketConfigFunc,
	prepareTestbedCb PrepareTestbedFunc,
	retain Retention,
) {
	// Bound the three-node exchange and its cleanup lifetime.
	ctx, ctxCancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer ctxCancel()

	// Scope storage operations to the exchange's cancellable sub-context.
	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	// Configure debug logging shared by each testbed.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Build the available transforms and select gzip for stored blocks.
	transformSet := transform_all.BuildFactorySet()
	tconf, err := block_transform.NewConfig([]config.Config{
		&transform_gzip.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Reserve state for the three participating nodes and their transports.
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

	// Link the first node directly to the last: the callers' lookups between
	// the two ends need it, and with only the chain of adjacent links the
	// solicit and psecho DEX tests fail.
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

	// Dial each adjacent peer pair to establish the exchange links.
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

	// Configure lookup and writeback behavior for the shared test bucket.
	lookupConf := &lc.Config{
		NotFoundBehavior:  lc.NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE_WAIT,
		PutBlockBehavior:  lc.PutBlockBehavior_PutBlockBehavior_ALL,
		WritebackBehavior: lc.WritebackBehavior_WritebackBehavior_ALL,
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
		}
		if prepareTestbedCb != nil {
			ac, err := prepareTestbedCb(tb, bc)
			if err != nil {
				t.Fatal(err.Error())
			}
			addlControllers = append(addlControllers, ac...)
		}
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
	}

	// Apply the shared bucket configuration to each node.
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
	dataUntouched := []byte("untouched")

	// get bucket handle
	var dataXferRef, untouchedRef *block.BlockRef
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
		dataXferRef, _, err = rootCursor.PutBlock(ctx, dataXfer, &block.PutOpts{RefsKnown: true})
		if err != nil {
			rootCursor.Release()
			t.Fatal(err.Error())
		}
		untouchedRef, _, err = rootCursor.PutBlock(ctx, dataUntouched, &block.PutOpts{RefsKnown: true})
		rootCursor.Release()
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// Record the source reference before requesting it from another node.
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
		_, bcs := rootCursor.BuildTransactionAtRef(nil, dataXferRef)
		lkDat, lkOk, err := bcs.FetchLeaf(ctx)
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

	// Confirm what the destination Volume kept through its bucket API.
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
		_ = dat // encrypted here
		if retain == RetainStored && !datOk {
			avRel.Release()
			t.Fatal("volume lookup on node 3 returned ok=false")
		}
		if retain == RetainNone && datOk {
			avRel.Release()
			t.Fatal("node 3 stored a block it read without its references")
		}
		untouchedOK, err := bav.GetBucket().GetBlockExists(ctx, untouchedRef)
		if err != nil {
			avRel.Release()
			t.Fatal(err.Error())
		} else if untouchedOK {
			avRel.Release()
			t.Fatal("untouched block present in local storage after requested read")
		}
		avRel.Release()
	}

	// An exchange that carries refs answers a subtree read with the blocks
	// under the root too.
	if retain == RetainStored {
		checkSubtreeWriteback(subCtx, t, le, testbeds[0], testbeds[2], bc.GetId())
	}
}

// checkSubtreeWriteback writes a small tree, with each block's refs, to the
// bucket on src, and walks it on dst: it reads the root through the bucket
// lookup as a subtree read, then the levels under it without the network. It
// checks that the peer's answer to the root carried those levels, and that
// dst's Volume holds every block once the walk decoded it.
func checkSubtreeWriteback(ctx context.Context, t *testing.T, le *logrus.Entry, src, dst *testbed.Testbed, bucketID string) {
	// Build the tree: the root references a and b, and a references c.
	t.Helper()
	var entries []*block.PutBatchEntry
	put := func(data string, refs ...*block.BlockRef) *block.BlockRef {
		// Name the block by its data and queue it with its refs.
		ref, err := block.BuildBlockRef([]byte(data), nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: []byte(data), Refs: refs, RefsKnown: true})
		return ref
	}
	c := put("subtree leaf c")
	a := put("subtree node a", c)
	b := put("subtree leaf b")
	root := put("subtree root", a, b)

	// Write it to the bucket on src.
	srcAPI, _, srcRel, err := bucket.ExBuildBucketAPI(ctx, src.Bus, false, bucketID, src.Volume.GetID(), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, err = srcAPI.GetBucket().PutBlockBatch(ctx, entries)
	srcRel.Release()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Read a block on dst through cur and decode it: the tree's blocks are
	// raw bytes, so the test names the refs a decoder would find.
	read := func(ctx context.Context, cur *bucket_lookup.Cursor, name string, ref *block.BlockRef, refs ...*block.BlockRef) {
		// Fetch the bytes as a reader that decodes them itself.
		t.Helper()
		_, bcs := cur.BuildTransactionAtRef(nil, ref)
		data, keep, found, err := bcs.FetchWithKeep(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !found || string(data) != name {
			t.Fatalf("read of %q returned %q, found %v", name, data, found)
		}

		// Hand the store the decoded refs when it asked to keep the block.
		if keep != nil {
			keep(ctx, refs)
		}
	}

	// Read the root on dst as a subtree read.
	cur, _, err := bucket_lookup.BuildEmptyCursor(ctx, dst.Bus, le, nil, bucketID, dst.Volume.GetID(), nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer cur.Release()
	read(block.WithSubtreeRead(ctx), cur, "subtree root", root, a, b)

	// Read the levels under it without the network. The Volume drops the fill
	// of a block whose refs a peer only listed, so the lookup holds the blocks
	// the peer sent with the root until the walk reads them.
	local := cur.CloneWithLocalOnlyReads()
	defer local.Release()
	read(ctx, local, "subtree node a", a, c)
	read(ctx, local, "subtree leaf b", b)
	read(ctx, local, "subtree leaf c", c)

	// Check that dst's Volume holds every block of the tree.
	dstAPI, _, dstRel, err := bucket.ExBuildBucketAPI(ctx, dst.Bus, false, bucketID, dst.Volume.GetID(), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer dstRel.Release()
	held, err := dstAPI.GetBucket().GetBlockExistsBatch(ctx, []*block.BlockRef{root, a, b, c})
	if err != nil {
		t.Fatal(err.Error())
	}
	for i, name := range []string{"root", "a", "b", "c"} {
		if !held[i] {
			t.Fatalf("the reading node does not hold subtree block %s", name)
		}
	}
}
