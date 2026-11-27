package prom_plugin

import (
	"context"
	"slices"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
)

// referenceQueryable is the in-memory reference: a storage.Queryable over
// samples held in Go slices, built with the fork's own list series.
type referenceQueryable struct {
	// series holds the samples by encoded label set.
	series map[string]*referenceSeries
}

// referenceSeries is one series of the reference.
type referenceSeries struct {
	// lset is the series' full label set.
	lset labels.Labels
	// samples are the series' samples in time order.
	samples []chunks.Sample
}

// newReferenceQueryable constructs an empty reference.
func newReferenceQueryable() *referenceQueryable {
	return &referenceQueryable{series: make(map[string]*referenceSeries)}
}

// add records a sample of the series lset.
func (r *referenceQueryable) add(lset labels.Labels, t int64, v float64) {
	key := string(lset.Bytes(nil))
	s := r.series[key]
	if s == nil {
		s = &referenceSeries{lset: lset}
		r.series[key] = s
	}
	s.samples = append(s.samples, referenceSample{t: t, f: v})
}

// Querier returns a querier over all recorded samples. The engine and the
// series iterators apply the time range.
func (r *referenceQueryable) Querier(int64, int64) (storage.Querier, error) {
	return r, nil
}

// Select returns the series matching matchers, sorted by labels.
func (r *referenceQueryable) Select(_ context.Context, _ bool, _ *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	var out []storage.Series
	for _, s := range r.series {
		match := true
		for _, m := range matchers {
			if !m.Matches(s.lset.Get(m.Name)) {
				match = false
				break
			}
		}
		if match {
			out = append(out, storage.NewListSeries(s.lset, s.samples))
		}
	}
	slices.SortFunc(out, func(a, b storage.Series) int {
		return labels.Compare(a.Labels(), b.Labels())
	})
	return NewSeriesSet(out)
}

// LabelValues is unused by the reference comparison.
func (r *referenceQueryable) LabelValues(context.Context, string, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

// LabelNames is unused by the reference comparison.
func (r *referenceQueryable) LabelNames(context.Context, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

// Close releases nothing.
func (r *referenceQueryable) Close() error { return nil }

// _ is a type assertion
var (
	_ storage.Queryable = (*referenceQueryable)(nil)
	_ storage.Querier   = (*referenceQueryable)(nil)
)
