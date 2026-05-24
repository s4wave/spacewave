package syncprogress

import (
	"io"
	"time"
)

const interval = 250 * time.Millisecond

// Reader wraps an io.Reader and fires a progress callback while bytes pass through.
type Reader struct {
	reader io.Reader
	sent   int64
	next   int64
	cb     func(int64)
}

// NewReader creates a progress reader.
func NewReader(reader io.Reader, cb func(int64)) *Reader {
	return &Reader{
		reader: reader,
		cb:     cb,
		next:   time.Now().Add(interval).UnixNano(),
	}
}

// Read reads from the wrapped reader.
func (p *Reader) Read(buf []byte) (int, error) {
	n, err := p.reader.Read(buf)
	if n > 0 {
		p.sent += int64(n)
		now := time.Now().UnixNano()
		if now >= p.next {
			p.next = now + int64(interval)
			p.cb(p.sent)
		}
	}
	if err == io.EOF {
		p.cb(p.sent)
	}
	return n, err
}

// _ is a type assertion.
var _ io.Reader = ((*Reader)(nil))
