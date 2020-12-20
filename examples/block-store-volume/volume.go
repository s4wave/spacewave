package main

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/hydra/block/iavl"
	"github.com/aperturerobotics/hydra/block/object"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_all "github.com/aperturerobotics/hydra/block/transform/all"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/kvtx"
	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	"github.com/aperturerobotics/hydra/volume"
	common_kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	"github.com/blang/semver"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller identifier.
const ControllerID = "hydra/toys/encrypted-volume/1"

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

var (
	headRefKey = []byte("head")
)

// EncryptedVolume wraps a base volume with a block graph.
//
// Note: prototype / toy.
//
// This is a prototype of implementing a volume on top of a Hydra DAG, stored
// inside a underlying volume. This will later be separated into parts:
//
// - hydra store on top of a hydra dag
// - hydra store on top of a anchor chain
// - volume implementation of both, on top of an underlying storage volume
//
// TODO the resources are not closed cleanly here (ideally iavlStore closes
// block cursor as well as underlying stores)
//
// TODO write HEAD reference in storage when Commit() is called
//
// TODO for this to be viable, a in-memory LRU cache must be implemented.

// NewEncryptedVolume wraps a volume with an encrypted block graph.
//
// Looks up the volume from the storage bus.
func NewEncryptedVolume(
	ctx context.Context,
	b bus.Bus,
	le *logrus.Entry,
	baseVol volume.Controller,
	conf *kvkey.Config,
	storeConf *store_kvtx.Config,
) (volume.Volume, error) {
	kvkey, err := kvkey.NewKVKey(conf)
	if err != nil {
		return nil, err
	}

	// Construct root cursor on old volume.
	vol, err := baseVol.GetVolume(ctx)
	if err != nil {
		return nil, err
	}
	bucketConf := &bucket.Config{
		Id:      "hydra/toys/encrypted-volume/bucket",
		Version: 1,
	}
	_, _, bucketConf, err = vol.PutBucketConfig(bucketConf)
	if err != nil {
		return nil, err
	}

	// Lookup HEAD ref from object store.
	objStore, err := vol.OpenObjectStore(ctx, "hydra/toys/encrypted-volume/store")
	if err != nil {
		return nil, err
	}

	// encryption transform types
	sfs, err := transform_all.BuildFactorySet()
	if err != nil {
		return nil, err
	}

	var headRef *object.ObjectRef
	var headCursor *object.Cursor

	// The READ only happens once, or this would be in a separate util function.
	otx, err := objStore.NewTransaction(true)
	if err != nil {
		return nil, err
	}
	headRefDat, headRefOk, err := otx.Get(headRefKey)
	otx.Discard()
	if err != nil {
		return nil, err
	}
	if headRefOk {
		headRef = &object.ObjectRef{}
		if err := proto.Unmarshal(headRefDat, headRef); err != nil {
			return nil, errors.Wrap(err, "unmarshal head ref from underlying storage")
		}
		le.Infof("loaded head reference from storage: %s", headRef.MarshalString())

		headCursor, err = object.BuildCursor(
			ctx,
			b,
			le,
			sfs,
			vol.GetID(),
			headRef,
			nil, // nil transform conf
		)
	} else {
		le.Info("head reference empty in storage, building new cursor")
		var transformConf *block_transform.Config // nil
		var putOpts *bucket.PutOpts               // nil
		headCursor, headRef, err = object.BuildEmptyCursor(
			ctx,
			b,
			le,
			sfs,
			bucketConf.Id,
			vol.GetID(),
			transformConf,
			putOpts,
		)
	}
	if err != nil {
		return nil, err
	}

	// Construct iavl block tree.
	avlTree := iavl.NewAVLTree(headCursor)

	// Build kvkey volume on top.
	return common_kvtx.NewVolume(
		ctx,
		"hydra/kvtxinmem",
		kvkey,
		&iavlStore{Store: avlTree},
		storeConf,
		false,
	)
}

// iavlStore wraps iavl store to satisfy kvtx.Store
type iavlStore struct {
	kvtx.Store
}

// Execute executes the given store.
// Returning nil ends execution.
func (s *iavlStore) Execute(ctx context.Context) error {
	return nil
}

// _ is a type assertion
var _ store_kvtx.Store = ((*iavlStore)(nil))
