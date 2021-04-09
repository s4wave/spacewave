package sizer

import (
	"io"
	"sync/atomic"
)

// Sizer implements io methods keeping total size metrics.
type Sizer struct {
	total uint64
	rdr   io.Reader
	wtr   io.Writer
}

// NewSizer constructs a sizer with optional reader and/or writer.
func NewSizer(rdr io.Reader, writer io.Writer) *Sizer {
	return &Sizer{rdr: rdr, wtr: writer}
}

// TotalSize returns the total amount of data transferred.
func (s *Sizer) TotalSize() uint64 {
	return atomic.LoadUint64(&s.total)
}

// Read reads data from the source.
func (s *Sizer) Read(p []byte) (n int, err error) {
	if s.rdr == nil {
		return 0, io.EOF
	}
	n, err = s.rdr.Read(p)
	if n != 0 {
		atomic.AddUint64(&s.total, uint64(n))
	}
	return
}

// Write writes data to the writer.
func (s *Sizer) Write(p []byte) (n int, err error) {
	if s.wtr == nil {
		return 0, io.EOF
	}
	n, err = s.wtr.Write(p)
	if n != 0 {
		atomic.AddUint64(&s.total, uint64(n))
	}
	return
}

// _ is a type assertion
var _ io.ReadWriter = ((*Sizer)(nil))
