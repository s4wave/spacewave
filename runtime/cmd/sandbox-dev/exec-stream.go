package main

import (
	"io"
)

// stdioStream implements the stdin/stdout streaming
type stdioStream struct {
	io.Reader
	io.Writer
}

// _ is a type assertion
var _ io.ReadWriter = ((*stdioStream)(nil))
