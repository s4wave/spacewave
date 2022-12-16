package block_transform

import (
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// Transformer is constructed using a factory set and a configuration, and
// implements a bucket interface.
type Transformer struct {
	steps []Step
	store block.Store
}

// NewTransformer constructs a new transformer from a factory set and a config.
func NewTransformer(
	copts controller.ConstructOpts,
	fs *StepFactorySet,
	c *Config,
	b block.Store,
) (*Transformer, error) {
	t := &Transformer{store: b}
	t.steps = make([]Step, len(c.GetSteps()))
	for i, s := range c.GetSteps() {
		if fs == nil {
			return nil, errors.New("no transform step factory set")
		}
		tf := fs.GetStepFactoryByConfigID(s.GetId())
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
	opts *block.PutOpts,
) (*block.BlockRef, bool, error) {
	bd, err := t.EncodeBlock(data)
	if err != nil {
		return nil, false, err
	}

	return t.store.PutBlock(bd, opts)
}

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (t *Transformer) GetBlock(ref *block.BlockRef) ([]byte, bool, error) {
	dat, ok, err := t.store.GetBlock(ref)
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
func (t *Transformer) GetBlockExists(ref *block.BlockRef) (bool, error) {
	return t.store.GetBlockExists(ref)
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (t *Transformer) RmBlock(ref *block.BlockRef) error {
	return t.store.RmBlock(ref)
}

// _ is a type assertion
var (
	_ Step        = ((*Transformer)(nil))
	_ block.Store = ((*Transformer)(nil))
)
