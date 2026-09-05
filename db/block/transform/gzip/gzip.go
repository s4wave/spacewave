package transform_gzip

import (
	"bytes"
	"compress/gzip"
	"io"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
)

// Gzip is the gzip compression step.
type Gzip struct {
	writers sync.Pool
}

// NewGzip constructs the gzip compression step.
func NewGzip(c *Config) (*Gzip, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	level := c.EffectiveCompressionLevel()
	return &Gzip{writers: sync.Pool{New: func() any {
		// The validated compression level cannot fail writer construction.
		writer, _ := gzip.NewWriterLevel(io.Discard, level)
		return writer
	}}}, nil
}

// EncodeBlock encodes the block according to the config.
// May reuse the same byte slice if possible.
func (g *Gzip) EncodeBlock(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(len(data))
	wr := g.writers.Get().(*gzip.Writer)
	wr.Reset(&buf)
	defer func() {
		// Keep compressor storage, but release the completed block's output buffer.
		wr.Reset(io.Discard)
		g.writers.Put(wr)
	}()
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
	lrd := &io.LimitedReader{R: rd, N: block.MaxBlockSize + 1}
	if _, err := io.Copy(&buf, lrd); err != nil {
		return nil, err
	}
	if lrd.N == 0 {
		return nil, errors.Errorf("gzip decoded block exceeds max block size: %d", block.MaxBlockSize)
	}
	return buf.Bytes(), nil
}

// _ is a type assertion
var _ block_transform.Step = (*Gzip)(nil)
