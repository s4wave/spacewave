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
	Level                     uint8
	SegmentCount              uint64
	SegmentBytes              uint64
	EntryCount                uint64
	LiveEntryCount            uint64
	TombstoneCount            uint64
	PendingDeleteSegmentCount uint64
	PendingDeleteBytes        uint64
}

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

// LevelStats returns active segment, byte, entry, and tombstone counts grouped
// by compaction level.
func (e *Engine) LevelStats() ([]LevelStats, error) {
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

	levels := make([]uint8, 0, len(byLevel))
	for level := range byLevel {
		levels = append(levels, level)
	}
	slices.Sort(levels)

	out := make([]LevelStats, 0, len(levels))
	for _, level := range levels {
		out = append(out, *byLevel[level])
	}
	return out, nil
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

func (s *Shard) levelStats() ([]LevelStats, error) {
	m := s.Manifest()
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

	levels := make([]uint8, 0, len(byLevel))
	for level := range byLevel {
		levels = append(levels, level)
	}
	slices.Sort(levels)

	out := make([]LevelStats, 0, len(levels))
	for _, level := range levels {
		out = append(out, *byLevel[level])
	}
	return out, nil
}

func (s *Shard) readSegmentEntries(seg *SegmentMeta) ([]segment.Entry, error) {
	data := readFileBytes(s.dir, seg.Filename)
	if data == nil {
		return nil, errors.Errorf("read segment %s for stats: not found", seg.Filename)
	}
	rd, err := segment.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.Errorf("parse segment %s for stats: %v", seg.Filename, err)
	}
	return rd.ReadEntries()
}

func levelStatsEntry(stats map[uint8]*LevelStats, level uint8) *LevelStats {
	stat := stats[level]
	if stat == nil {
		stat = &LevelStats{Level: level}
		stats[level] = stat
	}
	return stat
}
