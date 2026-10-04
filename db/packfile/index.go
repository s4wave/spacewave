package packfile

import (
	"io"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
)

// ReadIndex reads the key index of a packfile of size bytes from ra, without
// its block values. The entries are in key order.
func ReadIndex(ra io.ReaderAt, size uint64) ([]*kvfile.IndexEntry, error) {
	// Read the index at the tail of the packfile.
	_, tail, err := kvfile.ReadIndexTail(ra, size)
	if err != nil {
		return nil, errors.Wrap(err, "read packfile index")
	}
	rd, err := kvfile.BuildReaderWithIndexTail(tail, size)
	if err != nil {
		return nil, errors.Wrap(err, "open packfile index")
	}

	// Collect its entries.
	var index []*kvfile.IndexEntry
	err = rd.ScanPrefixEntries(nil, func(ie *kvfile.IndexEntry, _ int) error {
		index = append(index, ie.CloneVT())
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "scan packfile index")
	}
	return index, nil
}
