package blockenc

import "sync"

// AllocFn allocates a buffer for use.
// The cap of the slice must be at least n.
// This can be backed by an in-memory arena.
type AllocFn func(n int) []byte

// NewAllocFn constructs the default allocate func.
func NewAllocFn() AllocFn {
	return func(n int) []byte {
		return make([]byte, n)
	}
}

// CallAllocFn calls an alloc function, checking the result.
// If the result is invalid, allocates a new buf in memory.
func CallAllocFn(allocFn AllocFn, n int) []byte {
	v := allocFn(n)
	if cap(v) < n {
		return make([]byte, n)
	}
	if len(v) != n {
		v = v[:n]
	}
	return v
}

// NewPoolAlloc constructs a new pool alloc fn.
// call relBuf with the buffer when done.
// don't read or write to the buffer after calling relBuf.
func NewPoolAlloc() (allocFn AllocFn, relBuf func(b []byte)) {
	pool := sync.Pool{}
	defAlloc := NewAllocFn()
	return func(n int) []byte {
			var out []byte
			for cap(out) < n {
				gv := pool.Get()
				if gv == nil {
					return defAlloc(n)
				}
				out = gv.([]byte)
			}
			return out[:n]
		}, func(b []byte) {
			if cap(b) != 0 {
				// scrub entire buffer
				b = b[:cap(b)]
				// compiler optimizes to memset
				for i := 0; i < len(b); i++ {
					b[i] = 0
				}
				pool.Put(b)
			}
		}
}
