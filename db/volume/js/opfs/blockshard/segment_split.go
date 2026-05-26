//go:build js

package blockshard

import "github.com/s4wave/spacewave/db/volume/js/opfs/segment"

type writtenSegment struct {
	Meta   SegmentMeta
	Lookup *segment.LookupMeta
}

func splitSegmentEntries(entries []segment.Entry, maxDataBytes int) [][]segment.Entry {
	if len(entries) == 0 {
		return nil
	}
	if maxDataBytes < 1 {
		return [][]segment.Entry{entries}
	}

	var out [][]segment.Entry
	start := 0
	size := 0
	for i := range entries {
		entrySize := estimateSegmentEntryDataBytes(entries[i])
		if i > start && size+entrySize > maxDataBytes {
			out = append(out, entries[start:i])
			start = i
			size = 0
		}
		size += entrySize
	}
	out = append(out, entries[start:])
	return out
}

func estimateSegmentEntryDataBytes(entry segment.Entry) int {
	size := segment.EntryOverhead + len(entry.Key)
	if !entry.Tombstone {
		size += len(entry.Value)
	}
	return size
}
