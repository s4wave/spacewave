package transform_snappy

import (
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/klauspost/compress/snappy"
)

// Snappy is the Snappy compression step.
type Snappy struct{}

// NewSnappy constructs the snappy step.
func NewSnappy(c *Config) (*Snappy, error) {
	return &Snappy{}, nil
}

// EncodeBlock encodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *Snappy) EncodeBlock(data []byte) ([]byte, error) {
	return snappy.Encode(nil, data), nil
}

// DecodeBlock decodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *Snappy) DecodeBlock(data []byte) ([]byte, error) {
	return snappy.Decode(nil, data)
}

// _ is a type assertion
var _ block_transform.Step = ((*Snappy)(nil))
