package engine

import (
	"bytes"
	"context"
	"slices"
)

// keyRange is a half-open committed key interval; a nil upper is unbounded.
type keyRange struct {
	// lower is the inclusive first key.
	lower []byte
	// upper is the exclusive end key.
	upper []byte
}

// contains reports whether key falls within the range.
func (r keyRange) contains(key []byte) bool {
	return bytes.Compare(key, r.lower) >= 0 && (r.upper == nil || bytes.Compare(key, r.upper) < 0)
}

// overlaps reports whether the range intersects [lower, upper).
func (r keyRange) overlaps(lower, upper []byte) bool {
	return (r.upper == nil || bytes.Compare(lower, r.upper) < 0) && (upper == nil || bytes.Compare(r.lower, upper) < 0)
}

// readSet records the committed ranges one transaction observed in its snapshot.
// A commit conflicts only when a later generation changed one of these ranges.
type readSet struct {
	// ranges lists every observed interval, including absent point keys.
	ranges []keyRange
	// size charges the retained range bounds.
	size int
	// overflow replaces the ranges with the whole store once size exceeds its bound.
	overflow bool
}

// add records an observed interval within the transaction memory bound.
func (s *readSet) add(lower, upper []byte) {
	if s.overflow {
		return
	}
	s.size += len(lower) + len(upper) + 48
	if s.size > maxBatchBytes {
		s.overflow, s.ranges = true, nil
		return
	}
	s.ranges = append(s.ranges, keyRange{lower: bytes.Clone(lower), upper: bytes.Clone(upper)})
}

// addKey records one point read, which also observes the key's absence.
func (s *readSet) addKey(key []byte) {
	s.add(key, append(bytes.Clone(key), 0))
}

// conflicts reports whether current changed any range observed in before.
// The caller holds publication authority and file protection for both roots.
func (e *Engine) conflicts(ctx context.Context, reads *readSet, before, current *Root) (bool, error) {
	if before.Revision == current.Revision {
		return false, nil
	}
	if reads.overflow {
		return true, nil
	}
	c := &comparison{
		before:  &snapshot{engine: e, root: before},
		current: &snapshot{engine: e, root: current},
		entries: make(map[*Partition][]*Record),
	}
	for _, r := range reads.ranges {
		changed, err := c.changed(ctx, r)
		if err != nil || changed {
			return changed, err
		}
	}
	return false, nil
}

// comparison compares key ranges across two generations, merging each
// differing partition at most once.
type comparison struct {
	// before is the transaction's observed generation.
	before *snapshot
	// current is the durable generation under publication authority.
	current *snapshot
	// entries caches the live records of merged partitions.
	entries map[*Partition][]*Record
}

// changed reports whether the live records within r differ between generations.
// Partitions with identical bounds and immutable runs hold identical records.
func (c *comparison) changed(ctx context.Context, r keyRange) (bool, error) {
	before, err := c.before.partitions(ctx, c.before.root.Catalogue, r, nil)
	if err != nil {
		return false, err
	}
	current, err := c.current.partitions(ctx, c.current.root.Catalogue, r, nil)
	if err != nil {
		return false, err
	}
	if slices.EqualFunc(before, current, (*Partition).EqualVT) {
		return false, nil
	}
	beforeRecords, err := c.records(ctx, c.before, before, r)
	if err != nil {
		return false, err
	}
	currentRecords, err := c.records(ctx, c.current, current, r)
	if err != nil {
		return false, err
	}
	return !slices.EqualFunc(beforeRecords, currentRecords, (*Record).EqualVT), nil
}

// records returns the ordered live records within r from the given partitions.
func (c *comparison) records(ctx context.Context, s *snapshot, partitions []*Partition, r keyRange) ([]*Record, error) {
	var out []*Record
	for _, partition := range partitions {
		entries, ok := c.entries[partition]
		if !ok {
			var err error
			entries, err = s.partitionEntries(ctx, partition, nil, false, false)
			if err != nil {
				return nil, err
			}
			c.entries[partition] = entries
		}
		for _, record := range entries {
			if r.contains(record.Key) {
				out = append(out, record)
			}
		}
	}
	return out, nil
}

// partitions appends the ordered partitions whose bounds intersect r.
func (s *snapshot) partitions(ctx context.Context, name string, r keyRange, out []*Partition) ([]*Partition, error) {
	page, err := s.engine.readCatalogue(ctx, name)
	if err != nil {
		return nil, err
	}
	for i, child := range page.Children {
		var upper []byte
		if i+1 < len(page.Children) {
			upper = page.Children[i+1].Lower
		}
		if !r.overlaps(child.Lower, upper) {
			continue
		}
		out, err = s.partitions(ctx, child.File, r, out)
		if err != nil {
			return nil, err
		}
	}
	for i, partition := range page.Partitions {
		var upper []byte
		if i+1 < len(page.Partitions) {
			upper = page.Partitions[i+1].Lower
		}
		if r.overlaps(partition.Lower, upper) {
			out = append(out, partition)
		}
	}
	return out, nil
}
