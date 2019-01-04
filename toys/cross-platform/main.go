package main

import (
	"context"
	"regexp"

	"github.com/aperturerobotics/controllerbus/bus"
	// "github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/aperturerobotics/hydra/core"
	"github.com/aperturerobotics/hydra/node"
	"github.com/aperturerobotics/hydra/reconciler/example"
	"github.com/aperturerobotics/hydra/volume"
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
	av, ref, err := addStorageVolume(ctx, le, b, sr)
	if err != nil {
		panic(err)
	}
	defer ref.Release()

	le.Info("storage volume resolved")
	volCtr := av.GetValue().(volume.Controller)
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
	bucketConf, err := bucket.NewConfig("example-bucket-1", 1, []*bucket.ReconcilerConfig{
		recConf,
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

	// TODO: store something
	_, blRef, err := b.AddDirective(
		node.NewBuildBucketLookup("bucket-basic-1"),
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
					le.Info("lookup returned, attempting to lookup block")
					br, err := cid.UnmarshalString(
						// hello world 5
						"2W1M3RQVxc36HcJeBYKFct1Zsqk8voLjyScr4SpodkKS6DbhzqC3",
					)
					if err != nil {
						le.WithError(err).Warn("block cid unmarshal failed")
						return
					}
					data, found, err := l.LookupBlock(context.Background(), br)
					if err != nil {
						le.WithError(err).Warn("unable to lookup block")
						return
					}
					if !found {
						le.Info("block not found")
						return
					}
					le.Infof("fetched block with data: %s", string(data))
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

	<-ctx.Done()
}
