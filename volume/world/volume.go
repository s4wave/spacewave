package volume_world

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	bucket "github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_block "github.com/aperturerobotics/hydra/kvtx/block"
	kvtx_vlogger "github.com/aperturerobotics/hydra/kvtx/vlogger"
	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	"github.com/aperturerobotics/hydra/volume"
	common_kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	"github.com/aperturerobotics/hydra/world"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the world object volume controller.
const ControllerID = "hydra/volume/world"

// Version is the version of the KVTxInmem implementation.
var Version = semver.MustParse("0.0.1")

// Volume implements a World Object block-graph kvtx backed volume.
type Volume struct {
	*common_kvtx.Volume

	le   *logrus.Entry
	b    bus.Bus
	conf *Config
	rels []func()
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
	v = &Volume{
		le:   le,
		b:    b,
		conf: conf,
	}

	kvkey, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}

	// Determine the init ref to the HEAD
	var headRef *bucket.ObjectRef

	// initialize headRef using the configured head ref
	initRef := conf.GetInitHeadRef()
	if initRef != nil {
		headRef = initRef.Clone()
	}

	// Construct the bus engine
	busEngine := world.NewBusEngine(ctx, b, conf.GetEngineId())
	worldState := world.NewEngineWorldState(busEngine, true)

	// load initial head ref
	headState, headStateFound, err := v.loadHeadState(ctx, worldState)
	if err != nil {
		return nil, err
	}
	if headStateFound && headState != nil {
		headRef = headState
	}

	// override bucket id if configured
	if confBucketID := conf.GetBucketId(); confBucketID != "" {
		headRef.BucketId = confBucketID
	}

	// requires either initial ref or head ref to be set
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

	commitFn := func(nref *bucket.ObjectRef) error {
		// write state back to state store
		return v.writeHeadState(ctx, worldState, nref)
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
		ControllerID,
		kvkey,
		store,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
		conf.GetNoWriteKey(),
		nil,
		func() error { cursor.Release(); return nil },
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

// Delete closes the volume and removes the backing store.
func (v *Volume) Delete() error {
	for _, rel := range v.rels {
		rel()
	}
	return v.Volume.Delete()
}

// _ is a type assertion
var _ volume.Volume = ((*Volume)(nil))
