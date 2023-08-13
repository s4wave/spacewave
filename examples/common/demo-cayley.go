package common

import (
	"context"
	"fmt"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	csp "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	lc "github.com/aperturerobotics/hydra/bucket/lookup/concurrent"
	hydra_kvtx_cayley "github.com/aperturerobotics/hydra/kvtx/cayley"
	reconciler_example "github.com/aperturerobotics/hydra/reconciler/example"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
	"github.com/cayleygraph/quad"
	"github.com/sirupsen/logrus"
)

func RunDemoCayley(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	volCtr volume.Controller,
) error {
	tStart := time.Now()
	vol, err := volCtr.GetVolume(ctx)
	if err != nil {
		return err
	}
	recConf, err := bucket.NewReconcilerConfig(
		"example-reconciler-1",
		configset.NewControllerConfig(2, &reconciler_example.Config{}),
	)
	if err != nil {
		return err
	}

	lookupConf := &lc.Config{
		// NotFoundBehavior: lc.NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE,
		NotFoundBehavior: lc.NotFoundBehavior_NotFoundBehavior_NONE,
		PutBlockBehavior: lc.PutBlockBehavior_PutBlockBehavior_ALL_VOLUMES,
	}
	cc, err := csp.NewControllerConfig(configset.NewControllerConfig(1, lookupConf), true)
	if err != nil {
		return err
	}
	bucketConf, err := bucket.NewConfig("example-bucket-1", 1, []*bucket.ReconcilerConfig{
		recConf,
	}, &bucket.LookupConfig{
		Controller: cc,
	})
	if err != nil {
		return err
	}

	// apply the bucket config
	_, err = bucket.ExApplyBucketConfig(ctx, b, bucket.NewApplyBucketConfigToVolume(
		bucketConf,
		vol.GetID(),
	))
	if err != nil {
		return err
	}

	// store something
	lkr, _, lkRef, err := lookup.ExBuildBucketLookup(ctx, b, false, "example-bucket-1", nil)
	if err != nil {
		return err
	}
	defer lkRef.Release()

	lk, err := lkr.GetLookup(ctx)
	if err != nil {
		return err
	}

	le.Info("lookup returned, storing block")
	blockData := fmt.Sprintf("hello world: %s", time.Now().String())
	ev, _, err := lk.PutBlock(ctx, []byte(blockData), nil)
	if err != nil {
		return err
	}
	var refStr string
	for _, pr := range ev {
		refStr = pr.MarshalString()
		if len(refStr) == 0 {
			panic("empty ref after putblock")
		}
		le.Infof("placed block with ref: %v", refStr)
	}

	le.WithField("ref", refStr).Info("attempting to lookup block")
	br, err := bucket.ParseObjectRef(
		refStr,
	)
	if err != nil {
		return err
	}

	// race condition: bucket handle list was empty
	data, found, err := lk.LookupBlock(context.Background(), br.GetRootRef())
	if !found && err == nil {
		err = block.ErrNotFound
	}
	if err != nil {
		return err
	}
	le.Infof("fetched block with data: %s", string(data))

	// build the key/value "object store" for the volume
	objStoreAv, _, objStoreRef, err := bus.ExecOneOff(
		ctx,
		b,
		volume.NewBuildObjectStoreAPI("cayley-test", vol.GetID()),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	defer objStoreRef.Release()
	objStore := objStoreAv.GetValue().(volume.BuildObjectStoreAPIValue).GetObjectStore()

	// attempt concurrent transactions
	t1, _ := objStore.NewTransaction(ctx, false)
	t2, _ := objStore.NewTransaction(ctx, false)
	_, _, _ = t2.Get(ctx, []byte("test"))
	t2.Discard()
	// expect that t1 is still live (not discarded)
	_, _, err = t1.Get(ctx, []byte("test"))
	if err != nil {
		return err
	}
	t1.Discard()

	// build the cayley database
	graphOptions := graph.Options{}
	store, err := hydra_kvtx_cayley.NewGraph(ctx, objStore, graphOptions)
	if err != nil {
		return err
	}

	// perform the example hello_world from the cayley repository:
	store.AddQuad(ctx, quad.Make("phrase of the day", "is of course", "Hello World!", nil))

	// Now we create the path, to get to our data
	p := cayley.StartPath(store, quad.String("phrase of the day")).Out(quad.String("is of course"))

	// Now we iterate over results. Arguments:
	// 1. Optional context used for cancellation.
	// 2. Quad store, but we can omit it because we have already built path with it.
	err = p.Iterate(ctx).EachValue(ctx, nil, func(value quad.Value) error {
		nativeValue := quad.NativeOf(value) // this converts RDF values to normal Go types
		le.Info(nativeValue)
		return nil
	})
	if err != nil {
		return err
	}

	// Example 2
	le.Info("writing second round of quads")
	t := graph.NewTransaction()
	t.AddQuad(quad.Make("food", "is", "good", nil))
	t.AddQuad(quad.Make("food", "has", "calories", nil))
	t.AddQuad(quad.Make("cats", "are", "awesome", nil))
	t.AddQuad(quad.Make("cats", "type", "animal", "feline"))
	if err := store.ApplyTransaction(ctx, t); err != nil {
		return err
	}

	le.Info("printing all quads")
	it := store.QuadsAllIterator(ctx).Iterate(ctx)
	for it.Next(ctx) {
		res, err := it.Result(ctx)
		if err != nil {
			return err
		}
		q, err := store.Quad(ctx, res)
		if err != nil {
			return err
		}
		le.Infof("quad: %v", q)
	}

	// Now we iterate over results. Arguments:
	// 1. Optional context used for cancellation.
	// 2. Quad store, but we can omit it because we have already built path with it.
	le.Info("iterating quads")

	// Now we create the path, to get to our data.

	// This path checks for cats -> want to -> ??? where label == "really"
	// LabelContext filters by quads which have label "really"
	p = cayley.
		StartPath(store, quad.String("cats")).
		LabelContext("really").
		Out(quad.String("want to"))

	err = p.Iterate(ctx).EachValue(ctx, nil, func(value quad.Value) error {
		nativeValue := quad.NativeOf(value) // this converts RDF values to normal Go types
		le.Info(nativeValue)
		return err
	})
	if err != nil {
		return err
	}

	// Check for links to "kill you" via "want to"
	// Returns "food"
	p = cayley.
		StartPath(store, quad.String("kill you")).
		LabelContext("actually").
		In("want to")
	err = p.Iterate(ctx).EachValue(ctx, nil, func(value quad.Value) error {
		nativeValue := quad.NativeOf(value) // this converts RDF values to normal Go types
		le.Info(nativeValue)
		return nil
	})
	if err != nil {
		return err
	}

	tEnd := time.Now()
	le.Infof("demo completed in %v", tEnd.Sub(tStart).String())

	le.Info("demo: starting follow recursive: expect to see <f> <b> <d> <c>")
	// Test follow recursive
	gt := graph.NewTransaction()
	gt.AddQuad(quad.MakeIRI("a", "ref", "b", ""))
	gt.AddQuad(quad.MakeIRI("b", "ref", "c", ""))
	gt.AddQuad(quad.MakeIRI("c", "ref", "d", ""))
	gt.AddQuad(quad.MakeIRI("b", "ref", "d", ""))
	gt.AddQuad(quad.MakeIRI("e", "ref", "f", ""))
	gt.AddQuad(quad.MakeIRI("f", "ref", "d", ""))
	if err := store.ApplyTransaction(ctx, gt); err != nil {
		return err
	}
	// The third argument, "depthTags" is a set of tags that will return strings of
	// numeric values relating to how many applications of the path were applied the
	// first time the result node was seen.
	p = cayley.
		StartPath(store, quad.IRI("e"), quad.IRI("a")).
		FollowRecursive(quad.IRI("ref"), -1, []string{"depth"})
	err = p.Iterate(ctx).EachValue(ctx, nil, func(value quad.Value) error {
		nativeValue := quad.NativeOf(value) // this converts RDF values to normal Go types
		le.Info(nativeValue)
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
