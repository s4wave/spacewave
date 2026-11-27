package promstorage

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/s4wave/spacewave/db/kvtx"
)

// CompactSpan is the time bucket that compaction merges a target's segments
// into, the length of a Prometheus block.
const CompactSpan = 2 * time.Hour

// compactBucket returns the number of the CompactSpan bucket holding t.
func compactBucket(t int64) int64 {
	span := CompactSpan.Milliseconds()
	if t < 0 {
		return (t+1)/span - 1
	}
	return t / span
}

// segmentRef is a segment's key and time range, read without its body.
type segmentRef struct {
	// key is the segment's tree key.
	key []byte
	// minTime is the segment's earliest sample time.
	minTime int64
	// maxTime is the segment's latest sample time.
	maxTime int64
}

// maintain drops the segments that ended more than retention before now, the
// latest committed sample, and merges the segments of each closed bucket into
// one, within the transaction kv. A bucket is closed when it is before the
// bucket of now+1, the earliest time a later commit can write. Segments are
// grouped by the bucket of their earliest sample, so a merged segment can end
// past its bucket but never overlaps the next. Running maintain again on a
// maintained tree changes nothing.
func maintain(ctx context.Context, kv kvtx.Tx, now int64, retention time.Duration) error {
	// Read every segment's range from its key.
	var refs []segmentRef
	it := kv.Iterate(ctx, []byte{segmentKeyPrefix}, true, false)
	for it.Next() {
		minTime, maxTime, err := parseSegmentKey(it.Key())
		if err != nil {
			it.Close()
			return err
		}
		refs = append(refs, segmentRef{key: append([]byte(nil), it.Key()...), minTime: minTime, maxTime: maxTime})
	}
	err := it.Err()
	it.Close()
	if err != nil {
		return err
	}

	// Drop the expired segments and merge each closed bucket's run.
	expiry := now - retention.Milliseconds()
	openBucket := compactBucket(now + 1)
	var run []segmentRef
	flush := func() error {
		if len(run) > 1 {
			if err := mergeSegments(ctx, kv, run); err != nil {
				return err
			}
		}
		run = run[:0]
		return nil
	}
	for _, ref := range refs {
		if ref.maxTime < expiry {
			if err := kv.Delete(ctx, ref.key); err != nil {
				return err
			}
			continue
		}
		bucket := compactBucket(ref.minTime)
		if bucket >= openBucket {
			break
		}
		if len(run) != 0 && compactBucket(run[0].minTime) != bucket {
			if err := flush(); err != nil {
				return err
			}
		}
		run = append(run, ref)
	}
	return flush()
}

// mergeSegments replaces the consecutive segments run with one segment holding
// their samples, re-cut into chunks of ChunkSamples.
func mergeSegments(ctx context.Context, kv kvtx.Tx, run []segmentRef) error {
	// Replay every sample of the run, in time order, into one window.
	w := NewWindow(0)
	for _, ref := range run {
		data, found, err := kv.Get(ctx, ref.key)
		if err != nil {
			return err
		}
		if !found {
			return errors.Wrapf(ErrCorruptStore, "segment %x vanished", ref.key)
		}
		seg := &Segment{}
		if err := seg.UnmarshalVT(data); err != nil {
			return errors.Wrap(err, "decode segment")
		}
		if err := appendSegment(w, seg); err != nil {
			return err
		}
	}

	// Write the merged segment and delete the run's keys.
	merged := w.Segment()
	if merged == nil {
		return errors.Wrap(ErrCorruptStore, "merged segments hold no samples")
	}
	data, err := merged.MarshalVT()
	if err != nil {
		return err
	}
	for _, ref := range run {
		if err := kv.Delete(ctx, ref.key); err != nil {
			return err
		}
	}
	return kv.Set(ctx, segmentKey(merged.GetMinTime(), merged.GetMaxTime()), data)
}

// appendSegment appends every sample of seg to w.
func appendSegment(w *Window, seg *Segment) error {
	for _, sc := range seg.GetSeries() {
		for _, c := range sc.GetChunks() {
			if c.GetEncoding() != ChunkEncoding_CHUNK_ENCODING_XOR {
				return errors.Wrapf(ErrUnknownEncoding, "encoding %v", c.GetEncoding())
			}
			chunk, err := chunkenc.FromData(chunkenc.EncXOR, c.GetData())
			if err != nil {
				return errors.Wrap(err, "decode chunk")
			}
			it := chunk.Iterator(nil)
			for it.Next() == chunkenc.ValFloat {
				t, v := it.At()
				if err := w.Append(sc.GetSeries(), t, v); err != nil {
					return err
				}
			}
			if err := it.Err(); err != nil {
				return errors.Wrap(err, "read chunk")
			}
		}
	}
	return nil
}
