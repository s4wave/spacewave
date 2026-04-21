package transform_s2

import (
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/klauspost/compress/s2"
)

// S2 is the S2 compression step.
type S2 struct {
	better bool
	best   bool
}

// NewS2 constructs the s2 compress step.
func NewS2(c *Config) (*S2, error) {
	return &S2{
		better: c.GetBetter(),
		best:   c.GetBest(),
	}, nil
}

// EncodeBlock encodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *S2) EncodeBlock(data []byte) ([]byte, error) {
	switch {
	case s.best:
		return s2.EncodeBest(nil, data), nil
	case s.better:
		return s2.EncodeBetter(nil, data), nil
	default:
		return s2.Encode(nil, data), nil
	}
}

// DecodeBlock decodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *S2) DecodeBlock(data []byte) ([]byte, error) {
	return s2.Decode(nil, data)
}

// _ is a type assertion
var _ block_transform.Step = ((*S2)(nil))
