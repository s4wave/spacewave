package blockshard

import (
	"bytes"
	"sort"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

// MergeSegments performs a k-way merge of segment entries.
// Input readers are ordered oldest-first (index 0 = oldest).
// Newest entry wins per key. Tombstones suppress older values.
func MergeSegments(readers []*segment.Reader) ([]segment.Entry, error) {
	type indexedEntry struct {
		entry    segment.Entry
		segIndex int
	}

	var all []indexedEntry
	for i, rd := range readers {
		entries, err := rd.ReadEntries()
		if err != nil {
			return nil, errors.Errorf("read segment %d: %v", i, err)
		}
		for _, e := range entries {
			all = append(all, indexedEntry{entry: e, segIndex: i})
		}
	}

	// Sort by key, then by segment index descending (newest first for dedup).
	sort.SliceStable(all, func(i, j int) bool {
		cmp := bytes.Compare(all[i].entry.Key, all[j].entry.Key)
		if cmp != 0 {
			return cmp < 0
		}
		return all[i].segIndex > all[j].segIndex
	})

	// Deduplicate: keep newest per key.
	var result []segment.Entry
	var prevKey []byte
	for _, ie := range all {
		if bytes.Equal(ie.entry.Key, prevKey) {
			continue
		}
		prevKey = ie.entry.Key
		result = append(result, ie.entry)
	}

	return result, nil
}
