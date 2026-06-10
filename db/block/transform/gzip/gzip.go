package transform_gzip

import (
	"bytes"
	"compress/gzip"
	"io"

	block_transform "github.com/s4wave/spacewave/db/block/transform"
)

// Gzip is the gzip compression step.
type Gzip struct {
	level int
}

// NewGzip constructs the gzip compression step.
func NewGzip(c *Config) (*Gzip, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &Gzip{level: c.EffectiveCompressionLevel()}, nil
}

// EncodeBlock encodes the block according to the config.
// May reuse the same byte slice if possible.
func (g *Gzip) EncodeBlock(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(len(data))
	wr, err := gzip.NewWriterLevel(&buf, g.level)
	if err != nil {
		return nil, err
	}
	if _, err := wr.Write(data); err != nil {
		_ = wr.Close()
		return nil, err
	}
	if err := wr.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecodeBlock decodes the block according to the config.
// May reuse the same byte slice if possible.
func (g *Gzip) DecodeBlock(data []byte) ([]byte, error) {
	rd, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer rd.Close()

	var buf bytes.Buffer
	buf.Grow(len(data))
	if _, err := io.Copy(&buf, rd); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// _ is a type assertion
var _ block_transform.Step = ((*Gzip)(nil))
