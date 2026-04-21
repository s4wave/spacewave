package common

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	csp "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6/storage/memory"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	lc "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	git "github.com/s4wave/spacewave/db/git/block"
	git_examples "github.com/s4wave/spacewave/db/git/example"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
)

func RunDemoGit(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	volCtr volume.Controller,
	url string,
) error {
	vol, err := volCtr.GetVolume(ctx)
	if err != nil {
		return err
	}

	lookupConf := &lc.Config{
		// NotFoundBehavior: lc.NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE,
		NotFoundBehavior: lc.NotFoundBehavior_NotFoundBehavior_NONE,
		PutBlockBehavior: lc.PutBlockBehavior_PutBlockBehavior_ALL,
	}
	cc, err := csp.NewControllerConfig(configset.NewControllerConfig(1, lookupConf), true)
	if err != nil {
		return err
	}
	bucketConf, err := bucket.NewConfig(
		"example-bucket-1",
		1,
		nil,
		&bucket.LookupConfig{Controller: cc},
	)
	if err != nil {
		return err
	}
	bucketID := bucketConf.GetId()

	// assert the volume
	_, _, abcRef, err := bus.ExecOneOff(
		ctx,
		b,
		bucket.NewApplyBucketConfigToVolume(
			bucketConf,
			vol.GetID(),
		),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	abcRef.Release()

	inMem := memory.NewStorage()
	worktree := memfs.New()

	sfs := transform_all.BuildFactorySet()
	oc, rootRef, err := bucket_lookup.BuildEmptyCursor(ctx, b, le, sfs, bucketID, vol.GetID(), nil, nil)
	if err != nil {
		return err
	}
	_ = rootRef
	btx, bcs := oc.BuildTransaction(nil)
	store, err := git.NewStore(ctx, btx, bcs, inMem, nil)
	if err != nil {
		return err
	}
	err = git_examples.RunCloneExample(ctx, le, url, store, worktree)
	if err != nil {
		return err
	}
	return store.Commit()
}
