package block_transform

import (
	"crypto/sha256"
	"encoding/base64"
	"slices"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

const decodedBlockCacheTransformConfigKeyPrefix = "transform:config:v1:"

// Transformer is constructed using a factory set and a configuration.
type Transformer struct {
	steps                         []Step
	decodedBlockCacheTransformKey string
}

// NewTransformer constructs a new transformer from a factory set and a config.
func NewTransformer(
	copts controller.ConstructOpts,
	fs *StepFactorySet,
	c *Config,
) (*Transformer, error) {
	// Construct each configured transform step in order.
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

	// Derive the cache identity after all steps are constructed.
	key, err := decodedBlockCacheTransformKeyForConfig(c)
	if err != nil {
		return nil, err
	}
	return &Transformer{
		steps:                         steps,
		decodedBlockCacheTransformKey: key,
	}, nil
}

// NewTransformerWithSteps constructs a new transformer with the given steps.
func NewTransformerWithSteps(steps []Step) *Transformer {
	var key string
	if len(steps) == 0 {
		key = block.DecodedBlockCacheNoTransformKey
	}
	return &Transformer{
		steps:                         steps,
		decodedBlockCacheTransformKey: key,
	}
}

// DecodedBlockCacheTransformKey returns the exact decoded-cache transform identity.
func (t *Transformer) DecodedBlockCacheTransformKey() string {
	if t == nil {
		return ""
	}
	return t.decodedBlockCacheTransformKey
}

// EncodeBlock encodes the block according to the config.
// May reuse the same byte slice if possible.
func (t *Transformer) EncodeBlock(data []byte) ([]byte, error) {
	// Apply transforms in declaration order for encoding.
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
	// Leave untransformed data untouched when no decode steps exist.
	if len(t.steps) == 0 {
		return data, nil
	}

	// Reverse the configured steps to restore the original block.
	var err error
	for _, s := range slices.Backward(t.steps) {
		data, err = s.DecodeBlock(data)
		if err != nil {
			return nil, err
		}
	}

	return data, nil
}

func decodedBlockCacheTransformKeyForConfig(c *Config) (string, error) {
	if c == nil || c.GetEmpty() {
		return block.DecodedBlockCacheNoTransformKey, nil
	}
	data, err := c.MarshalVT()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return decodedBlockCacheTransformConfigKeyPrefix + base64.RawStdEncoding.EncodeToString(sum[:]), nil
}

// _ is a type assertion
var (
	_ Step                               = (*Transformer)(nil)
	_ block.Transformer                  = (*Transformer)(nil)
	_ block.DecodedBlockCacheTransformer = (*Transformer)(nil)
)
