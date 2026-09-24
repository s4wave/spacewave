package git_lfs

import (
	"context"
	"io"
)

// Store holds Git LFS objects addressed by their SHA-256 oid.
//
// The Agent validates every oid before calling a Store, so an oid is always 64
// lowercase hexadecimal characters.
type Store interface {
	// Has reports whether the Store holds oid with exactly size bytes.
	Has(ctx context.Context, oid string, size int64) (bool, error)
	// Put stores size bytes read from rdr as oid, replacing an entry of
	// another size. The object becomes visible only when Put succeeds.
	Put(ctx context.Context, oid string, size int64, rdr io.Reader) error
	// Get writes the size bytes of oid to w in order.
	Get(ctx context.Context, oid string, size int64, w io.Writer) error
}
