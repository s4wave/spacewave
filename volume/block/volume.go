package volume_block

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	bucket "github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_block "github.com/aperturerobotics/hydra/kvtx/block"
	kvtx_vlogger "github.com/aperturerobotics/hydra/kvtx/vlogger"
	"github.com/aperturerobotics/hydra/object"
	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	"github.com/aperturerobotics/hydra/volume"
	common_kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the block volume controller.
const ControllerID = "hydra/volume/block"

// Version is the version of the KVTxInmem implementation.
var Version = semver.MustParse("0.0.1")

// Volume implements a block-graph kvtx backed volume.
type Volume struct {
	*common_kvtx.Volume

	le   *logrus.Entry
	b    bus.Bus
	conf *Config
	rels []func()

	// stateXfrm is the state transformer
	stateXfrm *block_transform.Transformer
}

// NewVolume builds the block-graph volume storing state in a object store.
func NewVolume(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	conf *Config,
) (v *Volume, err error) {
	var rels []func()
	rel := func() {
		for _, f := range rels {
			f()
		}
	}
	defer func() {
		if err != nil {
			v = nil
			rel()
		}
	}()

	le.Debug("building volume")
	stateXfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: le},
		sfs,
		conf.GetStateTransformConf(),
	)
	if err != nil {
		return nil, err
	}

	v = &Volume{
		le:        le,
		b:         b,
		conf:      conf,
		stateXfrm: stateXfrm,
	}

	kvkey, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}
	_ = kvkey

	// Determine the init ref to the HEAD
	var headRef *bucket.ObjectRef

	// initialize headRef using the configured head ref
	initRef := conf.GetInitHeadRef()
	if initRef != nil {
		headRef = initRef.Clone()
	}

	// Lookup the state store
	stateStoreID := conf.GetObjectStoreId()
	stateStoreVol := conf.GetVolumeId()
	if stateStoreVol == "" {
		le.Debug("no volume id set, using any available volume")
	}

	var stateStore object.ObjectStore
	if stateStoreID != "" {
		storeVal, _, storeRef, err := volume.BuildObjectStoreAPIEx(ctx, b, false, stateStoreID, stateStoreVol, nil)
		if err != nil {
			return nil, err
		}
		rels = append(rels, storeRef.Release)
		if err := storeVal.GetError(); err != nil {
			return nil, err
		}
		stateStore = storeVal.GetObjectStore()
	}
	var headState *HeadState
	if stateStore != nil {
		// apply object store prefix
		if prefix := conf.GetObjectStorePrefix(); len(prefix) != 0 {
			stateStore = object.NewPrefixer(stateStore, []byte(prefix))
		}
		// load initial head ref
		var headStateFound bool
		var err error
		headState, headStateFound, err = v.loadHeadState(ctx, stateStore)
		if err != nil {
			return nil, err
		}
		if headStateFound {
			headRef = headState.GetHeadRef()
		}
	} else {
		le.Debug("state store is not configured, changes will not be persisted")
		if headRef.GetEmpty() {
			le.Debug("no initial head reference provided, initializing empty world")
		}
	}
	if headRef == nil {
		headRef = &bucket.ObjectRef{}
	}
	// override bucket id if configured
	if confBucketID := conf.GetBucketId(); confBucketID != "" {
		headRef.BucketId = confBucketID
	}
	if headRef.GetBucketId() == "" {
		return nil, errors.New("head ref bucket id required but was unset")
	}

	// Build the initial cursor (will lookup the bucket)
	cursor, err := bucket_lookup.BuildCursor(
		ctx,
		b,
		le,
		sfs,
		conf.GetVolumeId(),
		headRef,
		nil,
	)
	if err != nil {
		return nil, err
	}
	rels = append(rels, cursor.Release)

	var commitFn kvtx_block.CommitFn
	if stateStore != nil {
		commitFn = func(nref *bucket.ObjectRef) error {
			// write state back to state store
			return v.writeHeadState(ctx, stateStore, nref)
		}
	}

	// Build the kvtx block store.
	bstore, err := kvtx_block.NewStore(ctx, le, cursor, commitFn)
	if err != nil {
		return nil, err
	}

	var store kvtx.Store = bstore
	if conf.GetVerbose() {
		store = kvtx_vlogger.NewVLogger(le, store)
	}

	// Build the volume wrapping the store.
	bvol, err := common_kvtx.NewVolume(
		ctx,
		"hydra/block",
		kvkey,
		store,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
	)
	if err != nil {
		return nil, err
	}
	v.Volume = bvol
	v.rels = rels
	return v, nil
}

// Close closes the volume, returning any errors.
func (v *Volume) Close() error {
	err := v.Volume.Close()
	for _, rel := range v.rels {
		rel()
	}
	return err
}

// _ is a type assertion
var _ volume.Volume = ((*Volume)(nil))
