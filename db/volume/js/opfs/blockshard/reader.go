//go:build js

package blockshard

import (
	"syscall/js"

	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
	"github.com/pkg/errors"
)

// SegmentReader reads entries from a sealed SSTable segment file
// using async getFile().slice() reads. No WebLock or sync handle needed.
type SegmentReader struct {
	file   *opfs.AsyncFile
	size   int64
	lookup *segment.LookupMeta
}

// OpenSegment opens a sealed segment file for reading via async file access.
func OpenSegment(dir js.Value, filename string) (*SegmentReader, error) {
	f, err := opfs.OpenAsyncFile(dir, filename)
	if err != nil {
		return nil, errors.Wrap(err, "open segment file")
	}

	size, err := f.Size()
	if err != nil {
		return nil, errors.Wrap(err, "get segment size")
	}

	lookup, err := segment.LoadLookupMeta(f, size)
	if err != nil {
		return nil, errors.Wrap(err, "load segment lookup metadata")
	}

	sr := &SegmentReader{
		file:   f,
		size:   size,
		lookup: lookup,
	}
	return sr, nil
}

// Get looks up a key in this segment. Returns value, found, error.
// Uses bloom filter for fast rejection, sparse index for window narrowing,
// then linear scan within the data window. All reads are async (no WebLock).
func (sr *SegmentReader) Get(key []byte) ([]byte, bool, error) {
	return sr.lookup.Get(sr.file, key)
}

// MinKey returns the smallest key in the segment.
func (sr *SegmentReader) MinKey() []byte { return sr.lookup.MinKey }

// MaxKey returns the largest key in the segment.
func (sr *SegmentReader) MaxKey() []byte { return sr.lookup.MaxKey }

// EntryCount returns the number of entries.
func (sr *SegmentReader) EntryCount() uint32 { return sr.lookup.Header.EntryCount }
