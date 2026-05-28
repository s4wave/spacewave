package blockshard

import (
	"bytes"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

// MergeSegments performs a k-way merge of segment entries.
// Input readers are ordered oldest-first (index 0 = oldest).
// Newest entry wins per key. Tombstones suppress older values.
func MergeSegments(readers []*segment.Reader) ([]segment.Entry, error) {
	iters := make([]*segment.EntryIterator, len(readers))
	for i := range readers {
		iters[i] = readers[i].Entries()
	}

	var result []segment.Entry
	err := mergeSegmentIterators(iters, func(entry segment.Entry) error {
		result = append(result, entry)
		return nil
	})
	return result, err
}

func mergeSegmentIterators(
	iters []*segment.EntryIterator,
	emit func(segment.Entry) error,
) error {
	sources := make([]mergeSegmentSource, len(iters))
	active := 0
	for i := range iters {
		sources[i] = mergeSegmentSource{
			iter:     iters[i],
			segIndex: i,
		}
		if err := sources[i].advance(); err != nil {
			return errors.Errorf("read segment %d: %v", i, err)
		}
		if sources[i].ok {
			active++
		}
	}

	for active != 0 {
		minIdx := -1
		for i := range sources {
			if !sources[i].ok {
				continue
			}
			if minIdx < 0 || bytes.Compare(sources[i].entry.Key, sources[minIdx].entry.Key) < 0 {
				minIdx = i
			}
		}
		if minIdx < 0 {
			break
		}
		key := sources[minIdx].entry.Key
		best := segment.Entry{}
		bestSeg := -1
		for i := range sources {
			for sources[i].ok && bytes.Equal(sources[i].entry.Key, key) {
				if sources[i].segIndex >= bestSeg {
					best = sources[i].entry
					bestSeg = sources[i].segIndex
				}
				wasActive := sources[i].ok
				if err := sources[i].advance(); err != nil {
					return errors.Errorf("read segment %d: %v", i, err)
				}
				if wasActive && !sources[i].ok {
					active--
				}
			}
		}
		if bestSeg < 0 {
			return errors.New("merge did not select an entry")
		}
		if err := emit(best); err != nil {
			return err
		}
	}
	return nil
}

type mergeSegmentSource struct {
	iter     *segment.EntryIterator
	segIndex int
	entry    segment.Entry
	ok       bool
}

func (s *mergeSegmentSource) advance() error {
	entry, ok, err := s.iter.Next()
	if err != nil {
		return err
	}
	s.entry = entry
	s.ok = ok
	return nil
}
