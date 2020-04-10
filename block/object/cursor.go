package object

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/transform"
	transform_chksum "github.com/aperturerobotics/hydra/block/transform/chksum"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/aperturerobotics/hydra/node"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"
)

// Cursor contains and manages state for interfacing with objects and references
// across multiple buckets and transformation configurations.
type Cursor struct {
	// bus is the controller bus
	bus bus.Bus
	// sfs is the step factory set
	sfs *block_transform.StepFactorySet
	// le is the logger
	le *logrus.Entry
	// opArgs is the op args used
	opArgs *volume.BucketOpArgs
	// transformConf is the transform conf used
	transformConf *block_transform.Config
	// ref is the current ref
	ref *ObjectRef
	// bk is the bucket handle with the transformer applied
	bk bucket.Bucket
	// bkRaw is the bucket handle with no transformer
	bkRaw bucket.Bucket
	// rel is a release function
	rel func()
}

// BuildCursor constructs a new cursor with an initial object ref, configuration,
// an initial operation configuration (bucket and volume ID), and a controller
// bus to acquire handles. Constructing the cursor will also acquire a lookup
// handle. If the volume ID is set, will acquire a bucket handle for writing.
//
// The initial object ref can have an empty root block reference, as long as the
// bucket ID is specified.
//
// Some cursor methods will return another cursor, cloning existing references
// if necessary. Release should be called at least once on all cursors created.
// Cursor calls are not concurrency safe.
func BuildCursor(
	ctx context.Context,
	b bus.Bus,
	le *logrus.Entry,
	sfs *block_transform.StepFactorySet,
	volumeID string,
	ref *ObjectRef,
	transformConf *block_transform.Config,
) (*Cursor, error) {
	if ref.GetBucketId() == "" {
		ref = nil
	}
	c := &Cursor{
		le:  le,
		bus: b,
		sfs: sfs,
		// ref:           ref,
		opArgs:        &volume.BucketOpArgs{VolumeId: volumeID},
		transformConf: transformConf,
	}
	if ref != nil {
		return c.FollowRef(ctx, ref)
	}
	return c, nil
}

// BuildEmptyCursor constructs a bucket handle given the transformation
// configuration, writes the transform config block, then constructs a empty
// cursor.
//
// Note: the transformation configuration is written "raw" to the bucket,
// without encryption or other transformations.
func BuildEmptyCursor(
	ctx context.Context,
	b bus.Bus,
	le *logrus.Entry,
	sfs *block_transform.StepFactorySet,
	bucketID, volumeID string,
	transformConf *block_transform.Config,
	putOpts *bucket.PutOpts,
) (*Cursor, *ObjectRef, error) {
	if bucketID == "" || volumeID == "" {
		return nil, nil, errors.New("bucket id and volume id must be specified")
	}
	c, err := BuildCursor(ctx, b, le, sfs, volumeID, &ObjectRef{BucketId: bucketID}, transformConf)
	if err != nil {
		return nil, nil, err
	}
	if len(transformConf.GetSteps()) != 0 {
		bref, err := WriteTransformConf(c.bkRaw, putOpts, transformConf)
		if err != nil {
			return nil, nil, err
		}
		c.ref.TransformConfRef = bref
	}
	return c, c.ref, nil
}

// MarshalTransformConf marshals a transform configuration with a checksum.
func MarshalTransformConf(transformConf *block_transform.Config) ([]byte, error) {
	dat, err := proto.Marshal(transformConf)
	if err != nil {
		return nil, err
	}
	return transform_chksum.EncodeCRC32(dat)
}

// UnmarshalTransformConf unmarshals a transform configuration checking the checksum.
func UnmarshalTransformConf(data []byte) (*block_transform.Config, error) {
	conf := &block_transform.Config{}
	tdat, err := transform_chksum.DecodeCRC32(data)
	if err != nil {
		return nil, err
	}
	if err := proto.Unmarshal(tdat, conf); err != nil {
		return nil, err
	}
	return conf, nil
}

// WriteTransformConf writes a transformation configuration and returns the block ref.
func WriteTransformConf(
	bk bucket.Bucket,
	putOpts *bucket.PutOpts,
	transformConf *block_transform.Config,
) (*cid.BlockRef, error) {
	dat, err := MarshalTransformConf(transformConf)
	if err != nil {
		return nil, err
	}
	eve, err := bk.PutBlock(dat, putOpts)
	if err != nil {
		return nil, err
	}
	return eve.GetBlockCommon().GetBlockRef(), nil
}

// FetchTransformConf fetches a transform config.
// returns nil if block not found
func FetchTransformConf(
	bk bucket.Bucket,
	tconfRef *cid.BlockRef,
) (*block_transform.Config, error) {
	data, ok, err := bk.GetBlock(tconfRef)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return UnmarshalTransformConf(data)
}

// BuildTransaction builds a block transaction at the cursor location.
// putOpts is optional
func (c *Cursor) BuildTransaction(putOpts *bucket.PutOpts) (*block.Transaction, *block.Cursor) {
	return c.BuildTransactionAtRef(putOpts, c.ref.GetRootRef())
}

// BuildTransactionAtRef builds a transaction rooted at the reference.
func (c *Cursor) BuildTransactionAtRef(putOpts *bucket.PutOpts, ref *cid.BlockRef) (*block.Transaction, *block.Cursor) {
	return block.NewTransaction(c.bk, ref, putOpts)
}

// FollowRef attempts to follow a object reference.
func (c *Cursor) FollowRef(
	ctx context.Context,
	objRef *ObjectRef,
) (*Cursor, error) {
	bk := c.bk
	bkRaw := c.bkRaw
	transformConf := c.transformConf
	opArgs := &volume.BucketOpArgs{
		BucketId: c.opArgs.GetBucketId(),
		VolumeId: c.opArgs.GetVolumeId(),
	}
	var rel func()
	if orBkId := objRef.GetBucketId(); orBkId != "" {
		if c.opArgs.GetBucketId() != orBkId {
			// 1. acquire the handle
			var err error
			bk, rel, err = node.StartBucketRWOperation(ctx, c.bus, &volume.BucketOpArgs{
				VolumeId: c.opArgs.GetVolumeId(),
				BucketId: orBkId,
			})
			if err != nil {
				return nil, err
			}
			opArgs.BucketId = orBkId

			// 2. initial transform conf if necessary
			bkRaw = bk
			transformConf := c.transformConf
			if transformConf != nil {
				bk, err = block_transform.NewTransformer(
					controller.ConstructOpts{Logger: c.le},
					c.sfs,
					transformConf,
					bk,
				)
				if err != nil {
					rel()
					return nil, err
				}
			}
		}
	}

	// 3. fetch the transform config block
	if tconfRef := objRef.GetTransformConfRef(); !tconfRef.GetEmpty() &&
		!proto.Equal(tconfRef, c.ref.GetTransformConfRef()) {
		bc, err := FetchTransformConf(bk, tconfRef)
		if err != nil {
			if rel != nil {
				rel()
			}
			return nil, err
		}
		// actuate conf
		bk, err = block_transform.NewTransformer(
			controller.ConstructOpts{Logger: c.le},
			c.sfs,
			bc,
			bkRaw,
		)
		if err != nil {
			if rel != nil {
				rel()
			}
			return nil, err
		}
		transformConf = bc
	}

	// 4. return new cursor
	ncc := c.clone()
	ncc.bk = bk
	ncc.bkRaw = bkRaw
	ncc.ref = objRef
	ncc.transformConf = transformConf
	ncc.rel = rel
	ncc.opArgs = opArgs
	return ncc, nil
}

// SetRootRef sets the cursor's root ref.
func (c *Cursor) SetRootRef(b *cid.BlockRef) {
	if c.ref == nil {
		c.ref = &ObjectRef{}
	}
	c.ref.RootRef = b
}

// SetBucket sets the cursor's ref bucket.
func (c *Cursor) SetBucket(b string) {
	c.ref.BucketId = b
}

// GetEncBucket returns the bucket with the wrapped transformers.
func (c *Cursor) GetEncBucket() bucket.Bucket {
	return c.bk
}

// GetRawBucket returns the bucket without the wrapped transformers.
func (c *Cursor) GetRawBucket() bucket.Bucket {
	return c.bkRaw
}

// GetRef returns a copy of the current object ref.
func (c *Cursor) GetRef() *ObjectRef {
	if c.ref == nil {
		return &ObjectRef{}
	}

	return proto.Clone(c.ref).(*ObjectRef)
}

// GetTransformConf returns the current transform config.
func (c *Cursor) GetTransformConf() *block_transform.Config {
	return c.transformConf
}

// Unmarshal unmarshals a block at the position.
// Returns nil if the ref is empty or the block not found.
func (c *Cursor) Unmarshal(
	ctor func() block.Block,
) (block.Block, error) {
	rr := c.ref.GetRootRef()
	if rr.GetEmpty() {
		return nil, nil
	}

	data, ok, err := c.bk.GetBlock(rr)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	b := ctor()
	if b == nil {
		return nil, nil
	}
	if err := b.UnmarshalBlock(data); err != nil {
		return nil, err
	}
	return b, nil
}

// Release releases cursor resources.
func (c *Cursor) Release() {
	if c.rel != nil {
		c.rel()
	}
}

// clone returns a base copy of the cursor
func (c *Cursor) clone() *Cursor {
	return &Cursor{
		le:            c.le,
		sfs:           c.sfs,
		bus:           c.bus,
		transformConf: c.transformConf,
		bk:            c.bk,
		bkRaw:         c.bkRaw,
		ref:           c.ref,
	}
}
