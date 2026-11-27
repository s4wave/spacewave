package promstorage

import (
	"maps"
	"math"
	"slices"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// ChunkSamples is the number of samples after which a window starts a new
// chunk for a series, the same cut Prometheus uses for its head chunks.
const ChunkSamples = 120

// Window collects the samples one commit appends, per series, in XOR chunks.
//
// A window holds only samples that are not yet committed, so the bytes a
// commit writes follow the window, not the history of the series. A Window is
// not safe for concurrent use. The Store that owns it serializes access.
type Window struct {
	// firstSeries is the series number of the first series the window
	// introduced. The window's series table entries run from it to the table
	// end.
	firstSeries uint64
	// series holds the window's samples by series number.
	series map[uint64]*windowSeries
	// samples is the number of samples in the window.
	samples int
	// minTime is the earliest sample timestamp, valid when samples > 0.
	minTime int64
	// maxTime is the latest sample timestamp, valid when samples > 0.
	maxTime int64
}

// windowSeries is one series' samples within a window.
type windowSeries struct {
	// chunks are the series' chunks in time order. The last one is open.
	chunks []*windowChunk
	// lastTime is the timestamp of the series' latest sample.
	lastTime int64
	// lastValue is the value of the series' latest sample.
	lastValue float64
}

// windowChunk is one XOR chunk being appended to.
type windowChunk struct {
	// xor is the chunk being built.
	xor *chunkenc.XORChunk
	// app appends to xor.
	app chunkenc.Appender
	// minTime is the timestamp of the chunk's first sample.
	minTime int64
	// maxTime is the timestamp of the chunk's last sample.
	maxTime int64
	// count is the number of samples in the chunk.
	count int
}

// NewWindow constructs an empty window. firstSeries is the number the next
// new series of the target's series table will get.
func NewWindow(firstSeries uint64) *Window {
	return &Window{firstSeries: firstSeries, series: make(map[uint64]*windowSeries)}
}

// FirstSeries returns the number of the first series the window introduced.
func (w *Window) FirstSeries() uint64 {
	return w.firstSeries
}

// Samples returns the number of samples in the window.
func (w *Window) Samples() int {
	return w.samples
}

// Append adds a sample of series num.
//
// Timestamps of one series must increase. Repeating the latest timestamp with
// the same value is accepted and ignored, as Prometheus does. Repeating it with
// another value returns storage.ErrDuplicateSampleForTimestamp, and an earlier
// timestamp returns storage.ErrOutOfOrderSample.
func (w *Window) Append(num uint64, t int64, v float64) error {
	// Check the sample against the series' latest sample.
	s := w.series[num]
	if s != nil && t <= s.lastTime {
		if t < s.lastTime {
			return storage.ErrOutOfOrderSample
		}
		if math.Float64bits(v) != math.Float64bits(s.lastValue) {
			return storage.ErrDuplicateSampleForTimestamp
		}
		return nil
	}

	// Open the series, or cut its chunk once the chunk holds ChunkSamples
	// samples.
	if s == nil {
		s = &windowSeries{}
		w.series[num] = s
	}
	if len(s.chunks) == 0 || s.chunks[len(s.chunks)-1].count >= ChunkSamples {
		xor := chunkenc.NewXORChunk()
		app, err := xor.Appender()
		if err != nil {
			return errors.Wrap(err, "open chunk appender")
		}
		s.chunks = append(s.chunks, &windowChunk{xor: xor, app: app, minTime: t})
	}

	// Append to the open chunk and record the series' latest sample.
	c := s.chunks[len(s.chunks)-1]
	c.app.Append(0, t, v)
	c.maxTime = t
	c.count++
	s.lastTime, s.lastValue = t, v

	// Extend the window's time range and count the sample.
	if w.samples == 0 || t < w.minTime {
		w.minTime = t
	}
	if w.samples == 0 || t > w.maxTime {
		w.maxTime = t
	}
	w.samples++
	return nil
}

// Segment encodes the window as a segment. It returns nil for an empty window.
func (w *Window) Segment() *Segment {
	if w.samples == 0 {
		return nil
	}
	seg := &Segment{
		MinTime: w.minTime,
		MaxTime: w.maxTime,
		Series:  make([]*SeriesChunks, 0, len(w.series)),
	}
	for _, num := range slices.Sorted(maps.Keys(w.series)) {
		s := w.series[num]
		sc := &SeriesChunks{Series: num, Chunks: make([]*Chunk, 0, len(s.chunks))}
		for _, c := range s.chunks {
			sc.Chunks = append(sc.Chunks, &Chunk{
				MinTime:  c.minTime,
				MaxTime:  c.maxTime,
				Encoding: ChunkEncoding_CHUNK_ENCODING_XOR,
				Data:     c.xor.Bytes(),
			})
		}
		seg.Series = append(seg.Series, sc)
	}
	return seg
}
