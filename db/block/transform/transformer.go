package block_transform

import (
	"slices"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/db/block"
	"github.com/pkg/errors"
)

// Transformer is constructed using a factory set and a configuration.
type Transformer struct {
	steps []Step
}

// NewTransformer constructs a new transformer from a factory set and a config.
func NewTransformer(
	copts controller.ConstructOpts,
	fs *StepFactorySet,
	c *Config,
) (*Transformer, error) {
	steps := make([]Step, len(c.GetSteps()))
	for i, s := range c.GetSteps() {
		if fs == nil {
			return nil, errors.New("no transform step factory set")
		}
		cc, tf, err := fs.UnmarshalStepConfig(s)
		if err != nil {
			return nil, errors.Wrapf(err, "step[%d]", i)
		}
		s, err := tf.Construct(cc, copts)
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"step[%d]: construct",
				i,
			)
		}
		steps[i] = s
	}
	return NewTransformerWithSteps(steps), nil
}

// NewTransformerWithSteps constructs a new transformer with the given steps.
func NewTransformerWithSteps(steps []Step) *Transformer {
	return &Transformer{steps: steps}
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
	for _, v := range slices.Backward(t.steps) {
		s := v
		data, err = s.DecodeBlock(data)
		if err != nil {
			return nil, err
		}
	}

	return data, nil
}

// _ is a type assertion
var (
	_ Step              = ((*Transformer)(nil))
	_ block.Transformer = ((*Transformer)(nil))
)
