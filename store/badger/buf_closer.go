package badger

import (
	"bytes"
	"io"
)

type closableBuffer struct {
	bytes.Buffer
}

// Close is a stub to make Buffer satisfy ReadCloser
func (c *closableBuffer) Close() error {
	return nil
}

var _ io.ReadCloser = ((*closableBuffer)(nil))
