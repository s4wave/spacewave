package promstorage

import (
	"context"
	"encoding/binary"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/s4wave/spacewave/db/kvtx"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
)

// StoreReader reads one target's key-value tree: its labels, series table and
// segments. It holds no state beyond the transaction, which the caller
// discards.
type StoreReader struct {
	// kv is the transaction over the target's key-value tree.
	kv kvtx.Tx
}

// NewStoreReader constructs a reader over a transaction on a target's tree.
func NewStoreReader(kv kvtx.Tx) *StoreReader {
	return &StoreReader{kv: kv}
}

// Meta returns the target's TargetMeta.
func (r *StoreReader) Meta(ctx context.Context) (*s4wave_prom.TargetMeta, error) {
	// Read and decode the target's TargetMeta.
	data, found, err := r.kv.Get(ctx, s4wave_prom.MetaKey())
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.Wrap(ErrCorruptStore, "missing target meta")
	}
	meta := &s4wave_prom.TargetMeta{}
	if err := meta.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "decode target meta")
	}
	return meta, nil
}

// TargetLabels returns the labels every series of the target carries.
func (r *StoreReader) TargetLabels(ctx context.Context) (labels.Labels, error) {
	meta, err := r.Meta(ctx)
	if err != nil {
		return labels.EmptyLabels(), err
	}
	return ToLabels(meta.GetLabels()), nil
}

// Table reads the target's series table from its series pages.
func (r *StoreReader) Table(ctx context.Context) (*SeriesTable, error) {
	// Append each page's series in key order, which is series number order.
	table := NewSeriesTable()
	it := r.kv.Iterate(ctx, []byte{pageKeyPrefix}, true, false)
	defer it.Close()
	for it.Next() {
		first, err := parsePageKey(it.Key())
		if err != nil {
			return nil, err
		}
		if first != table.Len() {
			return nil, errors.Wrapf(ErrCorruptStore, "series page starts at %d, want %d", first, table.Len())
		}
		data, err := it.Value()
		if err != nil {
			return nil, err
		}
		page := &SeriesPage{}
		if err := page.UnmarshalVT(data); err != nil {
			return nil, errors.Wrap(err, "decode series page")
		}
		for _, set := range page.GetSeries() {
			table.Add(ToLabels(set))
		}
	}
	return table, it.Err()
}

// LastSegmentMaxTime returns the latest sample time the target has committed.
// It returns false when the target has no segment.
func (r *StoreReader) LastSegmentMaxTime(ctx context.Context) (int64, bool, error) {
	// Segment keys sort by time, so the last key holds the latest time.
	it := r.kv.Iterate(ctx, []byte{segmentKeyPrefix}, true, true)
	defer it.Close()
	if !it.Next() {
		return 0, false, it.Err()
	}
	_, maxTime, err := parseSegmentKey(it.Key())
	return maxTime, err == nil, err
}

// ForEachSegment calls fn with each segment overlapping mint to maxt
// inclusive, in time order. Segment keys carry the time range, so a segment
// outside the range is not read.
func (r *StoreReader) ForEachSegment(ctx context.Context, mint, maxt int64, fn func(*Segment) error) error {
	it := r.kv.Iterate(ctx, []byte{segmentKeyPrefix}, true, false)
	defer it.Close()
	for it.Next() {
		// Skip a segment before the range and stop after the last one in it.
		segMin, segMax, err := parseSegmentKey(it.Key())
		if err != nil {
			return err
		}
		if segMin > maxt {
			break
		}
		if segMax < mint {
			continue
		}

		// Decode the segment and hand it to the caller.
		data, err := it.Value()
		if err != nil {
			return err
		}
		seg := &Segment{}
		if err := seg.UnmarshalVT(data); err != nil {
			return errors.Wrap(err, "decode segment")
		}
		if err := fn(seg); err != nil {
			return err
		}
	}
	return it.Err()
}

// Select returns the target's series matching matchers, with their samples in
// mint to maxt. The target's labels are added to every series. The series are
// not sorted.
func (r *StoreReader) Select(ctx context.Context, mint, maxt int64, matchers []*labels.Matcher) ([]storage.Series, error) {
	// Match the target and its series table against the selector.
	meta, err := r.TargetLabels(ctx)
	if err != nil {
		return nil, err
	}
	if !matchesTarget(meta, matchers) {
		return nil, nil
	}
	table, err := r.Table(ctx)
	if err != nil {
		return nil, err
	}
	matched := matchSeries(meta, table, matchers)
	if len(matched) == 0 {
		return nil, nil
	}

	// Collect the matched series' chunks that overlap the range.
	chunks := make(map[uint64][]chunkenc.Chunk, len(matched))
	err = r.ForEachSegment(ctx, mint, maxt, func(seg *Segment) error {
		for _, sc := range seg.GetSeries() {
			if _, ok := matched[sc.GetSeries()]; !ok {
				continue
			}
			for _, c := range sc.GetChunks() {
				if c.GetMaxTime() < mint || c.GetMinTime() > maxt {
					continue
				}
				if c.GetEncoding() != ChunkEncoding_CHUNK_ENCODING_XOR {
					return errors.Wrapf(ErrUnknownEncoding, "encoding %v", c.GetEncoding())
				}
				chunk, err := chunkenc.FromData(chunkenc.EncXOR, c.GetData())
				if err != nil {
					return errors.Wrap(err, "decode chunk")
				}
				chunks[sc.GetSeries()] = append(chunks[sc.GetSeries()], chunk)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Build one series per matched number that has samples in the range.
	out := make([]storage.Series, 0, len(chunks))
	for num, parts := range chunks {
		out = append(out, newChunkSeries(matched[num], parts))
	}
	return out, nil
}

// Labels returns the full label sets of the target's series matching
// matchers, ignoring time.
func (r *StoreReader) Labels(ctx context.Context, matchers []*labels.Matcher) ([]labels.Labels, error) {
	// Match the target and its series table against the selector.
	meta, err := r.TargetLabels(ctx)
	if err != nil {
		return nil, err
	}
	if !matchesTarget(meta, matchers) {
		return nil, nil
	}
	table, err := r.Table(ctx)
	if err != nil {
		return nil, err
	}

	// Return the matching series' label sets.
	matched := matchSeries(meta, table, matchers)
	out := make([]labels.Labels, 0, len(matched))
	for _, lset := range matched {
		out = append(out, lset)
	}
	return out, nil
}

// matchesTarget reports whether a target with the given labels can hold a
// series matching all matchers. A matcher on a name the target sets decides
// for every series of the target, so the target's segments need not be read.
func matchesTarget(meta labels.Labels, matchers []*labels.Matcher) bool {
	for _, m := range matchers {
		if meta.Has(m.Name) && !m.Matches(meta.Get(m.Name)) {
			return false
		}
	}
	return true
}

// matchSeries returns the full label sets, target labels included, of the
// series in table matching all matchers, by series number.
func matchSeries(meta labels.Labels, table *SeriesTable, matchers []*labels.Matcher) map[uint64]labels.Labels {
	out := make(map[uint64]labels.Labels)
	for num := range table.Len() {
		full := mergeLabels(meta, table.Labels(num))
		match := true
		for _, m := range matchers {
			if !m.Matches(full.Get(m.Name)) {
				match = false
				break
			}
		}
		if match {
			out[num] = full
		}
	}
	return out
}

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
