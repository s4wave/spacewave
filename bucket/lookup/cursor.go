package bucket_lookup

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/block"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_chksum "github.com/aperturerobotics/hydra/block/transform/chksum"
	"github.com/aperturerobotics/hydra/bucket"
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
	// ref is the current ref
	// contains the current transform config ref
	ref *bucket.ObjectRef
	// bkt is the store handle
	bkt bucket.Bucket
	// opArgs is the bucket ID / volume ID pair used for bkt
	opArgs *bucket.BucketOpArgs
	// xfrm is the transformer handle
	xfrm block.Transformer
	// transformConf is the transform conf used for xfrm
	transformConf *block_transform.Config
	// rel is a release function
	rel func()
}

// NewCursor constructs a new Cursor with the provided parameters.
//
// This function allows the caller to create a Cursor with specific details,
// providing more control over the Cursor's initial state compared to BuildCursor.
//
// NOTE: it is almost always recommended to use BuildCursor or BuildEmptyCursor instead.
//
// Parameters:
// - ctx: The context for the operation
// - b: The controller bus
// - le: The logger entry
// - sfs: The step factory set
// - bkt: The bucket to use
// - xfrm: The transformer to use (can be nil)
// - ref: The initial object reference (can be nil)
// - opArgs: The bucket operation arguments
// - transformConf: The transform configuration (can be nil)
//
// Returns a new Cursor instance and any error encountered during creation.
func NewCursor(
	ctx context.Context,
	b bus.Bus,
	le *logrus.Entry,
	sfs *block_transform.StepFactorySet,
	bkt bucket.Bucket,
	xfrm block.Transformer,
	ref *bucket.ObjectRef,
	opArgs *bucket.BucketOpArgs,
	transformConf *block_transform.Config,
) *Cursor {
	if ref == nil {
		ref = &bucket.ObjectRef{}
	}
	if opArgs == nil {
		opArgs = &bucket.BucketOpArgs{}
	}
	return &Cursor{
		le:            le,
		bus:           b,
		sfs:           sfs,
		bkt:           bkt,
		xfrm:          xfrm,
		ref:           ref,
		opArgs:        opArgs,
		transformConf: transformConf,
	}
}

// BuildCursor constructs a new cursor with an initial object ref, configuration,
// an initial operation configuration (bucket and volume ID), and a controller
// bus to acquire handles. Constructing the cursor will also acquire a lookup
// handle. If the volume ID is set, will acquire a bucket handle for writing.
//
// The initial object ref can have an empty root block reference, as long as the
// bucket ID is specified. The object ref can be nil to create an empty cursor,
// which can be used with FollowRef to access buckets.
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
	ref *bucket.ObjectRef,
	transformConf *block_transform.Config,
) (*Cursor, error) {
	var xfrm block.Transformer
	if !transformConf.GetEmpty() {
		var err error
		xfrm, err = block_transform.NewTransformer(controller.ConstructOpts{
			Logger: le,
		}, sfs, transformConf)
		if err != nil {
			return nil, err
		}
	} else {
		transformConf = nil
	}
	c := &Cursor{
		le:            le,
		bus:           b,
		sfs:           sfs,
		opArgs:        &bucket.BucketOpArgs{VolumeId: volumeID},
		xfrm:          xfrm,
		transformConf: transformConf,
	}
	refBucketID := ref.GetBucketId()
	if !ref.GetEmpty() && refBucketID == "" {
		return nil, errors.New("reference not empty: bucket id must be specified")
	}
	// if a bucket id is specified, FollowRef to build the stores.
	if !ref.GetEmpty() || refBucketID != "" {
		return c.FollowRef(ctx, ref)
	}
	return c, nil
}

// BuildEmptyCursor constructs a bucket handle with a new blank ObjectRef.
//
// The optional transform config is used to transform block reads/writes.
// If set, the transform config is stored in-line in the root ObjectRef.
func BuildEmptyCursor(
	ctx context.Context,
	b bus.Bus,
	le *logrus.Entry,
	sfs *block_transform.StepFactorySet,
	bucketID, volumeID string,
	transformConf *block_transform.Config,
	putOpts *block.PutOpts,
) (*Cursor, *bucket.ObjectRef, error) {
	if bucketID == "" || volumeID == "" {
		return nil, nil, errors.New("bucket id and volume id must be specified")
	}
	c, err := BuildCursor(
		ctx,
		b,
		le,
		sfs,
		volumeID,
		&bucket.ObjectRef{BucketId: bucketID, TransformConf: transformConf.CloneVT()},
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	return c, c.ref, nil
}

// MarshalTransformConf marshals a transform configuration with a checksum.
func MarshalTransformConf(transformConf *block_transform.Config) ([]byte, error) {
	dat, err := transformConf.MarshalVT()
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
	if err := conf.UnmarshalVT(tdat); err != nil {
		return nil, err
	}
	return conf, nil
}

// WriteTransformConf writes a transformation configuration and returns the block ref.
func WriteTransformConf(
	ctx context.Context,
	bk bucket.Bucket,
	xfrm block.Transformer,
	putOpts *block.PutOpts,
	transformConf *block_transform.Config,
) (*block.BlockRef, bool, error) {
	dat, err := MarshalTransformConf(transformConf)
	if err != nil {
		return nil, false, err
	}
	if xfrm != nil {
		dat, err = xfrm.EncodeBlock(dat)
		if err != nil {
			return nil, false, err
		}
	}
	return bk.PutBlock(ctx, dat, putOpts)
}

// FetchTransformConf fetches a transform config.
// returns nil if block not found
func FetchTransformConf(
	ctx context.Context,
	bk block.StoreOps,
	tconfRef *block.BlockRef,
	xfrm block.Transformer,
) (*block_transform.Config, error) {
	data, ok, err := bk.GetBlock(ctx, tconfRef)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	if xfrm != nil {
		data, err = xfrm.DecodeBlock(data)
		if err != nil {
			return nil, err
		}
	}
	return UnmarshalTransformConf(data)
}

// Clone clones the block cursor.
func (c *Cursor) Clone() *Cursor {
	return c.clone()
}

// BuildTransaction builds a block transaction at the cursor location.
// putOpts is optional
func (c *Cursor) BuildTransaction(putOpts *block.PutOpts) (*block.Transaction, *block.Cursor) {
	return c.BuildTransactionAtRef(putOpts, c.ref.GetRootRef())
}

// BuildTransactionAtRef builds a transaction rooted at the reference.
func (c *Cursor) BuildTransactionAtRef(putOpts *block.PutOpts, ref *block.BlockRef) (*block.Transaction, *block.Cursor) {
	return block.NewTransaction(c.bkt, c.xfrm, ref, putOpts)
}

// FollowRef attempts to follow a object reference using the bucket ID from the reference.
//
// Keeps the same volume ID from the existing cursor.
// If the reference bucket id is empty, uses the existing bucket id.
func (c *Cursor) FollowRef(
	ctx context.Context,
	objRef *bucket.ObjectRef,
) (*Cursor, error) {
	opArgs := &bucket.BucketOpArgs{
		BucketId: c.opArgs.GetBucketId(),
		VolumeId: c.opArgs.GetVolumeId(),
	}
	if refBucketID := objRef.GetBucketId(); refBucketID != "" {
		opArgs.BucketId = refBucketID
	}
	return c.FollowRefWithOpArgs(ctx, objRef, opArgs)
}

// FollowRefWithOpArgs attempts to follow a object reference.
//
// The op args are used to control which bucket and volume ID are used.
// If no volume ID is set, uses a cross-volume read-only lookup.
// The bucket ID in the reference is ignored.
// If opArgs is nil re-uses the current op args for cursor c.
func (c *Cursor) FollowRefWithOpArgs(
	ctx context.Context,
	objRef *bucket.ObjectRef,
	opArgs *bucket.BucketOpArgs,
) (*Cursor, error) {
	var rel func()
	bkt, xfrm := c.bkt, c.xfrm
	transformConf := c.transformConf
	if opArgs == nil {
		opArgs = c.opArgs.CloneVT()
	} else {
		opArgs = opArgs.CloneVT()
	}

	// if we are switching bucket IDs
	if orBkId := opArgs.GetBucketId(); orBkId != "" && c.opArgs.GetBucketId() != orBkId {
		// acquire the new bucket handle
		var err error
		bkt, rel, err = StartBucketRWOperation(
			ctx,
			c.bus,
			opArgs.CloneVT(),
		)
		if err != nil {
			return nil, err
		}
	}

	// fetch the transform config block, if set.
	// use the previous bucket ref (transformed) to fetch it
	// wrap bkRaw with the result
	applyTransformConf := func(bc *block_transform.Config) error {
		if transformConf.EqualVT(bc) {
			// no-op equiv to old config
			return nil
		}

		blockXfrm, err := block_transform.NewTransformer(
			controller.ConstructOpts{Logger: c.le},
			c.sfs,
			bc,
		)
		if err != nil {
			return err
		}
		transformConf, xfrm = bc, blockXfrm
		return nil
	}

	// check if transform config changed
	var err error
	oldTconfRef := c.ref.GetTransformConfRef()
	refTconfRef := objRef.GetTransformConfRef()
	refTconf := objRef.GetTransformConf()
	if !refTconf.GetEmpty() {
		// apply in-line transform config
		err = applyTransformConf(refTconf)
	} else if !refTconfRef.GetEmpty() {
		// referenced config: check if references are equal
		if oldTconfRef.GetEmpty() || !oldTconfRef.EqualsRef(refTconfRef) {
			// transform config ref changed, fetch new transform config
			// use old transformer to transform the conf
			var bc *block_transform.Config
			bc, err = FetchTransformConf(ctx, bkt, refTconfRef, xfrm)
			if err == nil {
				err = applyTransformConf(bc)
			}
		}
	}

	// handle any error from the above operation
	if err != nil {
		if rel != nil {
			rel()
		}
		return nil, err
	}

	// return new cursor
	ncc := c.clone()
	ncc.bkt = bkt
	ncc.xfrm = xfrm
	ncc.ref = objRef.Clone()
	ncc.transformConf = transformConf
	ncc.rel = rel
	ncc.opArgs = opArgs

	return ncc, nil
}

// SetRootRef sets the cursor's root ref.
func (c *Cursor) SetRootRef(b *block.BlockRef) {
	if c.ref == nil {
		c.ref = &bucket.ObjectRef{}
	}
	c.ref.RootRef = b
}

// SetRootRefBucket sets the cursor's root ref bucket.
// Note: this does not actually traverse to the bucket.
func (c *Cursor) SetRootRefBucket(b string) {
	if c.ref == nil {
		c.ref = &bucket.ObjectRef{}
	}
	c.ref.BucketId = b
}

// GetBucket returns the bucket. Note: transform config is not applied.
func (c *Cursor) GetBucket() bucket.Bucket {
	return c.bkt
}

// GetTransformer returns the bucket transformer.
// May return nil.
func (c *Cursor) GetTransformer() block.Transformer {
	return c.xfrm
}

// PutBlock puts a block into the store, applying any configured transforms.
// The ref should not be modified after return.
// The second return value can optionally indicate if the block already existed.
// If the hash type is unset, use the type from GetHashType().
func (c *Cursor) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	var err error
	if c.xfrm != nil {
		// we have to copy the data since EncodeBlock might reuse the buffer.
		dataOrig := data
		data = make([]byte, len(dataOrig))
		copy(data, dataOrig)
		data, err = c.xfrm.EncodeBlock(data)
		if err != nil {
			return nil, false, err
		}
	}
	return c.bkt.PutBlock(ctx, data, opts)
}

// GetBlock gets a block with a cid reference, applying any configured transforms.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (c *Cursor) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := c.bkt.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, found, err
	}
	if c.xfrm != nil {
		data, err = c.xfrm.DecodeBlock(data)
		if err != nil {
			return nil, true, err
		}
	}
	return data, true, nil
}

// GetRef returns a copy of the current object ref.
func (c *Cursor) GetRef() *bucket.ObjectRef {
	if c.ref == nil {
		return &bucket.ObjectRef{}
	}
	return c.ref.Clone()
}

// GetOpArgs returns a copy of the current operation args.
func (c *Cursor) GetOpArgs() *bucket.BucketOpArgs {
	if c.opArgs == nil {
		return &bucket.BucketOpArgs{}
	}
	return c.opArgs.CloneVT()
}

// GetRefWithOpArgs gets the ref and sets the BucketId and TransformConf (if unset).
func (c *Cursor) GetRefWithOpArgs() *bucket.ObjectRef {
	ref := c.ref.Clone()
	if ref.BucketId == "" {
		ref.BucketId = c.opArgs.GetBucketId()
	}
	if ref.TransformConfRef.GetEmpty() {
		ref.TransformConf = c.transformConf.Clone()
	}
	return ref
}

// GetTransformConf returns the current transform config.
func (c *Cursor) GetTransformConf() *block_transform.Config {
	return c.transformConf
}

// GetStepFactorySet returns the step factory set for the cursor.
func (c *Cursor) GetStepFactorySet() (sfs *block_transform.StepFactorySet) {
	return c.sfs
}

// SetStepFactorySet sets the step factory set for the cursor.
func (c *Cursor) SetStepFactorySet(sfs *block_transform.StepFactorySet) {
	c.sfs = sfs
}

// Unmarshal unmarshals a block at the position.
// Returns nil if the ref is empty or the block not found.
func (c *Cursor) Unmarshal(
	ctx context.Context,
	ctor func() block.Block,
) (block.Block, error) {
	rr := c.ref.GetRootRef()
	if rr.GetEmpty() {
		return nil, nil
	}

	data, ok, err := c.bkt.GetBlock(ctx, rr)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	if c.xfrm != nil {
		data, err = c.xfrm.DecodeBlock(data)
		if err != nil {
			return nil, err
		}
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
	if c != nil && c.rel != nil {
		c.rel()
	}
}

// clone returns a base copy of the cursor
func (c *Cursor) clone() *Cursor {
	return &Cursor{
		le:            c.le,
		sfs:           c.sfs,
		bus:           c.bus,
		transformConf: c.transformConf.Clone(),
		bkt:           c.bkt,
		xfrm:          c.xfrm,
		ref:           c.ref.Clone(),
		opArgs:        c.opArgs.CloneVT(),
	}
}
