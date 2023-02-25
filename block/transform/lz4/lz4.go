package transform_lz4

import (
	"bytes"
	"io"

	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/pierrec/lz4/v4"
)

// LZ4 is the LZ4 compression step.
type LZ4 struct {
	opts []lz4.Option
}

// NewLZ4 constructs the s2 compress step.
func NewLZ4(c *Config) (*LZ4, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	opts := c.ToOptions()
	return &LZ4{opts: opts}, nil
}

// EncodeBlock encodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *LZ4) EncodeBlock(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(len(data))
	wr := lz4.NewWriter(&buf)
	if err := wr.Apply(s.opts...); err != nil {
		return nil, err
	}
	if err := wr.Apply(lz4.SizeOption(uint64(len(data)))); err != nil {
		return nil, err
	}
	_, err := wr.Write(data) // note: writes all of data in 1 call
	if err != nil {
		return nil, err
	}
	if err := wr.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecodeBlock decodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *LZ4) DecodeBlock(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(len(data))
	// note: reader does not accept any options.
	rd := lz4.NewReader(bytes.NewReader(data))
	_, err := rd.WriteTo(&buf)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return buf.Bytes(), nil
}

// _ is a type assertion
var _ block_transform.Step = ((*LZ4)(nil))
