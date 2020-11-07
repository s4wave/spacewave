package hydra_kvtx_cayley

import (
	"context"
	"regexp"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	csp "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/hydra/bucket"
	lc "github.com/aperturerobotics/hydra/bucket/lookup/concurrent"
	"github.com/aperturerobotics/hydra/core"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	reconciler_example "github.com/aperturerobotics/hydra/reconciler/example"
	"github.com/aperturerobotics/hydra/volume"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	volume_kvtxinmem "github.com/aperturerobotics/hydra/volume/kvtxinmem"

	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
	"github.com/cayleygraph/quad"
	"github.com/sirupsen/logrus"
)

// TestCayleyGraph_Basic performs a basic cayley test.
func TestCayleyGraph_Basic(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		panic(err)
	}

	sr.AddFactory(reconciler_example.NewFactory(b))

	av, _, ref, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&volume_kvtxinmem.Config{
			Verbose: true,
		}),
		nil,
	)
	if err != nil {
		panic(err)
	}
	defer ref.Release()
	storageVolCtrl := av.(*volume_controller.Controller)
	storageVol, err := storageVolCtrl.GetVolume(ctx)
	if err != nil {
		panic(err)
	}

	// Construct the node controller.
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, ncRef, err := bus.ExecOneOff(ctx, b, dir, nil)
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

	// build the key/value "object store" for the volume
	objStoreAv, objStoreRef, err := bus.ExecOneOff(
		ctx,
		b,
		volume.NewBuildObjectStoreAPI("cayley-test", storageVol.GetID()),
		nil,
	)
	if err != nil {
		panic(err)
	}
	defer objStoreRef.Release()

	// build the cayley database
	objStore := objStoreAv.GetValue().(volume.BuildObjectStoreAPIValue).GetObjectStore()
	graphOptions := graph.Options{}
	graph, err := NewGraph(objStore, graphOptions)
	if err != nil {
		panic(err)
	}

	// graph is the cayley graph.
	// perform the example hello_world from the cayley repository:
	store := graph

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
}
