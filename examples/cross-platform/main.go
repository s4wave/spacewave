package main

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	csp "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	lc "github.com/aperturerobotics/hydra/bucket/lookup/concurrent"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/aperturerobotics/hydra/core"
	common "github.com/aperturerobotics/hydra/examples/common"
	"github.com/aperturerobotics/hydra/kvtx/cayley"
	"github.com/aperturerobotics/hydra/node"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	reconciler_example "github.com/aperturerobotics/hydra/reconciler/example"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
	"github.com/cayleygraph/quad"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		panic(err)
	}

	sr.AddFactory(reconciler_example.NewFactory(b))

	// TODO: add storage depending on if we are in js or not.
	av, _, ref, err := common.AddStorageVolume(ctx, le, b, sr)
	if err != nil {
		panic(err)
	}
	defer ref.Release()

	// Construct the node controller.
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, _, ncRef, err := loader.WaitExecControllerRunning(ctx, b, dir, nil)
	if err != nil {
		panic(err)
	}
	defer ncRef.Release()
	le.Info("node controller resolved")

	le.Info("storage volume resolved")
	volCtr := av.(volume.Controller)
	vol, err := volCtr.GetVolume(ctx)
	if err != nil {
		panic(err)
	}
	recConf, err := bucket.NewReconcilerConfig(
		"example-reconciler-1",
		configset.NewControllerConfig(2, &reconciler_example.Config{}),
	)
	if err != nil {
		panic(err)
	}

	lookupConf := &lc.Config{
		// NotFoundBehavior: lc.NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE,
		NotFoundBehavior: lc.NotFoundBehavior_NotFoundBehavior_NONE,
		PutBlockBehavior: lc.PutBlockBehavior_PutBlockBehavior_ALL_VOLUMES,
	}
	cc, err := csp.NewControllerConfig(configset.NewControllerConfig(1, lookupConf))
	if err != nil {
		panic(err)
	}
	bucketConf, err := bucket.NewConfig("example-bucket-1", 1, []*bucket.ReconcilerConfig{
		recConf,
	}, &bucket.LookupConfig{
		Controller: cc,
	})
	if err != nil {
		panic(err)
	}

	// assert the volume
	_, abcRef, err := bus.ExecOneOff(
		ctx,
		b,
		bucket.NewApplyBucketConfig(
			bucketConf,
			regexp.MustCompile(regexp.QuoteMeta(vol.GetID())),
		),
		nil,
	)
	if err != nil {
		panic(err)
	}
	abcRef.Release()

	// store something
	lkCh := make(chan lookup.Lookup, 1)
	_, blRef, err := b.AddDirective(
		node.NewBuildBucketLookup("example-bucket-1"),
		bus.NewCallbackHandler(
			func(av directive.AttachedValue) {
				v := av.GetValue().(node.BuildBucketLookupValue)
				conf := v.GetBucketConfig()
				le.Infof("bucket lookup added: conf(%#v)", conf)
				go func() {
					l, err := v.GetLookup(ctx)
					if err != nil {
						le.WithError(err).Warn("cannot get lookup")
						return
					}
					if l == nil {
						le.Info("handle w/ lookup not ready yet")
						return
					}
					select {
					case lkCh <- l:
					default:
					}
				}()
			},
			func(av directive.AttachedValue) {
				le.Infof("bucket lookup removed: %#v", av.GetValue())
			}, nil,
		),
	)
	if err != nil {
		le.WithError(err).Warn("cannot build bucket lookup")
		return
	}
	defer blRef.Release()

	var lk lookup.Lookup
	select {
	case <-ctx.Done():
		return
	case lk = <-lkCh:
	}

	le.Info("lookup returned, attempting to place block")
	blockData := fmt.Sprintf("hello world: %s", time.Now().String())
	ev, err := lk.PutBlock(ctx, []byte(blockData), nil)
	if err != nil {
		panic(err)
	}
	pr := ev.GetBlockCommon().GetBlockRef()
	refStr := pr.MarshalString()
	if len(refStr) == 0 {
		panic("empty ref after putblock")
	}
	le.Infof("placed block with ref: %v", refStr)

	le.WithField("ref", refStr).Info("attempting to lookup block")
	br, err := cid.UnmarshalString(
		refStr,
	)
	if err != nil {
		panic(err)
	}

	// race condition: bucket handle list was empty
	data, found, err := lk.LookupBlock(context.Background(), br)
	if err != nil {
		panic(err)
	}
	if !found {
		le.Info("block not found")
		return
	}
	le.Infof("fetched block with data: %s", string(data))

	// build the key/value "object store" for the volume
	objStoreAv, objStoreRef, err := bus.ExecOneOff(
		ctx,
		b,
		volume.NewBuildObjectStoreAPI("cayley-test", vol.GetID()),
		nil,
	)
	if err != nil {
		panic(err)
	}
	defer objStoreRef.Release()
	objStore := objStoreAv.GetValue().(volume.BuildObjectStoreAPIValue).GetObjectStore()

	// attempt concurrent transactions
	t1, _ := objStore.NewTransaction(false)
	t2, _ := objStore.NewTransaction(false)
	_, _, _ = t2.Get([]byte("test"))
	t2.Discard()
	// expect that t1 is still live (not discarded)
	_, _, err = t1.Get([]byte("test"))
	if err != nil {
		panic(err)
	}
	t1.Discard()

	// build the cayley database
	graphOptions := graph.Options{}
	graph, err := hydra_kvtx_cayley.NewGraph(objStore, graphOptions)
	if err != nil {
		panic(err)
	}

	// graph is the cayley graph.
	// perform the example hello_world from the cayley repository:
	store := graph
	_ = store

	store.AddQuad(quad.Make("phrase of the day", "is of course", "Hello World!", nil))

	// Now we create the path, to get to our data
	p := cayley.StartPath(store, quad.String("phrase of the day")).Out(quad.String("is of course"))

	// Now we iterate over results. Arguments:
	// 1. Optional context used for cancellation.
	// 2. Quad store, but we can omit it because we have already built path with it.
	err = p.Iterate(nil).EachValue(nil, func(value quad.Value) {
		nativeValue := quad.NativeOf(value) // this converts RDF values to normal Go types
		le.Info(nativeValue)
	})
	if err != nil {
		panic(err)
	}

	// Example 2
	le.Info("writing second round of quads")
	t := store
	//t, err := cayley.NewMemoryGraph()
	//	_ = err
	t.AddQuad(quad.Make("food", "is", "good", nil))
	t.AddQuad(quad.Make("cats", "are", "awesome", nil))
	t.AddQuad(quad.Make("cats", "are", "scary", nil))
	t.AddQuad(quad.Make("cats", "want to", "kill you", nil))

	// TODO BUG -
	// 1. Run without this line.
	// 2. Uncomment the line and run again
	// 3. You will notice that the "failing" is not shown.
	// 4. Clear database
	// 5. Run again (different output)
	// For some reason on a second run the quad is not added properly.
	t.AddQuad(quad.Make("cats", "are", "failing", nil))
	// t.RemoveQuad(quad.Make("cats", "are", "failing", nil))

	// Now we iterate over results. Arguments:
	// 1. Optional context used for cancellation.
	// 2. Quad store, but we can omit it because we have already built path with it.
	le.Info("iterating quads")

	// Now we create the path, to get to our data
	p = cayley.StartPath(t, quad.String("cats")).Out(quad.String("are"))

	err = p.Iterate(nil).EachValue(nil, func(value quad.Value) {
		nativeValue := quad.NativeOf(value) // this converts RDF values to normal Go types
		le.Info(nativeValue)
	})
	if err != nil {
		// panic(err)
	}
	<-time.After(time.Second)
}
