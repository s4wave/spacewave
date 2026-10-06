package paylog

import (
	"context"
	"encoding/binary"
	"maps"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume/device"
)

// moveBytes bounds the live payload bytes one maintenance pass moves.
const moveBytes = 8 << 20

// segmentStats counts the published blocks in a segment.
type segmentStats struct {
	// count is the number of blocks.
	count int64
	// bytes is the total payload length.
	bytes int64
}

// add returns the statistics with count blocks of bytes total added.
func (st segmentStats) add(count, bytes int64) segmentStats {
	return segmentStats{count: st.count + count, bytes: st.bytes + bytes}
}

// marshal encodes the statistics as two unsigned varints.
func (st segmentStats) marshal() []byte {
	b := binary.AppendUvarint(nil, uint64(st.count)) //nolint:gosec
	return binary.AppendUvarint(b, uint64(st.bytes)) //nolint:gosec
}

// unmarshalSegmentStats decodes segment statistics.
func unmarshalSegmentStats(b []byte) (segmentStats, error) {
	var v [2]uint64
	for i := range v {
		n, size := binary.Uvarint(b)
		if size <= 0 || n > 1<<62 {
			return segmentStats{}, errors.New("invalid segment statistics")
		}
		v[i], b = n, b[size:]
	}
	return segmentStats{count: int64(v[0]), bytes: int64(v[1])}, nil //nolint:gosec
}

// segmentStatsKey returns the index key of segment n's statistics.
func segmentStatsKey(n uint64) []byte {
	return binary.BigEndian.AppendUint64([]byte(segmentStatsPrefix), n)
}

// addSegmentStats adds delta to segment n's statistics in tx, removing them
// when no block is left.
func addSegmentStats(ctx context.Context, tx kvtx.Tx, n uint64, delta segmentStats) error {
	// Read the current statistics.
	key := segmentStatsKey(n)
	var st segmentStats
	v, found, err := tx.Get(ctx, key)
	if err != nil {
		return err
	}
	if found {
		if st, err = unmarshalSegmentStats(v); err != nil {
			return err
		}
	}

	// Write the sum, or remove it with the segment's last block.
	st = st.add(delta.count, delta.bytes)
	if st.count <= 0 {
		return tx.Delete(ctx, key)
	}
	return tx.Set(ctx, key, st.marshal())
}

// readSegmentStats reads the statistics of every segment with a published
// block.
func (s *Store) readSegmentStats(ctx context.Context) (map[uint64]segmentStats, error) {
	stats := make(map[uint64]segmentStats)
	err := s.view(ctx, func(tx kvtx.Tx) error {
		return tx.ScanPrefix(ctx, []byte(segmentStatsPrefix), func(key, value []byte) error {
			st, err := unmarshalSegmentStats(value)
			if err != nil {
				return err
			}
			stats[binary.BigEndian.Uint64(key[len(segmentStatsPrefix):])] = st
			return nil
		})
	})
	return stats, err
}

// BlockStats returns the number and total payload length of the published
// blocks.
func (s *Store) BlockStats(ctx context.Context) (count, size uint64, err error) {
	stats, err := s.readSegmentStats(ctx)
	for _, st := range stats {
		count += uint64(st.count) //nolint:gosec
		size += uint64(st.bytes)  //nolint:gosec
	}
	return count, size, err
}

// Maintenance reclaims segment space in one bounded pass. It removes the
// sealed segments that hold no published or pending block, then moves up to
// moveBytes of live blocks out of the sealed segment with the most dead bytes,
// if at least half of it is dead. A later pass removes the emptied segment.
func (s *Store) Maintenance(ctx context.Context) error {
	// Snapshot the open segment and the segments with pending payloads before
	// reading the statistics, so a payload published in between is counted.
	s.mtx.Lock()
	open := s.segment
	busy := make(map[uint64]bool)
	for _, p := range s.pending {
		if p.loc != nil {
			busy[p.loc.segment] = true
		}
	}
	s.mtx.Unlock()

	// Read the published statistics and the segment files.
	stats, err := s.readSegmentStats(ctx)
	if err != nil {
		return err
	}
	files, err := s.dev.List(ctx)
	if err != nil {
		return err
	}

	// Find the empty sealed segments and the one with the most dead bytes.
	var empty []string
	var victim uint64
	victimDead := int64(-1)
	for _, f := range files {
		n, ok := parseSegment(f.Name)
		if !ok || n >= open || busy[n] {
			continue
		}
		st := stats[n]
		if st.count == 0 {
			empty = append(empty, f.Name)
			continue
		}
		if dead := f.Size - st.bytes; dead*2 >= f.Size && dead > victimDead {
			victim, victimDead = n, dead
		}
	}

	// Remove the empty segments and move live blocks out of the victim.
	if len(empty) != 0 {
		if err := s.dev.Remove(ctx, empty); err != nil {
			return err
		}
	}
	if victimDead < 0 {
		return nil
	}
	return s.move(ctx, victim)
}

// move rewrites up to moveBytes of segment n's blocks at the end of the log
// and publishes their new locations durably. Each move publishes only if its
// block is still at the old location, so a block removed meanwhile stays
// removed.
func (s *Store) move(ctx context.Context, n uint64) error {
	// Find the segment's blocks, up to moveBytes.
	var keys []string
	var locs []*location
	var size int64
	err := s.view(ctx, func(tx kvtx.Tx) error {
		return tx.ScanPrefix(ctx, []byte(blockPrefix), func(key, value []byte) error {
			// Collect the blocks in segment n until the move is full.
			loc, err := unmarshalLocation(value)
			if err != nil || loc.segment != n || size >= moveBytes {
				return err
			}
			keys, locs = append(keys, string(key)), append(locs, loc)
			size += loc.length
			return nil
		})
	})
	if err != nil || len(keys) == 0 {
		return err
	}

	// Read their payloads in one device call.
	reads := make([]device.Read, len(locs))
	for i, loc := range locs {
		reads[i] = device.Read{Name: segmentName(n), Offset: loc.offset, Data: make([]byte, loc.length)}
	}
	if err := s.dev.Read(ctx, reads); err != nil {
		return errors.Wrap(err, "read moved blocks")
	}

	// Append the payloads as pending moves, skipping blocks with a pending
	// change.
	if err := s.appendMoves(ctx, keys, locs, reads); err != nil {
		return err
	}

	// Publish the moves.
	_, err = s.Sync(ctx)
	return err
}

// appendMoves writes the payloads of the blocks at keys, read from locs, at
// the end of the log and adds them as pending moves.
func (s *Store) appendMoves(ctx context.Context, keys []string, locs []*location, reads []device.Read) error {
	// Place each payload without a pending change at the log's end.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	var writes []device.Write
	added := make(map[string]*pendingBlock)
	for i, key := range keys {
		if s.pending[key] != nil {
			continue
		}
		loc := s.place(locs[i].length)
		writes = append(writes, device.Write{Name: segmentName(loc.segment), Offset: loc.offset, Data: reads[i].Data})
		added[key] = &pendingBlock{loc: loc, from: locs[i]}
	}

	// Write the payloads, then add them as pending moves.
	if len(writes) != 0 {
		if err := s.dev.Write(ctx, writes, false); err != nil {
			return err
		}
	}
	maps.Copy(s.pending, added)
	return nil
}

// place returns the location of the next payload of length bytes, starting
// a new segment when the open one is full. The caller holds mtx.
func (s *Store) place(length int64) *location {
	if s.offset != 0 && s.offset+length > segmentSize {
		s.segment++
		s.offset = 0
	}
	loc := &location{segment: s.segment, offset: s.offset, length: length}
	s.offset += length
	return loc
}
