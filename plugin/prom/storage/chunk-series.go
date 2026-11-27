package promstorage

import (
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// newChunkSeries returns a series whose samples are those of chunks, which are
// in time order and do not overlap.
func newChunkSeries(lset labels.Labels, chunks []chunkenc.Chunk) storage.Series {
	parts := make([]storage.Series, len(chunks))
	for i, c := range chunks {
		parts[i] = &storage.SeriesEntry{
			Lset: lset,
			SampleIteratorFn: func(it chunkenc.Iterator) chunkenc.Iterator {
				return c.Iterator(it)
			},
		}
	}
	return storage.ChainedSeriesMerge(parts...)
}
