package transform_s2

import (
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/klauspost/compress/s2"
)

// S2 is the S2 compression step.
type S2 struct {
}

// NewS2 constructs the s2 compress step.
func NewS2(c *Config) (*S2, error) {
	return &S2{}, nil
}

// EncodeBlock encodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *S2) EncodeBlock(data []byte) ([]byte, error) {
	return s2.Encode(nil, data), nil
}

// DecodeBlock decodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *S2) DecodeBlock(data []byte) ([]byte, error) {
	return s2.Decode(nil, data)
}

// _ is a type assertion
var _ block_transform.Step = ((*S2)(nil))
