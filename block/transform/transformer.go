package block_transform

import (
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/event"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// Transformer is constructed using a factory set and a configuration, and
// implements a bucket interface.
type Transformer struct {
	steps []Step
	bkt   bucket.Bucket
}

// NewTransformer constructs a new transformer from a factory set and a config.
// Applies automatic padding to multiples of 32.
// Applies a 1-byte trailer with the # of padding bytes.
func NewTransformer(
	copts controller.ConstructOpts,
	fs *StepFactorySet,
	c *Config,
	b bucket.Bucket,
) (*Transformer, error) {
	t := &Transformer{bkt: b}
	t.steps = make([]Step, len(c.GetSteps()))
	for i, s := range c.GetSteps() {
		tf := fs.GetFactoryByConfigID(s.GetId())
		if tf == nil {
			return nil, errors.Errorf(
				"step[%d]: transform unknown: %s",
				i,
				s.GetId(),
			)
		}
		cc := tf.ConstructConfig()
		if err := proto.Unmarshal(s.GetConfig(), cc); err != nil {
			return nil, errors.Errorf(
				"step[%d]: config invalid: %s",
				i,
				err.Error(),
			)
		}
		s, err := tf.Construct(cc, copts)
		if err != nil {
			return nil, errors.Errorf(
				"step[%d]: construct: %s",
				i,
				err.Error(),
			)
		}
		t.steps[i] = s
	}
	return t, nil
}

// GetBucketConfig returns a copy of the bucket configuration.
func (t *Transformer) GetBucketConfig() *bucket.Config {
	return t.bkt.GetBucketConfig()
}

// EncodeBlock encodes the block according to the config.
// May reuse the same byte slice if possible.
func (t *Transformer) EncodeBlock(data []byte) ([]byte, error) {
	var err error
	for _, s := range t.steps {
		data, err = s.EncodeBlock(data)
		if err != nil {
			return nil, err
		}
	}

	return data, nil
}

// DecodeBlock decodes the block according to the config.
// May reuse the same byte slice if possible.
func (t *Transformer) DecodeBlock(data []byte) ([]byte, error) {
	if len(t.steps) == 0 {
		return data, nil
	}

	var err error
	for i := len(t.steps) - 1; i >= 0; i-- {
		s := t.steps[i]
		data, err = s.DecodeBlock(data)
		if err != nil {
			return nil, err
		}
	}

	return data, nil
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
func (t *Transformer) PutBlock(
	data []byte,
	opts *bucket.PutOpts,
) (*bucket_event.PutBlock, error) {
	bd, err := t.EncodeBlock(data)
	if err != nil {
		return nil, err
	}

	return t.bkt.PutBlock(bd, opts)
}

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (t *Transformer) GetBlock(ref *cid.BlockRef) ([]byte, bool, error) {
	dat, ok, err := t.bkt.GetBlock(ref)
	if !ok || err != nil {
		return nil, ok, err
	}

	bd, err := t.DecodeBlock(dat)
	if err != nil {
		return nil, ok, err
	}
	return bd, true, nil
}

// GetBlockExists checks if a block exists with a cid reference.
// Note: the block may not be in the specified bucket.
func (t *Transformer) GetBlockExists(ref *cid.BlockRef) (bool, error) {
	return t.bkt.GetBlockExists(ref)
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (t *Transformer) RmBlock(ref *cid.BlockRef) error {
	return t.bkt.RmBlock(ref)
}

// _ is a type assertion
var _ Step = ((*Transformer)(nil))

// _ is a type assertion
var _ bucket.Bucket = ((*Transformer)(nil))
