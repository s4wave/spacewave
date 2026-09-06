//go:build js

package blockshard

import (
	"bytes"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

// LevelStats describes active and retired segment stats for one compaction
// level.
type LevelStats struct {
	// Level identifies the compaction level.
	Level uint8
	// SegmentCount counts active segments.
	SegmentCount uint64
	// SegmentBytes sums active segment file sizes.
	SegmentBytes uint64
	// EntryCount counts active entries including tombstones.
	EntryCount uint64
	// LiveEntryCount counts active entries with values.
	LiveEntryCount uint64
	// TombstoneCount counts active tombstones.
	TombstoneCount uint64
	// PendingDeleteSegmentCount counts retired segments awaiting reclamation.
	PendingDeleteSegmentCount uint64
	// PendingDeleteBytes sums retired segment file sizes.
	PendingDeleteBytes uint64
}

// StorageStats returns the current active segment entry count and byte size.
func (e *Engine) StorageStats() (uint64, uint64) {
	var count uint64
	var totalBytes uint64
	for i := range e.shards {
		n, sz := e.shards[i].storageStats()
		count += n
		totalBytes += sz
	}
	return count, totalBytes
}

// LevelStats returns active segment, byte, entry, and tombstone counts grouped
// by compaction level.
func (e *Engine) LevelStats() ([]LevelStats, error) {
	// Combine per-shard counters by compaction level.
	byLevel := make(map[uint8]*LevelStats)
	for i := range e.shards {
		stats, err := e.shards[i].levelStats()
		if err != nil {
			return nil, err
		}
		for _, stat := range stats {
			dst := byLevel[stat.Level]
			if dst == nil {
				dst = &LevelStats{Level: stat.Level}
				byLevel[stat.Level] = dst
			}
			dst.SegmentCount += stat.SegmentCount
			dst.SegmentBytes += stat.SegmentBytes
			dst.EntryCount += stat.EntryCount
			dst.LiveEntryCount += stat.LiveEntryCount
			dst.TombstoneCount += stat.TombstoneCount
			dst.PendingDeleteSegmentCount += stat.PendingDeleteSegmentCount
			dst.PendingDeleteBytes += stat.PendingDeleteBytes
		}
	}

	// Emit levels in ascending order.
	levels := make([]uint8, 0, len(byLevel))
	for level := range byLevel {
		levels = append(levels, level)
	}
	slices.Sort(levels)

	// Copy accumulator values into the result.
	out := make([]LevelStats, 0, len(levels))
	for _, level := range levels {
		out = append(out, *byLevel[level])
	}
	return out, nil
}

// storageStats totals active storage from one immutable generation.
func (s *Shard) storageStats() (uint64, uint64) {
	m := s.manifestSnapshot()
	var count uint64
	var totalBytes uint64
	for i := range m.Segments {
		seg := &m.Segments[i]
		count += uint64(seg.EntryCount)
		totalBytes += uint64(seg.Size)
	}
	return count, totalBytes
}

// levelStats reads active entries and retirement metadata by compaction level.
func (s *Shard) levelStats() ([]LevelStats, error) {
	// Read segment counters from one coherent generation.
	m := s.manifestSnapshot()
	byLevel := make(map[uint8]*LevelStats)
	for i := range m.Segments {
		seg := &m.Segments[i]
		stat := levelStatsEntry(byLevel, seg.Level)
		stat.SegmentCount++
		stat.SegmentBytes += uint64(seg.Size)
		entries, err := s.readSegmentEntries(seg)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			stat.EntryCount++
			if entry.Tombstone {
				stat.TombstoneCount++
				continue
			}
			stat.LiveEntryCount++
		}
	}
	for i := range m.PendingDelete {
		seg := &m.PendingDelete[i]
		stat := levelStatsEntry(byLevel, seg.Level)
		stat.PendingDeleteSegmentCount++
		stat.PendingDeleteBytes += uint64(seg.Size)
	}

	// Emit levels in ascending order.
	levels := make([]uint8, 0, len(byLevel))
	for level := range byLevel {
		levels = append(levels, level)
	}
	slices.Sort(levels)

	// Copy accumulator values into the result.
	out := make([]LevelStats, 0, len(levels))
	for _, level := range levels {
		out = append(out, *byLevel[level])
	}
	return out, nil
}

// readSegmentEntries verifies and decodes a segment for diagnostics.
func (s *Shard) readSegmentEntries(seg *SegmentMeta) ([]segment.Entry, error) {
	// Read the immutable segment bytes for checksum validation.
	data := readFileBytes(s.dir, seg.Filename)
	if data == nil {
		return nil, errors.Errorf("read segment %s for stats: not found", seg.Filename)
	}

	// Decode entries only after the segment reader validates the file.
	rd, err := segment.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.Errorf("parse segment %s for stats: %v", seg.Filename, err)
	}
	return rd.ReadEntries()
}

// levelStatsEntry returns the accumulator for a level, creating it when absent.
func levelStatsEntry(stats map[uint8]*LevelStats, level uint8) *LevelStats {
	stat := stats[level]
	if stat == nil {
		stat = &LevelStats{Level: level}
		stats[level] = stat
	}
	return stat
}
