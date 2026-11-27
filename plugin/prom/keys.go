package prom_plugin

import (
	"encoding/binary"

	"github.com/pkg/errors"
)

// Key prefixes belong to the segment store in a target's key-value tree. The
// meta and status keys are in s4wave_prom. Keys sort by prefix, then by the
// big-endian numbers that follow, so a prefix scan returns series pages and
// segments in the order they were committed.
const (
	// pageKeyPrefix starts the key of a SeriesPage: prefix, first series.
	pageKeyPrefix byte = 0x01
	// segmentKeyPrefix starts the key of a Segment: prefix, min time, max time.
	segmentKeyPrefix byte = 0x02
)

// signBit flips the sign of a timestamp so negative times sort first as
// unsigned big-endian bytes.
const signBit = 1 << 63

// sortableTime returns t as an unsigned value that sorts in time order.
func sortableTime(t int64) uint64 {
	return uint64(t) ^ signBit //nolint:gosec // the wrap is the encoding.
}

// parseSortableTime inverts sortableTime.
func parseSortableTime(u uint64) int64 {
	return int64(u ^ signBit) //nolint:gosec // the wrap is the encoding.
}

// pageKey returns the key of the SeriesPage whose first series is first.
func pageKey(first uint64) []byte {
	return binary.BigEndian.AppendUint64([]byte{pageKeyPrefix}, first)
}

// segmentKey returns the key of the Segment covering minTime to maxTime.
func segmentKey(minTime, maxTime int64) []byte {
	key := binary.BigEndian.AppendUint64([]byte{segmentKeyPrefix}, sortableTime(minTime))
	return binary.BigEndian.AppendUint64(key, sortableTime(maxTime))
}

// parsePageKey returns the first series number of a SeriesPage key.
func parsePageKey(key []byte) (uint64, error) {
	if len(key) != 9 || key[0] != pageKeyPrefix {
		return 0, errors.Wrapf(ErrCorruptStore, "series page key %x", key)
	}
	return binary.BigEndian.Uint64(key[1:]), nil
}

// parseSegmentKey returns the time range of a Segment key.
func parseSegmentKey(key []byte) (minTime, maxTime int64, err error) {
	if len(key) != 17 || key[0] != segmentKeyPrefix {
		return 0, 0, errors.Wrapf(ErrCorruptStore, "segment key %x", key)
	}
	minTime = parseSortableTime(binary.BigEndian.Uint64(key[1:9]))
	maxTime = parseSortableTime(binary.BigEndian.Uint64(key[9:]))
	return minTime, maxTime, nil
}
