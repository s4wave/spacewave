//go:build js

package blockshard

import (
	"bytes"

	"github.com/aperturerobotics/hydra/volume/js/opfs/segment"
	"github.com/pkg/errors"
)

// LiveStats returns the current live block count and total live bytes.
func (e *Engine) LiveStats() (uint64, uint64, error) {
	var count uint64
	var totalBytes uint64
	for i := range e.shards {
		n, sz, err := e.shards[i].liveStats()
		if err != nil {
			return 0, 0, err
		}
		count += n
		totalBytes += sz
	}
	return count, totalBytes, nil
}

func (s *Shard) liveStats() (uint64, uint64, error) {
	m := s.Manifest()
	if len(m.Segments) == 0 {
		return 0, 0, nil
	}

	readers := make([]*segment.Reader, len(m.Segments))
	for i := range m.Segments {
		data := readFileBytes(s.dir, m.Segments[i].Filename)
		if data == nil {
			return 0, 0, errors.Errorf("read segment %s for stats: not found", m.Segments[i].Filename)
		}
		rd, err := segment.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return 0, 0, errors.Errorf("parse segment %s for stats: %v", m.Segments[i].Filename, err)
		}
		readers[i] = rd
	}

	merged, err := MergeSegments(readers)
	if err != nil {
		return 0, 0, errors.Wrap(err, "merge segments for stats")
	}

	var count uint64
	var totalBytes uint64
	for i := range merged {
		if merged[i].Tombstone {
			continue
		}
		count++
		totalBytes += uint64(len(merged[i].Value))
	}
	return count, totalBytes, nil
}
