package main

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_all "github.com/aperturerobotics/hydra/block/transform/all"
	transform_blockenc "github.com/aperturerobotics/hydra/block/transform/blockenc"
	transform_s2 "github.com/aperturerobotics/hydra/block/transform/s2"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/kvtx"
	iavl "github.com/aperturerobotics/hydra/kvtx/block/iavl"
	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	"github.com/aperturerobotics/hydra/util/blockenc"
	"github.com/aperturerobotics/hydra/volume"
	common_kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// ControllerID is the controller identifier.
const ControllerID = "hydra/prototypes/block-store-volume"

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

var headRefKey = []byte("head")

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
	noGenerateKey, noWriteKey bool,
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
		Id:  "hydra/toys/encrypted-volume/bucket",
		Rev: 1,
	}
	_, _, bucketConf, err = vol.ApplyBucketConfig(ctx, bucketConf)
	if err != nil {
		return nil, err
	}

	// Lookup HEAD ref from object store.
	objStore, err := vol.OpenObjectStore(ctx, "hydra/toys/encrypted-volume/store")
	if err != nil {
		return nil, err
	}

	// encryption transform types
	sfs := transform_all.BuildFactorySet()

	var headCursor *bucket_lookup.Cursor

	// The READ only happens once, or this would be in a separate util function.
	otx, err := objStore.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	headRefDat, headRefOk, err := otx.Get(ctx, headRefKey)
	otx.Discard()
	if err != nil {
		return nil, err
	}
	if headRefOk {
		var headRef *bucket.ObjectRef
		headRef, err = bucket.UnmarshalObjectRef(headRefDat)
		if err != nil {
			return nil, errors.Wrap(err, "unmarshal head ref from storage")
		}
		le.Infof("loaded head reference from storage: %s", headRef.MarshalString())

		headCursor, err = bucket_lookup.BuildCursor(
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
		var transformConf *block_transform.Config
		var putOpts *block.PutOpts

		// note: don't use this key!
		volPeerID := vol.GetPeerID()
		var demoKey [32]byte
		blake3.DeriveKey(
			"aperture-alpha/toys/session-store/volume.go demo",
			[]byte(volPeerID.String()),
			demoKey[:],
		)
		transformConf, err = block_transform.NewConfig([]config.Config{
			&transform_s2.Config{},
			&transform_blockenc.Config{
				BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
				Key:      demoKey[:],
			},
		})
		if err != nil {
			return nil, err
		}
		headCursor, _, err = bucket_lookup.BuildEmptyCursor(
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
		ControllerID,
		kvkey,
		&iavlStore{Store: avlTree},
		storeConf,
		noGenerateKey,
		noWriteKey,
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
